package mempoolv1

import (
	"context"
	"errors"

	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/acceptance"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	mockedconflictdag "github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag/mocked"
	iotago "github.com/iotaledger/iota.go/v4"
)

type MemPool[VotePower conflictdag.VotePowerType[VotePower]] struct {
	events *mempool.Events

	executeStateTransition mempool.VM

	requestInput ledger.StateReferenceResolver

	attachments *memstorage.IndexedStorage[iotago.SlotIndex, iotago.BlockID, *TransactionWithMetadata]

	cachedTransactions *shrinkingmap.ShrinkingMap[iotago.TransactionID, *TransactionWithMetadata]

	cachedStateRequests *shrinkingmap.ShrinkingMap[iotago.OutputID, *promise.Promise[*StateWithMetadata]]

	conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, VotePower]

	executionWorkers *workerpool.WorkerPool
}

func New[VotePower conflictdag.VotePowerType[VotePower]](vm mempool.VM, inputResolver ledger.StateReferenceResolver, workers *workerpool.Group) *MemPool[VotePower] {
	m := &MemPool[VotePower]{
		events:                 mempool.NewEvents(),
		executeStateTransition: vm,
		requestInput:           inputResolver,
		attachments:            memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.BlockID, *TransactionWithMetadata](),
		cachedTransactions:     shrinkingmap.New[iotago.TransactionID, *TransactionWithMetadata](),
		cachedStateRequests:    shrinkingmap.New[iotago.OutputID, *promise.Promise[*StateWithMetadata]](),
		executionWorkers:       workers.CreatePool("executionWorkers", 1),
		conflictDAG:            mockedconflictdag.New[iotago.TransactionID, iotago.OutputID, VotePower](),
	}

	m.ConflictDAG().Events().ConflictAccepted.Hook(func(id iotago.TransactionID) {
		if transaction, exists := m.cachedTransactions.Get(id); !exists {
			m.updateAcceptance(transaction)
		}
	})

	return m
}

func (m *MemPool[VotePower]) Evict(slotIndex iotago.SlotIndex) {
	attachmentsInSlot := m.attachments.Evict(slotIndex)
	if attachmentsInSlot == nil {
		return
	}

	attachmentsInSlot.ForEach(func(blockID iotago.BlockID, transaction *TransactionWithMetadata) bool {
		transaction.attachments.Evict(blockID)
		return true
	})
}

func (m *MemPool[VotePower]) Events() *mempool.Events {
	return m.events
}

func (m *MemPool[VotePower]) ConflictDAG() conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, VotePower] {
	return m.conflictDAG
}

func (m *MemPool[VotePower]) AttachTransaction(transaction mempool.Transaction, blockID iotago.BlockID) (metadata mempool.TransactionWithMetadata, err error) {
	newMetadata, err := NewTransactionWithMetadata(transaction)
	if err != nil {
		return nil, xerrors.Errorf("failed to create transaction metadata: %w", err)
	}

	newMetadata, stored := m.cachedTransactions.GetOrCreate(newMetadata.ID(), func() *TransactionWithMetadata {
		return m.setupTransaction(newMetadata)
	})

	// TODO CLEANUP GLOBAL attachments
	m.attachments.Get(blockID.Index(), true).Set(blockID, newMetadata)
	newMetadata.attachments.Add(blockID)

	if !stored {
		return metadata, xerrors.Errorf("transaction with id %s already exists: %w", newMetadata.ID(), mempool.ErrTransactionExistsAlready)
	}

	newMetadata.setStored()

	return newMetadata, nil
}

func (m *MemPool[VotePower]) RemoveTransaction(transactionID iotago.TransactionID) {
	if transaction, deleted := m.cachedTransactions.DeleteAndReturn(transactionID); deleted {
		transaction.evicted.Trigger()
	}
}

func (m *MemPool[VotePower]) Transaction(id iotago.TransactionID) (transaction mempool.TransactionWithMetadata, exists bool) {
	return m.cachedTransactions.Get(id)
}

func (m *MemPool[VotePower]) State(stateReference ledger.StateReference) (state mempool.StateWithMetadata, err error) {
	stateRequest, exists := m.cachedStateRequests.Get(stateReference.StateID())
	if !exists || !stateRequest.WasCompleted() {
		stateRequest = m.requestState(stateReference)
	}

	stateRequest.OnSuccess(func(loadedState *StateWithMetadata) { state = loadedState })
	stateRequest.OnError(func(requestErr error) { err = xerrors.Errorf("failed to request state: %w", requestErr) })
	stateRequest.WaitComplete()

	return state, err
}

func (m *MemPool[VotePower]) MarkAttachmentIncluded(blockID iotago.BlockID) error {
	attachmentsBySlotIndex := m.attachments.Get(blockID.Index())
	if attachmentsBySlotIndex == nil {
		return xerrors.Errorf("no attachments found for block id %s: %w", blockID, mempool.ErrAttachmentNotFound)
	}

	transaction, exists := attachmentsBySlotIndex.Get(blockID)
	if !exists {
		return xerrors.Errorf("no attachments found for block id %s: %w", blockID, mempool.ErrAttachmentNotFound)
	}

	transaction.attachments.MarkIncluded(blockID)

	return nil
}

