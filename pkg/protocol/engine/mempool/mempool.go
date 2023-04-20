package mempool

import (
	"iota-core/pkg/types"

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
	ledger types.Ledger
	vm     types.VM

	transactions      *shrinkingmap.ShrinkingMap[iotago.TransactionID, *TransactionMetadata]
	cachedOutputs     *shrinkingmap.ShrinkingMap[iotago.OutputID, *OutputMetadata]
	asyncLoadedEvents *shrinkingmap.ShrinkingMap[iotago.OutputID, *event.Event1[*OutputMetadata]]

	events *types.MemPoolEvents

	ioWorker         *workerpool.WorkerPool
	executionWorkers *workerpool.WorkerPool
	bookingWorkers   *workerpool.WorkerPool

	solidificationMutex *syncutils.DAGMutex[iotago.OutputID]
}

func New(ledgerInstance types.Ledger) *MemPool {
	return &MemPool{
		ledger:              ledgerInstance,
		events:              types.NewMemPoolEvents(),
		solidificationMutex: syncutils.NewDAGMutex[iotago.OutputID](),
	}
}

func (m *MemPool) ProcessTransaction(transaction types.Transaction) error {
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

func (m *MemPool) Output(id iotago.OutputID) *OutputMetadata {
	panic("implement me")
}

func (m *MemPool) storeTransaction(transaction types.Transaction) (metadata *TransactionMetadata, err error) {
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
			missingInputs: advancedset.New(lo.Map(inputs, types.Input.ID)...),
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
						Source:   transactionMetadata,
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
	interfaceOutputs := make([]types.OutputMetadata, len(rawOutputs))
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

func (m *MemPool) Events() *types.MemPoolEvents {
	return m.events
}

func (m *MemPool) ConflictDAG() interface{} {
	// TODO implement me
	panic("implement me")
}

var _ types.MemPool = new(MemPool)
