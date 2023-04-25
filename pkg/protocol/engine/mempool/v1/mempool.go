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
	executeStateTransition mempool.VM

	resolveInput mempool.StateReferenceResolver

	events *mempool.Events

	cachedTransactions *shrinkingmap.ShrinkingMap[iotago.TransactionID, *TransactionWithMetadata]

	cachedStates *shrinkingmap.ShrinkingMap[iotago.OutputID, *promise.Promise[*StateWithMetadata]]

	executionWorkers *workerpool.WorkerPool

	bookingWorkers *workerpool.WorkerPool
}

func New(vm mempool.VM, inputResolver mempool.StateReferenceResolver, workers *workerpool.Group) *MemPool {
	m := &MemPool{
		executeStateTransition: vm,
		resolveInput:           inputResolver,
		events:                 mempool.NewEvents(),
		cachedTransactions:     shrinkingmap.New[iotago.TransactionID, *TransactionWithMetadata](),
		cachedStates:           shrinkingmap.New[iotago.OutputID, *promise.Promise[*StateWithMetadata]](),
		executionWorkers:       workers.CreatePool("executionWorkers", 1),
		bookingWorkers:         workers.CreatePool("bookingWorkers", 1),
	}

	return m
}

func (m *MemPool) TransactionMetadata(id iotago.TransactionID) (metadata mempool.TransactionWithMetadata, exists bool) {
	return m.cachedTransactions.Get(id)
}

func (m *MemPool) ProcessTransaction(transaction mempool.Transaction) error {
	transactionMetadata, err := NewTransactionMetadata(transaction)
	if err != nil {
		return xerrors.Errorf("failed to create transaction metadata: %w", err)
	}

	if m.cachedTransactions.Set(transactionMetadata.ID(), transactionMetadata) {
		m.events.TransactionStored.Trigger(transactionMetadata)

		m.resolveInputs(transactionMetadata)
	}

	return nil
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

func (m *MemPool) resolveInputs(transactionMetadata *TransactionWithMetadata) {
	missingInputs := uint64(len(transactionMetadata.inputReferences))

	for i, input := range transactionMetadata.inputReferences {
		var cancelRequest context.CancelFunc

		stateRequest := m.cachedStates.Compute(input.ReferencedStateID(), func(value *promise.Promise[*StateWithMetadata], exists bool) *promise.Promise[*StateWithMetadata] {
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
					if transactionMetadata.publishInput(i, state); atomic.AddUint64(&missingInputs, ^uint64(0)) == 0 {
						transactionMetadata.solid.Trigger()
						m.events.TransactionSolid.Trigger(transactionMetadata)

						m.executionWorkers.Submit(func() { m.executeTransaction(transactionMetadata) })
					}
				}),
				value.OnError(transactionMetadata.invalid.Trigger),
			)

			return value
		})

		transactionMetadata.HookEvicted(func() {
			cancelRequest()

			if stateRequest.IsEmpty() {
				m.cachedStates.Delete(input.ReferencedStateID(), stateRequest.IsEmpty)
			}
		})
	}
}

func (m *MemPool) executeTransaction(transactionMetadata *TransactionWithMetadata) {
	outputStates, executionErr := m.executeStateTransition(transactionMetadata.Transaction(), lo.Map(transactionMetadata.inputs, (*StateWithMetadata).State), context.Background())
	if executionErr != nil {
		m.events.TransactionExecutionFailed.Trigger(transactionMetadata, executionErr)

		return
	}

	transactionMetadata.publishOutputStates(outputStates)
	transactionMetadata.executed.Trigger()

	m.events.TransactionExecuted.Trigger(transactionMetadata)

	m.bookingWorkers.Submit(func() { m.bookTransaction(transactionMetadata) })
}

func (m *MemPool) bookTransaction(transactionMetadata *TransactionWithMetadata) {
	// determine the branches and inherit them to the outputs

	for _, output := range transactionMetadata.outputs {
		lo.Return1(m.cachedStates.GetOrCreate(output.id, lo.NoVariadic(promise.New[*StateWithMetadata]))).Resolve(output)
	}

	transactionMetadata.booked.Trigger()

	m.events.TransactionBooked.Trigger(transactionMetadata)
}

var _ mempool.MemPool = new(MemPool)