func (m *MemPool[VotePower]) setupTransaction(transaction *TransactionWithMetadata) *TransactionWithMetadata {
	transaction.OnStored(func() {
		m.events.TransactionStored.Trigger(transaction)
		m.solidifyInputs(transaction)
	})

	transaction.OnSolid(func() {
		m.events.TransactionSolid.Trigger(transaction)
		m.executeTransaction(transaction)
	})

	transaction.OnExecuted(func() {
		m.events.TransactionExecuted.Trigger(transaction)
		m.bookTransaction(transaction)
	})

	transaction.OnBooked(func() {
		m.events.TransactionBooked.Trigger(transaction)
		m.publishOutputs(transaction)
	})

	transaction.OnInvalid(func(err error) {
		m.events.TransactionInvalid.Trigger(transaction, err)
	})

	transaction.OnAccepted(func() {
		m.events.TransactionAccepted.Trigger(transaction)
	})

	transaction.attachments.OnEarliestIncludedSlotUpdated(func(index iotago.SlotIndex) {
		switch index {
		case 0:
			if transaction.IsAccepted() {
				// roll back acceptance in diff + mark tx as orphaned
			}
		default:
			m.updateAcceptance(transaction)
		}
	})
	transaction.OnAllInputsAccepted(func() { m.updateAcceptance(transaction) })

	transaction.attachments.OnAllAttachmentsEvicted(func() {
		transaction.setEvicted()
	})

	transaction.OnCommitted(func() {
		if m.cachedTransactions.Delete(transaction.ID()) {
			// TODO ? m.events.TransactionEvicted.Trigger(transaction)
		}
	})

	transaction.OnEvicted(func() {
		if m.cachedTransactions.Delete(transaction.ID()) {
			// TODO ? m.events.TransactionEvicted.Trigger(transaction)
		}
	})

	return transaction
}

func (m *MemPool[VotePower]) setupInput(transaction *TransactionWithMetadata, input *StateWithMetadata) *StateWithMetadata {
	input.increaseConsumerCount()
	transaction.OnEvicted(input.decreaseConsumerCount)
	transaction.OnCommitted(input.decreaseConsumerCount)

	input.OnAccepted(transaction.markInputAccepted)
	input.OnRejected(transaction.setRejected)
	input.OnEvicted(func() {
		if transaction.IsRejected() {
			transaction.setEvicted()
		}
	})

	input.OnDoubleSpent(func() {
		m.forkTransaction(transaction, input)
	})

	input.OnSpendAccepted(func(spender *TransactionWithMetadata) {
		if spender != transaction {
			transaction.setRejected()
		}
	})

	input.OnSpendCommitted(func(spender *TransactionWithMetadata) {
		if spender != transaction {
			transaction.setEvicted()
		}
	})

	return input
}

func (m *MemPool[VotePower]) forkTransaction(transaction *TransactionWithMetadata, input *StateWithMetadata) {
	switch err := m.conflictDAG.CreateConflict(transaction.ID(), transaction.conflictIDs, advancedset.New(input.ID()), acceptance.Pending); {
	case errors.Is(err, conflictdag.ErrConflictExists):
		m.conflictDAG.JoinConflictSets(transaction.ID(), advancedset.New(input.ID()))
	case err != nil:
		panic(err)
	default:
		//transaction.setConflicting()
	}
}

func (m *MemPool[VotePower]) solidifyInputs(transaction *TransactionWithMetadata) {
	for i, input := range transaction.inputReferences {
		currentIndex := i

		m.cachedStateRequests.Compute(input.StateID(), func(request *promise.Promise[*StateWithMetadata], exists bool) *promise.Promise[*StateWithMetadata] {
			if !exists {
				request = m.requestState(input, true)
			}

			request.OnSuccess(func(input *StateWithMetadata) {
				if !exists {
					input.OnAllConsumersEvicted(func() {
						m.cachedStateRequests.Delete(input.ID(), input.AllConsumersEvicted)
					})
				}

				transaction.publishInput(currentIndex, m.setupInput(transaction, input))
			})
			request.OnError(transaction.setInvalid)

			return request
		})
	}
}

func (m *MemPool[VotePower]) executeTransaction(transaction *TransactionWithMetadata) {
	m.executionWorkers.Submit(func() {
		if outputStates, err := m.executeStateTransition(transaction.Transaction(), lo.Map(transaction.inputs, (*StateWithMetadata).State), context.Background()); err != nil {
			transaction.setInvalid(err)
		} else {
			transaction.setExecuted(outputStates)
		}
	})
}

func (m *MemPool[VotePower]) bookTransaction(transaction *TransactionWithMetadata) {
	transaction.setBooked()
}

func (m *MemPool[VotePower]) publishOutputs(transaction *TransactionWithMetadata) {
	for _, output := range transaction.outputs {
		lo.Return1(m.cachedStateRequests.GetOrCreate(output.id, lo.NoVariadic(promise.New[*StateWithMetadata]))).Resolve(output)

		output.OnAllConsumersEvicted(func() {
			m.cachedStateRequests.Delete(output.id, output.AllConsumersEvicted)
		})
	}
}

func (m *MemPool[VotePower]) requestState(stateReference ledger.StateReference, waitIfMissing ...bool) *promise.Promise[*StateWithMetadata] {
	return promise.New[*StateWithMetadata](func(p *promise.Promise[*StateWithMetadata]) {
		request := m.requestInput(stateReference)

		request.OnSuccess(func(state ledger.State) {
			stateWithMetadata := NewStateWithMetadata(state)
			stateWithMetadata.setAccepted()

			p.Resolve(stateWithMetadata)
		})

		request.OnError(func(err error) {
			if lo.First(waitIfMissing) && errors.Is(err, ledger.ErrStateNotFound) {
				return
			}

			p.Reject(err)
		})
	})
}

func (m *MemPool[VotePower]) updateAcceptance(transaction *TransactionWithMetadata) {
	if transaction.AllInputsAccepted() && transaction.Attachments().WasIncluded() && m.conflictDAG.AcceptanceState(transaction.conflictIDs).IsAccepted() {
		transaction.setAccepted()
	}
}

var _ mempool.MemPool[vote.MockedPower] = new(MemPool[vote.MockedPower])
