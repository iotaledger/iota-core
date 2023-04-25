package mempoolv1

import (
	"context"
	"sync/atomic"

	"github.com/pkg/errors"
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

	resolveInput mempool.StateReferenceResolver

	cachedTransactions *shrinkingmap.ShrinkingMap[iotago.TransactionID, *TransactionWithMetadata]

	cachedStates *shrinkingmap.ShrinkingMap[iotago.OutputID, *promise.Promise[*StateWithMetadata]]

	executionWorkers *workerpool.WorkerPool
}

func New(vm mempool.VM, inputResolver mempool.StateReferenceResolver, workers *workerpool.Group) *MemPool {
	m := &MemPool{
		events:                 mempool.NewEvents(),
		executeStateTransition: vm,
		resolveInput:           inputResolver,
		cachedTransactions:     shrinkingmap.New[iotago.TransactionID, *TransactionWithMetadata](),
		cachedStates:           shrinkingmap.New[iotago.OutputID, *promise.Promise[*StateWithMetadata]](),
		executionWorkers:       workers.CreatePool("executionWorkers", 1),
	}

	return m
}

func (m *MemPool) TransactionMetadata(id iotago.TransactionID) (metadata mempool.TransactionWithMetadata, exists bool) {
	return m.cachedTransactions.Get(id)
}

func (m *MemPool) ProcessTransaction(transaction mempool.Transaction) (metadata mempool.TransactionWithMetadata, err error) {
	newTransactionMetadata, err := NewTransactionMetadata(transaction)
	if err != nil {
		return nil, xerrors.Errorf("failed to create transaction metadata: %w", err)
	}

	transactionMetadata, stored := m.cachedTransactions.GetOrCreate(newTransactionMetadata.ID(), func() *TransactionWithMetadata {
		return newTransactionMetadata
	})

	if stored {
		m.events.TransactionStored.Trigger(transactionMetadata)

		m.loadInputs(transactionMetadata)
	}

	return transactionMetadata, nil
}

func (m *MemPool) SetTransactionInclusionSlot(id iotago.TransactionID, inclusionSlot iotago.SlotIndex) error {
	// TODO implement me
	panic("implement me")
}

func (m *MemPool) EvictTransaction(transactionID iotago.TransactionID) {
	if transaction, exists := m.cachedTransactions.Get(transactionID); exists && m.cachedTransactions.Delete(transactionID) {
		transaction.evicted.Trigger()
	}
}

func (m *MemPool) Events() *mempool.Events {
	return m.events
}

func (m *MemPool) ConflictDAG() interface{} {
	// TODO implement me
	panic("implement me")
}

func (m *MemPool) loadInputs(transactionMetadata *TransactionWithMetadata) {
	inputsToLoadCounter := uint64(len(transactionMetadata.inputReferences))

	requestInput := func(input mempool.StateReference, index int) (request *promise.Promise[*StateWithMetadata], cancelRequest func()) {
		return m.cachedStates.Compute(input.ReferencedStateID(), func(value *promise.Promise[*StateWithMetadata], exists bool) *promise.Promise[*StateWithMetadata] {
			if !exists {
				value = promise.New[*StateWithMetadata](func(p *promise.Promise[*StateWithMetadata]) {
					stateRequest := m.resolveInput(input)

					stateRequest.OnSuccess(func(state ledger.State) {
						p.Resolve(NewStateWithMetadata(state))
					})

					stateRequest.OnError(func(err error) {
						if !errors.Is(err, ledger.ErrStateNotFound) {
							p.Reject(err)
						}
					})
				})
			}

			cancelRequest = lo.Batch(
				value.OnSuccess(func(state *StateWithMetadata) {
					if transactionMetadata.publishInput(index, state); atomic.AddUint64(&inputsToLoadCounter, ^uint64(0)) == 0 {
						transactionMetadata.solid.Trigger()

						m.events.TransactionSolid.Trigger(transactionMetadata)

						m.executeTransaction(transactionMetadata)
					}
				}),
				value.OnError(transactionMetadata.invalid.Trigger),
			)

			return value
		}), cancelRequest
	}

	for i, input := range transactionMetadata.inputReferences {
		requester, cancel := requestInput(input, i)

		transactionMetadata.OnEvicted(func() {
			if cancel(); requester.IsEmpty() {
				m.cachedStates.Delete(input.ReferencedStateID(), requester.IsEmpty)
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

func (m *MemPool) bookTransaction(transactionMetadata *TransactionWithMetadata) {
	// determine the branches and inherit them to the outputs

	transactionMetadata.booked.Trigger()

	m.events.TransactionBooked.Trigger(transactionMetadata)

	for _, output := range transactionMetadata.outputs {
		lo.Return1(m.cachedStates.GetOrCreate(output.id, lo.NoVariadic(promise.New[*StateWithMetadata]))).Resolve(output)
	}
}

var _ mempool.MemPool = new(MemPool)
