package mempoolv1

import (
	"context"
	"sync"

	"iota-core/pkg/protocol/engine/mempool"

	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type MemPool struct {
	transactions   *shrinkingmap.ShrinkingMap[iotago.TransactionID, *Transaction]
	cachedOutputs  *shrinkingmap.ShrinkingMap[iotago.OutputID, *Output]
	missingOutputs *shrinkingmap.ShrinkingMap[iotago.OutputID, *event.Event1[*Output]]

	events *mempool.Events

	ledgerStateWorker workerpool.WorkerPool

	cachedOutputsMutex sync.RWMutex
}

func NewMemPool() *MemPool {
	return &MemPool{
		events: mempool.NewEvents(),
	}
}

func (m *MemPool) Events() *mempool.Events {
	return m.events
}

func (m *MemPool) ProcessTransaction(tx mempool.Transaction, ctx context.Context) error {
	txMetadata, isNew, err := m.storeTransaction(tx)
	if err != nil {
		return xerrors.Errorf("failed to store transaction: %w", err)
	} else if !isNew {
		return lo.Cond(txMetadata.outputs == nil, xerrors.Errorf("transaction %s is an unprocessed reattachment", txMetadata.id), nil)
	}

	// Trigger stored event

	if !m.allInputsAvailable(txMetadata) {
		return xerrors.Errorf("transaction %s is missing inputs", txMetadata.id)
	}

	// Trigger solid event

	// push to execution queue

	return nil
}

func (m *MemPool) Output(id iotago.OutputID) *Output {
	panic("implement me")
}

// storeTransaction is a ChainedCommand that stores a Transaction.
func (m *MemPool) storeTransaction(tx mempool.Transaction) (txMetadata *Transaction, isNew bool, err error) {
	txID, err := tx.ID()
	if err != nil {
		return nil, false, xerrors.Errorf("failed to retrieve transaction ID: %w", err)
	}

	inputReferences, err := tx.Inputs()
	if err != nil {
		return nil, false, xerrors.Errorf("failed to retrieve inputs of transaction %s: %w", txID, err)
	}

	txMetadata, isNew = m.transactions.GetOrCreate(txID, func() *Transaction {
		return &Transaction{
			id:            txID,
			missingInputs: advancedset.New(lo.Map(inputReferences, mempool.Input.ID)...),
			inputs:        make([]*Output, len(inputReferences)),
		}
	})

	return txMetadata, isNew, nil
}

func (m *MemPool) allInputsAvailable(txMetadata *Transaction) (allInputsAvailable bool) {
	var i int
	_ = txMetadata.missingInputs.ForEach(func(inputID iotago.OutputID) error {
		if input, exists := m.loadOutput(i, inputID, txMetadata); exists {
			allInputsAvailable = txMetadata.PublishInput(i, input) || allInputsAvailable
		}

		i++

		return nil
	})

	return allInputsAvailable
}

func (m *MemPool) sthOld(i int, inputID iotago.OutputID, txMetadata *Transaction) (*Output, bool) {

	if !allInputsAvailable {
		return xerrors.Errorf("transaction %s is missing inputs", txID)
	}

	inputs := make([]*Output, len(inputReferences))
	for i, inputReference := range inputReferences {
		inputs[i], _ = s.cachedOutputs.GetOrCreate(inputReference.ID(), func() *Output {
			return &Output{
				Source:   nil,
				Spenders: advancedset.New[*Transaction](),
				Content:  nil,
			}
		})
	}

	s.transactions.GetOrCreate(txID, func() *Transaction {

		return &Transaction{
			inputs:  nil,
			outputs: nil,
			Content: tx,
		}
	})

	created := false
	cachedTransactionMetadata := s.CachedTransactionMetadata(params.Transaction.ID(), func(txID utxo.TransactionID) *mempool.TransactionMetadata {
		s.transactionStorage.Store(params.Transaction).Release()
		created = true
		return mempool.NewTransactionMetadata(txID)
	})
	defer cachedTransactionMetadata.Release()

	params.TransactionMetadata, _ = cachedTransactionMetadata.Unwrap()

	if !created {
		if params.TransactionMetadata.IsBooked() {
			return nil
		}

		return errors.WithMessagef(mempool.ErrTransactionUnsolid, "%s is an unsolid reattachment", params.Transaction.ID())
	}

	params.InputIDs = s.ledger.Utils().ResolveInputs(params.Transaction.Inputs())

	cachedConsumers := s.initConsumers(params.InputIDs, params.Transaction.ID())
	defer cachedConsumers.Release()
	params.Consumers = cachedConsumers.Unwrap(true)

	s.ledger.events.TransactionStored.Trigger(&mempool.TransactionStoredEvent{
		TransactionID: params.Transaction.ID(),
	})

	return next(params)
}

func (m *MemPool) loadOutput(index int, id iotago.OutputID, txMetadata *Transaction) (output *Output, exists bool) {
	m.cachedOutputsMutex.RLock()
	defer m.cachedOutputsMutex.RUnlock()

	if output, exists = m.cachedOutputs.Get(id); exists {
		return output, true
	}

	missingOutputLoadedEvent, created := m.missingOutputs.GetOrCreate(id, func() *event.Event1[*Output] {
		return event.New1[*Output](event.WithMaxTriggerCount(1))
	})

	missingOutputLoadedEvent.Hook(func(output *Output) {
		if txMetadata.PublishInput(index, output) {
			// push to execution queue
		}
	})

	if created {
		m.ledgerStateWorker.Submit(func() {
			if output, exists := m.loadOutputFromDisk(id); exists {
				m.cachedOutputsMutex.Lock()
				defer m.cachedOutputsMutex.Unlock()

				if m.cachedOutputs.Set(id, output) && m.missingOutputs.Delete(id) {
					missingOutputLoadedEvent.Trigger(output)
				}
			}
		})
	}

	return nil, true
}

func (m *MemPool) SetTransactionInclusionSlot(id iotago.TransactionID, inclusionSlot iotago.SlotIndex) error {
	// TODO implement me
	panic("implement me")
}

func (m *MemPool) EvictTransaction(id iotago.TransactionID) error {
	// TODO implement me
	panic("implement me")
}

func (m *MemPool) ConflictDAG() interface{} {
	// TODO implement me
	panic("implement me")
}

var _ mempool.MemPool = new(MemPool)
