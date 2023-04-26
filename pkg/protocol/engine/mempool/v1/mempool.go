package mempoolv1

import (
	"context"
	"errors"
	"sync/atomic"

	"golang.org/x/xerrors"
	"iota-core/pkg/promise"
	"iota-core/pkg/protocol/engine/ledger"
	"iota-core/pkg/protocol/engine/mempool"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type MemPool struct {
	events *mempool.Events

	executeStateTransition mempool.VM

	requestInput ledger.StateReferenceResolver

	cachedTransactions *shrinkingmap.ShrinkingMap[iotago.TransactionID, *TransactionWithMetadata]

	cachedStateRequests *shrinkingmap.ShrinkingMap[iotago.OutputID, *promise.Promise[*StateWithMetadata]]

	executionWorkers *workerpool.WorkerPool
}

func New(vm mempool.VM, inputResolver ledger.StateReferenceResolver, workers *workerpool.Group) *MemPool {
	m := &MemPool{
		events:                 mempool.NewEvents(),
		executeStateTransition: vm,
		requestInput:           inputResolver,
		cachedTransactions:     shrinkingmap.New[iotago.TransactionID, *TransactionWithMetadata](),
		cachedStateRequests:    shrinkingmap.New[iotago.OutputID, *promise.Promise[*StateWithMetadata]](),
		executionWorkers:       workers.CreatePool("executionWorkers", 1),
	}

	return m
}

func (m *MemPool) Events() *mempool.Events {
	return m.events
}

func (m *MemPool) ConflictDAG() interface{} {
	// TODO implement me
	panic("implement me")
}

func (m *MemPool) AddTransaction(transaction mempool.Transaction) (metadata mempool.TransactionWithMetadata, err error) {
	newTransactionMetadata, err := NewTransactionMetadata(transaction)
	if err != nil {
		return nil, xerrors.Errorf("failed to create transaction metadata: %w", err)
	}

	transactionMetadata, stored := m.cachedTransactions.GetOrCreate(newTransactionMetadata.ID(), func() *TransactionWithMetadata {
		return newTransactionMetadata
	})

	if stored {
		m.events.TransactionStored.Trigger(transactionMetadata)

		m.solidifyInputs(transactionMetadata)
	}

	return transactionMetadata, nil
}

func (m *MemPool) RemoveTransaction(transactionID iotago.TransactionID) {
	if transaction, deleted := m.cachedTransactions.DeleteAndReturn(transactionID); deleted {
		transaction.evicted.Trigger()
	}
}

func (m *MemPool) Transaction(id iotago.TransactionID) (transaction mempool.TransactionWithMetadata, exists bool) {
	return m.cachedTransactions.Get(id)
}

func (m *MemPool) State(stateReference ledger.StateReference) (state mempool.StateWithMetadata, err error) {
	stateRequest, exists := m.cachedStateRequests.Get(stateReference.StateID())
	if !exists || !stateRequest.WasCompleted() {
		stateRequest = m.requestStateMetadata(stateReference)
	}

	stateRequest.OnSuccess(func(loadedState *StateWithMetadata) { state = loadedState })
	stateRequest.OnError(func(requestErr error) { err = xerrors.Errorf("failed to request state: %w", requestErr) })

	return state, err
}

func (m *MemPool) SetTransactionIncluded(id iotago.TransactionID, inclusionSlot iotago.SlotIndex) error {
	// TODO implement me
	panic("implement me")
}

func (m *MemPool) solidifyInputs(transactionMetadata *TransactionWithMetadata) {
	// inputsToSolidify is used by solidify to keep track of how many inputs are still missing to become solid.
	inputsToSolidify := uint64(len(transactionMetadata.inputReferences))

	// solidify requests an input from the ledger and triggers the transaction solid event if all inputs are loaded.
	solidify := func(input ledger.StateReference, index int) (request *promise.Promise[*StateWithMetadata], cancelRequest func()) {
		return m.cachedStateRequests.Compute(input.StateID(), func(request *promise.Promise[*StateWithMetadata], exists bool) *promise.Promise[*StateWithMetadata] {
			if !exists {
				request = m.requestStateMetadata(input, true)
			}

			cancelRequest = lo.Batch(
				request.OnSuccess(func(state *StateWithMetadata) {
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
		solidificationRequest, cancelRequest := solidify(input, i)

		transactionMetadata.OnEvicted(func() {
			if cancelRequest(); solidificationRequest.IsEmpty() {
				m.cachedStateRequests.Delete(input.StateID(), solidificationRequest.IsEmpty)
			}
		})
	}
}

func (m *MemPool) executeTransaction(transactionMetadata *TransactionWithMetadata) {
	m.executionWorkers.Submit(func() {
		outputStates, err := m.executeStateTransition(transactionMetadata.Transaction(), lo.Map(transactionMetadata.inputs, (*StateWithMetadata).State), context.Background())
		if err != nil {
			transactionMetadata.invalid.Trigger(err)
			m.events.TransactionInvalid.Trigger(transactionMetadata, err)

			return
		}

		transactionMetadata.publishExecutionResult(outputStates)

		m.events.TransactionExecuted.Trigger(transactionMetadata)

		m.bookTransaction(transactionMetadata)
	})
}

func (m *MemPool) bookTransaction(transaction *TransactionWithMetadata) {
	// TODO: determine the branches and inherit them to the outputs

	transaction.triggerBooked()

	m.events.TransactionBooked.Trigger(transaction)

	m.publishOutputsToCache(transaction)
}

func (m *MemPool) publishOutputsToCache(transaction *TransactionWithMetadata) {
	for _, output := range transaction.outputs {
		lo.Return1(m.cachedStateRequests.GetOrCreate(output.id, lo.NoVariadic(promise.New[*StateWithMetadata]))).Resolve(output)
	}
}

// requestStateMetadata requests the state with the given reference from the ledger.
func (m *MemPool) requestStateMetadata(stateReference ledger.StateReference, waitIfMissing ...bool) *promise.Promise[*StateWithMetadata] {
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

var _ mempool.MemPool = new(MemPool)
