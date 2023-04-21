package mempoolv1

import (
	types2 "iota-core/pkg/protocol/engine/ledger"
	"iota-core/pkg/protocol/engine/mempool"

	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type MemPool struct {
	ledger types2.Ledger
	vm     mempool.VM

	transactions      *shrinkingmap.ShrinkingMap[mempool.TransactionID, *TransactionMetadata]
	cachedOutputs     *shrinkingmap.ShrinkingMap[types2.OutputID, *OutputMetadata]
	asyncLoadedEvents *shrinkingmap.ShrinkingMap[types2.OutputID, *event.Event1[*OutputMetadata]]

	events *mempool.MemPoolEvents

	ioWorker         *workerpool.WorkerPool
	executionWorkers *workerpool.WorkerPool
	bookingWorkers   *workerpool.WorkerPool

	solidificationMutex *syncutils.DAGMutex[types2.OutputID]
}

func New(vm mempool.VM, ledgerInstance types2.Ledger, workers *workerpool.Group) *MemPool {
	return &MemPool{
		transactions:      shrinkingmap.New[mempool.TransactionID, *TransactionMetadata](),
		cachedOutputs:     shrinkingmap.New[types2.OutputID, *OutputMetadata](),
		asyncLoadedEvents: shrinkingmap.New[types2.OutputID, *event.Event1[*OutputMetadata]](),

		ioWorker:         workers.CreatePool("ioWorker", 1),
		executionWorkers: workers.CreatePool("executionWorkers", 1),
		bookingWorkers:   workers.CreatePool("bookingWorkers", 1),

		ledger:              ledgerInstance,
		vm:                  vm,
		events:              mempool.NewMemPoolEvents(),
		solidificationMutex: syncutils.NewDAGMutex[types2.OutputID](),
	}
}

func (m *MemPool) ProcessTransaction(transaction mempool.Transaction) error {
	transactionMetadata, err := m.storeTransaction(transaction)
	if err != nil {
		return xerrors.Errorf("failed to store transaction: %w", err)
	}

	if !m.checkSolidity(transactionMetadata) {
		return xerrors.Errorf("transaction %s is not solid", transactionMetadata.id)
	}

	m.executionWorkers.Submit(func() { m.executeTransaction(transactionMetadata) })

	return nil
}

func (m *MemPool) Output(id types2.OutputID) (types2.Output, bool) {
	m.solidificationMutex.RLock(id)
	defer m.solidificationMutex.RUnlock(id)

	output, exists := m.cachedOutputs.Get(id)
	if exists {
		return output.Output(), true
	}

	return m.ledger.Output(id)
}

// storeTransaction stores the given transaction in the MemPool and returns the corresponding metadata.
func (m *MemPool) storeTransaction(transaction mempool.Transaction) (metadata *TransactionMetadata, err error) {
	transactionID, err := transaction.ID()
	if err != nil {
		return nil, xerrors.Errorf("failed to retrieve transaction ID: %w", err)
	}

	inputs, err := transaction.Inputs()
	if err != nil {
		return nil, xerrors.Errorf("failed to retrieve inputs of transaction %s: %w", transactionID, err)
	}

	metadata, isNew := m.transactions.GetOrCreate(transactionID, func() *TransactionMetadata {
		return &TransactionMetadata{
			id:            transactionID,
			missingInputs: advancedset.New(lo.Map(inputs, mempool.Input.ID)...),
			inputs:        make([]*OutputMetadata, len(inputs)),
		}
	})

	if !isNew && !metadata.IsBooked() {
		return metadata, xerrors.Errorf("transaction %s is an un-booked reattachment", metadata.id)
	}

	if isNew {
		m.events.TransactionStored.Trigger(metadata)
	}

	return metadata, err
}

func (m *MemPool) checkSolidity(transactionMetadata *TransactionMetadata) (isSolid bool) {
	loadOutput := func(outputID iotago.OutputID, index int) (output *OutputMetadata, exists bool) {
		m.solidificationMutex.RLock(outputID)
		defer m.solidificationMutex.RUnlock(outputID)

		// try to load the output from the cache
		if output, exists = m.cachedOutputs.Get(outputID); exists {
			return output, true
		}

		// create / subscribe to the output loaded event
		_, eventCreated := m.asyncLoadedEvents.GetOrCreate(outputID, func() *event.Event1[*OutputMetadata] {
			outputLoadedEvent := event.New1[*OutputMetadata](event.WithMaxTriggerCount(1))
			outputLoadedEvent.Hook(func(output *OutputMetadata) {
				if transactionMetadata.PublishInput(index, output) {
					m.executionWorkers.Submit(func() { m.executeTransaction(transactionMetadata) })
				}
			})

			return outputLoadedEvent
		})

		// try to load the output from the ledger asynchronously (only if the event was created / once)
		if eventCreated {
			m.ioWorker.Submit(func() {
				if output, exists := m.ledger.Output(outputID); exists {
					m.publishOutput(&OutputMetadata{
						ID:       outputID,
						Source:   nil,
						Spenders: advancedset.New[*TransactionMetadata](),
						output:   output,
					})
				}
			})
		}

		return nil, false
	}

	i := 0
	transactionMetadata.missingInputs.Range(func(inputID iotago.OutputID) {
		if input, loaded := loadOutput(inputID, i); loaded {
			isSolid = transactionMetadata.PublishInput(i, input) || isSolid
		}

		i++
	})

	if isSolid {
		m.events.TransactionSolid.Trigger(transactionMetadata)
	}

	return isSolid
}

func (m *MemPool) publishOutput(output *OutputMetadata) {
	if asyncLoadedEvent, trigger := func() (*event.Event1[*OutputMetadata], bool) {
		m.solidificationMutex.Lock(output.ID)
		defer m.solidificationMutex.Unlock(output.ID)

		if m.cachedOutputs.Set(output.ID, output) {
			if asyncLoadedEvent, exists := m.asyncLoadedEvents.Get(output.ID); exists {
				return asyncLoadedEvent, m.asyncLoadedEvents.Delete(output.ID)
			}
		}

		return nil, false
	}(); trigger {
		asyncLoadedEvent.Trigger(output)
	}
}

func (m *MemPool) executeTransaction(transactionMetadata *TransactionMetadata) {
	rawOutputs, err := m.vm(transactionMetadata.Transaction, lo.Map(transactionMetadata.inputs, (*OutputMetadata).Output), 0)
	if err != nil {
		m.events.TransactionExecutionFailed.Trigger(transactionMetadata, err)

		return
	}

	outputs := make([]*OutputMetadata, len(rawOutputs))
	interfaceOutputs := make([]types2.OutputMetadata, len(rawOutputs))
	for i, rawOutput := range rawOutputs {
		output := &OutputMetadata{
			ID:       iotago.OutputIDFromTransactionIDAndIndex(transactionMetadata.id, uint16(i)),
			Source:   transactionMetadata,
			Spenders: advancedset.New[*TransactionMetadata](),
			output:   rawOutput,
		}

		outputs[i] = output
		interfaceOutputs[i] = output
	}

	m.events.TransactionExecuted.Trigger(transactionMetadata, interfaceOutputs)

	m.bookingWorkers.Submit(func() { m.bookTransaction(transactionMetadata, outputs) })
}

func (m *MemPool) bookTransaction(transactionMetadata *TransactionMetadata, outputs []*OutputMetadata) {
	// determine the branches and inherit them to the outputs

	lo.ForEach(outputs, m.publishOutput)

	m.events.TransactionBooked.Trigger(transactionMetadata)
}

func (m *MemPool) SetTransactionInclusionSlot(id iotago.TransactionID, inclusionSlot iotago.SlotIndex) error {
	// TODO implement me
	panic("implement me")
}

func (m *MemPool) EvictTransaction(id iotago.TransactionID) error {
	// TODO implement me
	panic("implement me")
}

func (m *MemPool) Events() *mempool.MemPoolEvents {
	return m.events
}

func (m *MemPool) ConflictDAG() interface{} {
	// TODO implement me
	panic("implement me")
}

var _ mempool.MemPool = new(MemPool)
