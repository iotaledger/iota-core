package mempoolv1

import (
	"context"
	"errors"
	"fmt"
	"sync"

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

	lastEvictedSlot iotago.SlotIndex

	evictionMutex sync.RWMutex
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

func (m *MemPool[VotePower]) AttachTransaction(transaction mempool.Transaction, blockID iotago.BlockID) (metadata mempool.TransactionWithMetadata, err error) {
	storedTransaction, isNew, err := m.storeTransaction(transaction, blockID)
	if err != nil {
		return nil, xerrors.Errorf("failed to store transaction: %w", err)
	}

	if isNew {
		m.setupTransactionLifecycle(storedTransaction)
	}

	return storedTransaction, nil
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

func (m *MemPool[VotePower]) Evict(slotIndex iotago.SlotIndex) {
	evict := func() *shrinkingmap.ShrinkingMap[iotago.BlockID, *TransactionWithMetadata] {
		m.evictionMutex.Lock()
		defer m.evictionMutex.Unlock()

		m.lastEvictedSlot = slotIndex

		return m.attachments.Evict(slotIndex)
	}

	if attachmentsInSlot := evict(); attachmentsInSlot != nil {
		attachmentsInSlot.ForEach(func(blockID iotago.BlockID, transaction *TransactionWithMetadata) bool {
			transaction.attachments.Evict(blockID)

			return true
		})
	}
}

func (m *MemPool[VotePower]) ConflictDAG() conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, VotePower] {
	return m.conflictDAG
}

func (m *MemPool[VotePower]) Events() *mempool.Events {
	return m.events
}

func (m *MemPool[VotePower]) setupTransactionLifecycle(transaction *TransactionWithMetadata) {
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

	transaction.Attachments().OnEarliestIncludedAttachmentUpdated(func(blockID iotago.BlockID) {
		if blockID.Index() == 0 && transaction.IsAccepted() {
			// TODO: roll back acceptance in diff + mark tx as orphaned
		} else if blockID.Index() > 0 && !transaction.IsAccepted() {
			m.updateAcceptance(transaction)
		}
	})

	transaction.OnAllInputsAccepted(func() {
		m.updateAcceptance(transaction)
	})

	transaction.OnCommitted(func() {
		transaction.Attachments().attachments.ForEach(func(blockID iotago.BlockID, _ AttachmentStatus) bool {
			m.attachments.Get(blockID.Index(), false).Delete(blockID)
			return true
		})

		m.cachedTransactions.Delete(transaction.ID())
	})

	transaction.OnEvicted(func() {
		m.cachedTransactions.Delete(transaction.ID())
	})
}

func (m *MemPool[VotePower]) setupStateLifecycle(state *StateWithMetadata) {
	deleteIfNoConsumers := func() {
		if !m.cachedStateRequests.Delete(state.ID(), state.HasNoConsumers) && m.cachedStateRequests.Has(state.ID()) {
			state.OnAllConsumersEvicted(func() {
				m.cachedStateRequests.Delete(state.ID(), state.HasNoConsumers)
			})
		}
	}

	state.OnCommitted(func() {
		deleteIfNoConsumers()
	})
	state.OnEvicted(func() {
		m.cachedStateRequests.Delete(state.ID())
	})
}

func (m *MemPool[VotePower]) storeTransaction(transaction mempool.Transaction, blockID iotago.BlockID) (storedTransaction *TransactionWithMetadata, isNew bool, err error) {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	if m.lastEvictedSlot >= blockID.Index() {
		return nil, false, xerrors.Errorf("blockID %d is older than last evicted slot %d", blockID.Index(), m.lastEvictedSlot)
	}

	newMetadata, err := NewTransactionWithMetadata(transaction)
	if err != nil {
		return nil, false, xerrors.Errorf("failed to create transaction metadata: %w", err)
	}

	storedTransaction, isNew = m.cachedTransactions.GetOrCreate(newMetadata.ID(), func() *TransactionWithMetadata {
		newMetadata.setStored()

		return newMetadata
	})

	storedTransaction.attachments.Add(blockID)
	m.attachments.Get(blockID.Index(), true).Set(blockID, storedTransaction)

	return storedTransaction, isNew, nil
}

func (m *MemPool[VotePower]) solidifyInputs(transaction *TransactionWithMetadata) {
	for i, inputReference := range transaction.inputReferences {
		stateReference, index := inputReference, i

		request, created := m.cachedStateRequests.GetOrCreate(stateReference.StateID(), func() *promise.Promise[*StateWithMetadata] {
			return m.requestState(stateReference, true)
		})

		request.OnSuccess(func(input *StateWithMetadata) {
			transaction.publishInput(index, input)

			if created {
				m.setupStateLifecycle(input)
			}
		})

		request.OnError(transaction.setInvalid)
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
	lo.ForEach(transaction.inputs, func(input *StateWithMetadata) {
		input.OnDoubleSpent(func() {
			m.forkTransaction(transaction, input)
		})
	})

	transaction.setBooked()
}

func (m *MemPool[VotePower]) publishOutputs(transaction *TransactionWithMetadata) {
	for _, output := range transaction.outputs {
		outputRequest, isNew := m.cachedStateRequests.GetOrCreate(output.id, lo.NoVariadic(promise.New[*StateWithMetadata]))
		outputRequest.Resolve(output)

		if isNew {
			m.setupStateLifecycle(output)
		}
	}
}

func (m *MemPool[VotePower]) requestState(stateReference ledger.StateReference, waitIfMissing ...bool) *promise.Promise[*StateWithMetadata] {
	return promise.New[*StateWithMetadata](func(p *promise.Promise[*StateWithMetadata]) {
		request := m.requestInput(stateReference)

		request.OnSuccess(func(state ledger.State) {
			stateWithMetadata := NewStateWithMetadata(state)
			stateWithMetadata.setAccepted()
			stateWithMetadata.setCommitted()

			p.Resolve(stateWithMetadata)
		})

		request.OnError(func(err error) {
			if !lo.First(waitIfMissing) || !errors.Is(err, ledger.ErrStateNotFound) {
				p.Reject(err)
			}
		})
	})
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

func (m *MemPool[VotePower]) updateAcceptance(transaction *TransactionWithMetadata) {
	if transaction.AllInputsAccepted() && transaction.Attachments().WasIncluded() && m.conflictDAG.AcceptanceState(transaction.conflictIDs).IsAccepted() {
		transaction.setAccepted()
	}
}

func (m *MemPool[VotePower]) StateDiff(index iotago.SlotIndex) (*mempool.StateDiff, error) {
	return nil, fmt.Errorf("not implemented yet")
}

var _ mempool.MemPool[vote.MockedPower] = new(MemPool[vote.MockedPower])
