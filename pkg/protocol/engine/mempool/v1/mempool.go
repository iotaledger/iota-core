package mempoolv1

import (
	"context"
	"errors"
	"sync/atomic"

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
		conflictDAG:            mockedconflictdag.NewConflictDAG[iotago.TransactionID, iotago.OutputID, VotePower](),
	}

	m.ConflictDAG().Events().ConflictAccepted.Hook(func(id iotago.TransactionID) {
		if transaction, exists := m.cachedTransactions.Get(id); !exists {
			m.tryToAccept(transaction)
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
	metadataToStore, err := NewTransactionMetadata(transaction)
	if err != nil {
		return nil, xerrors.Errorf("failed to create transaction metadata: %w", err)
	}

	metadata, stored := m.cachedTransactions.GetOrCreate(metadataToStore.ID(), func() *TransactionWithMetadata {
		return metadataToStore
	})

	if stored {
		m.events.TransactionStored.Trigger(metadataToStore)

		m.solidifyInputs(metadataToStore)
	}

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
		stateRequest = m.requestStateMetadata(stateReference)
	}

	stateRequest.OnSuccess(func(loadedState *StateWithMetadata) { state = loadedState })
	stateRequest.OnError(func(requestErr error) { err = xerrors.Errorf("failed to request state: %w", requestErr) })

	return state, err
}

func (m *MemPool[VotePower]) SetTransactionIncluded(id iotago.TransactionID, slot iotago.SlotIndex) error {
	transaction, exists := m.cachedTransactions.Get(id)
	if !exists {
		return xerrors.Errorf("transaction with id %s not found: %w", id, mempool.ErrTransactionNotFound)
	}

	switch previousSlot := transaction.setInclusionSlot(slot); true {
	case previousSlot == 0:
		m.tryToAccept(transaction)
	case previousSlot < slot:
		//transaction.triggerInclusionSlotUpdated(previousSlot, slot)
	}

	return nil
}

func (m *MemPool[VotePower]) solidifyInputs(transactionMetadata *TransactionWithMetadata) {
	// inputsToSolidify is used by solidifyInput to keep track of how many inputs are still missing to become solid.
	inputsToSolidify := uint64(len(transactionMetadata.inputReferences))

	// solidifyInput requests an input from the ledger and triggers the transaction solid event if all inputs are loaded.
	solidifyInput := func(input ledger.StateReference, index int) (request *promise.Promise[*StateWithMetadata], cancelRequest func()) {
		return m.cachedStateRequests.Compute(input.StateID(), func(request *promise.Promise[*StateWithMetadata], exists bool) *promise.Promise[*StateWithMetadata] {
			if !exists {
				request = m.requestStateMetadata(input, true)
			}

			cancelRequest = lo.Batch(
				request.OnSuccess(func(state *StateWithMetadata) {
					state.accepted.OnTrigger(func() {
						if transactionMetadata.decreaseUnacceptedInputsCount() == 0 {
							m.tryToAccept(transactionMetadata)
						}
					})

					if transactionMetadata.publishInput(index, state); atomic.AddUint64(&inputsToSolidify, ^uint64(0)) == 0 {
						transactionMetadata.solid.Trigger()

						m.events.TransactionSolid.Trigger(transactionMetadata)

						m.executeTransaction(transactionMetadata)
					}
				}),

				request.OnError(transactionMetadata.invalid.Trigger),
			)

			return request
		}), cancelRequest
	}

	for i, input := range transactionMetadata.inputReferences {
		solidificationRequest, cancelRequest := solidifyInput(input, i)

		transactionMetadata.OnEvicted(func() {
			if cancelRequest(); solidificationRequest.IsEmpty() {
				m.cachedStateRequests.Delete(input.StateID(), solidificationRequest.IsEmpty)
			}
		})
	}
}

func (m *MemPool[VotePower]) tryToAccept(transactionMetadata *TransactionWithMetadata) {
	if !transactionMetadata.AllInputsAccepted() || !transactionMetadata.IsIncluded() || !m.ConflictDAG().AcceptanceState(transactionMetadata.conflictIDs).IsAccepted() {
		return
	}

	if transactionMetadata.setAccepted() {
		m.events.TransactionAccepted.Trigger(transactionMetadata)
	}
}

func (m *MemPool[VotePower]) executeTransaction(transactionMetadata *TransactionWithMetadata) {
	m.executionWorkers.Submit(func() {
		outputStates, err := m.executeStateTransition(transactionMetadata.Transaction(), lo.Map(transactionMetadata.inputs, (*StateWithMetadata).State), context.Background())
		if err != nil {
			transactionMetadata.triggerInvalid(err)

			m.events.TransactionInvalid.Trigger(transactionMetadata, err)

			return
		}

		transactionMetadata.publishExecutionResult(outputStates)

		m.events.TransactionExecuted.Trigger(transactionMetadata)

		m.bookTransaction(transactionMetadata)
	})
}

func (m *MemPool[VotePower]) bookTransaction(transaction *TransactionWithMetadata) {
	// TODO: determine the branches and inherit them to the outputs

	transaction.triggerBooked()

	m.events.TransactionBooked.Trigger(transaction)

	m.publishOutputsToCache(transaction)
}

func (m *MemPool[VotePower]) publishOutputsToCache(transaction *TransactionWithMetadata) {
	for _, output := range transaction.outputs {
		lo.Return1(m.cachedStateRequests.GetOrCreate(output.id, lo.NoVariadic(promise.New[*StateWithMetadata]))).Resolve(output)
	}
}

// requestStateMetadata requests the state with the given reference from the ledger.
func (m *MemPool[VotePower]) requestStateMetadata(stateReference ledger.StateReference, waitIfMissing ...bool) *promise.Promise[*StateWithMetadata] {
	return promise.New[*StateWithMetadata](func(p *promise.Promise[*StateWithMetadata]) {
		request := m.requestInput(stateReference)

		request.OnSuccess(func(state ledger.State) {
			p.Resolve(NewStateWithMetadata(state))
		})

		request.OnError(func(err error) {
			if lo.First(waitIfMissing) && errors.Is(err, ledger.ErrStateNotFound) {
				return
			}

			p.Reject(err)
		})
	})
}

var _ mempool.MemPool[vote.MockedPower] = new(MemPool[vote.MockedPower])
