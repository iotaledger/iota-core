package mempoolv1

import (
	"context"
	"sync/atomic"

	"github.com/pkg/errors"
	"iota-core/pkg/protocol/engine/ledger"
	"iota-core/pkg/protocol/engine/mempool"
	"iota-core/pkg/protocol/engine/mempool/promise"
	"iota-core/pkg/protocol/engine/vm"

	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type MemPool struct {
	executeStateTransition vm.VM

	resolveInput mempool.StateReferenceResolver

	events *mempool.Events

	transactions *shrinkingmap.ShrinkingMap[iotago.TransactionID, *TransactionMetadata]

	cachedStates *shrinkingmap.ShrinkingMap[iotago.OutputID, *promise.Promise[*StateMetadata]]

	executionWorkers *workerpool.WorkerPool

	bookingWorkers *workerpool.WorkerPool
}

func New(vm vm.VM, inputResolver mempool.StateReferenceResolver, workers *workerpool.Group) *MemPool {
	m := &MemPool{
		executeStateTransition: vm,
		resolveInput:           inputResolver,
		events:                 mempool.NewEvents(),
		transactions:           shrinkingmap.New[iotago.TransactionID, *TransactionMetadata](),
		cachedStates:           shrinkingmap.New[iotago.OutputID, *promise.Promise[*StateMetadata]](),
		executionWorkers:       workers.CreatePool("executionWorkers", 1),
		bookingWorkers:         workers.CreatePool("bookingWorkers", 1),
	}

	return m
}

func (m *MemPool) ProcessTransaction(transaction mempool.Transaction) error {
	transactionMetadata, err := NewTransactionMetadata(transaction)
	if err != nil {
		return xerrors.Errorf("failed to create transaction metadata: %w", err)
	}

	if m.transactions.Set(transactionMetadata.ID(), transactionMetadata) {
		m.events.TransactionStored.Trigger(transactionMetadata)

		m.resolveInputs(transactionMetadata)
	}

	return nil
}

func (m *MemPool) SetTransactionInclusionSlot(id iotago.TransactionID, inclusionSlot iotago.SlotIndex) error {
	// TODO implement me
	panic("implement me")
}

func (m *MemPool) EvictTransaction(id iotago.TransactionID) error {
	// TODO implement me
	panic("implement me")
}

func (m *MemPool) Events() *mempool.Events {
	return m.events
}

func (m *MemPool) ConflictDAG() interface{} {
	// TODO implement me
	panic("implement me")
}

func (m *MemPool) resolveInputs(transactionMetadata *TransactionMetadata) {
	missingInputs := uint64(len(transactionMetadata.inputReferences))

	for i, input := range transactionMetadata.inputReferences {
		inputRequest, inputRequestCreated := m.cachedStates.GetOrCreate(input.ReferencedStateID(), promise.New[*StateMetadata])
		inputRequest.OnSuccess(func(state *StateMetadata) {
			if transactionMetadata.PublishInput(i, state); atomic.AddUint64(&missingInputs, ^uint64(0)) == 0 {
				m.events.TransactionSolid.Trigger(transactionMetadata)

				m.executionWorkers.Submit(func() { m.executeTransaction(transactionMetadata) })
			}
		}).OnError(func(err error) {
			// TODO: MARK TRANSACTION AS UNSOLIDIFIABLE AND CLEAN UP
		})

		if inputRequestCreated {
			m.resolveInput(input).OnSuccess(func(state vm.State) {
				inputRequest.Resolve(NewStateMetadata(state))
			}).OnError(func(err error) {
				if errors.Is(err, ledger.ErrStateNotFound) {
					// TODO: DELAYED inputRequest.Reject(err) / WAIT FOR SOLIDIFICATION (BUT NOT FOREVER)
				} else {
					inputRequest.Reject(err)
				}
			})
		}
	}
}

func (m *MemPool) executeTransaction(transactionMetadata *TransactionMetadata) {
	outputStates, executionErr := m.executeStateTransition(transactionMetadata.Transaction, lo.Map(transactionMetadata.inputs, (*StateMetadata).State), context.Background())
	if executionErr != nil {
		m.events.TransactionExecutionFailed.Trigger(transactionMetadata, executionErr)

		return
	}

	transactionMetadata.PublishOutputStates(outputStates)

	m.events.TransactionExecuted.Trigger(transactionMetadata)

	m.bookingWorkers.Submit(func() { m.bookTransaction(transactionMetadata) })
}

func (m *MemPool) bookTransaction(transactionMetadata *TransactionMetadata) {
	// determine the branches and inherit them to the outputs

	for _, output := range transactionMetadata.outputs {
		lo.Return1(m.cachedStates.GetOrCreate(output.ID, promise.New[*StateMetadata])).Resolve(output)
	}

	m.events.TransactionBooked.Trigger(transactionMetadata)
}

var _ mempool.MemPool = new(MemPool)
