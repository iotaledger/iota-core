package mempoolv1

import (
	"context"
	"errors"

	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/workerpool"
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

func (m *MemPool[VotePower]) Events() *mempool.Events {
	return m.events
}

func (m *MemPool[VotePower]) ConflictDAG() conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, VotePower] {
	return m.conflictDAG
}

func (m *MemPool[VotePower]) AddTransaction(transaction mempool.Transaction) (metadata mempool.TransactionWithMetadata, err error) {
	newMetadata, err := NewTransactionWithMetadata(transaction)
	if err != nil {
		return nil, xerrors.Errorf("failed to create transaction metadata: %w", err)
	}

	metadata, stored := m.cachedTransactions.GetOrCreate(newMetadata.ID(), func() *TransactionWithMetadata {
		return m.setupTransaction(newMetadata)
	})
	if !stored {
		return metadata, xerrors.Errorf("transaction with id %s already exists: %w", newMetadata.ID(), mempool.ErrTransactionExistsAlready)
	}

	m.events.TransactionStored.Trigger(metadata)

	m.solidifyInputs(newMetadata)

	return metadata, nil
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

func (m *MemPool[VotePower]) SetTransactionIncluded(id iotago.TransactionID, slot iotago.SlotIndex) error {
	transaction, exists := m.cachedTransactions.Get(id)
	if !exists {
		return xerrors.Errorf("transaction with id %s not found: %w", id, mempool.ErrTransactionNotFound)
	}

	switch previousSlot := transaction.setInclusionSlot(slot); true {
	case previousSlot == 0:
		m.updateAcceptance(transaction)
	case previousSlot < slot:
		//transaction.triggerInclusionSlotUpdated(previousSlot, slot)
	}

	return nil
}

func (m *MemPool[VotePower]) setupTransaction(transaction *TransactionWithMetadata) *TransactionWithMetadata {
	transaction.exposeEvents(m.events)

	transaction.OnSolid(func() { m.executeTransaction(transaction) })
	transaction.OnExecuted(func() { m.bookTransaction(transaction) })
	transaction.OnBooked(func() { m.publishOutputs(transaction) })

	transaction.OnIncluded(func() { m.updateAcceptance(transaction) })
	transaction.OnAllInputsAccepted(func() { m.updateAcceptance(transaction) })

	return transaction
}

func (m *MemPool[VotePower]) setupInput(transaction *TransactionWithMetadata, input *StateWithMetadata) *StateWithMetadata {
	input.OnAccepted(transaction.markInputAccepted)
	input.OnRejected(transaction.setRejected)
	input.OnSpendAccepted(func(spender *TransactionWithMetadata) {
		if spender != transaction {
			transaction.setRejected()
		}
	})

	return input
}

func (m *MemPool[VotePower]) solidifyInputs(transaction *TransactionWithMetadata) {
	for i, input := range transaction.inputReferences {
		solidificationRequest, cancelRequest := func(input ledger.StateReference, index int) (request *promise.Promise[*StateWithMetadata], cancelRequest func()) {
			return m.cachedStateRequests.Compute(input.StateID(), func(request *promise.Promise[*StateWithMetadata], exists bool) *promise.Promise[*StateWithMetadata] {
				if !exists {
					request = m.requestState(input, true)
				}

				cancelRequest = lo.Batch(
					request.OnSuccess(func(input *StateWithMetadata) { transaction.publishInput(index, m.setupInput(transaction, input)) }),
					request.OnError(transaction.setInvalid),
				)

				return request
			}), cancelRequest
		}(input, i)

		transaction.OnEvicted(func() {
			if cancelRequest(); solidificationRequest.IsEmpty() {
				m.cachedStateRequests.Delete(input.StateID(), solidificationRequest.IsEmpty)
			}
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
	if transaction.AllInputsAccepted() && transaction.IsIncluded() && m.conflictDAG.AcceptanceState(transaction.conflictIDs).IsAccepted() {
		transaction.setAccepted()
	}
}

var _ mempool.MemPool[vote.MockedPower] = new(MemPool[vote.MockedPower])
