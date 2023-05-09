package mempoolv1

import (
	"sync"
	"sync/atomic"

	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TransactionWithMetadata struct {
	id              iotago.TransactionID
	inputReferences []ledger.StateReference
	inputs          []*StateWithMetadata
	outputs         []*StateWithMetadata
	transaction     mempool.Transaction
	attachments     *Attachments
	conflictIDs     *advancedset.AdvancedSet[iotago.TransactionID]

	unsolidInputsCount    uint64
	unacceptedInputsCount uint64
	allInputsAccepted     *promise.Event

	inclusion *InclusionState
	lifecycle *LifecycleState

	mutex sync.RWMutex
}

func NewTransactionWithMetadata(transaction mempool.Transaction) (*TransactionWithMetadata, error) {
	transactionID, transactionIDErr := transaction.ID()
	if transactionIDErr != nil {
		return nil, xerrors.Errorf("failed to retrieve transaction ID: %w", transactionIDErr)
	}

	inputReferences, inputsErr := transaction.Inputs()
	if inputsErr != nil {
		return nil, xerrors.Errorf("failed to retrieve inputReferences of transaction %s: %w", transactionID, inputsErr)
	}

	t := &TransactionWithMetadata{
		id:              transactionID,
		inputReferences: inputReferences,
		inputs:          make([]*StateWithMetadata, len(inputReferences)),
		transaction:     transaction,
		conflictIDs:     advancedset.New[iotago.TransactionID](),
		attachments:     NewAttachments(),

		inclusion: NewInclusionState(),
		lifecycle: NewLifecycleState(),

		allInputsAccepted: promise.NewEvent(),

		unsolidInputsCount:    uint64(len(inputReferences)),
		unacceptedInputsCount: uint64(len(inputReferences)),
	}

	t.attachments.OnAllAttachmentsEvicted(func() {
		if !t.inclusion.IsCommitted() {
			t.inclusion.setEvicted()
		}
	})

	return t, nil
}

func (t *TransactionWithMetadata) ID() iotago.TransactionID {
	return t.id
}

func (t *TransactionWithMetadata) Transaction() mempool.Transaction {
	return t.transaction
}

func (t *TransactionWithMetadata) Inputs() *advancedset.AdvancedSet[mempool.StateWithMetadata] {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	inputs := advancedset.New[mempool.StateWithMetadata]()
	for _, input := range t.inputs {
		inputs.Add(input)
	}

	return inputs
}

func (t *TransactionWithMetadata) Outputs() *advancedset.AdvancedSet[mempool.StateWithMetadata] {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	outputs := advancedset.New[mempool.StateWithMetadata]()
	for _, output := range t.outputs {
		outputs.Add(output)
	}

	return outputs
}

func (t *TransactionWithMetadata) Attachments() *Attachments {
	return t.attachments
}

// region Attachments //////////////////////////////////////////////////////////////////////////////////////////////////

func (t *TransactionWithMetadata) OnEarliestIncludedSlotUpdated(callback func(prevIndex, newIndex iotago.SlotIndex)) (unsubscribe func()) {
	return t.attachments.earliestIncludedSlot.OnUpdate(callback)
}

func (t *TransactionWithMetadata) MarkIncluded(blockID iotago.BlockID) bool {
	return t.attachments.MarkIncluded(blockID)
}

func (t *TransactionWithMetadata) MarkOrphaned(blockID iotago.BlockID) bool {
	return t.attachments.MarkOrphaned(blockID)
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region InclusionState ///////////////////////////////////////////////////////////////////////////////////////////////

func (t *TransactionWithMetadata) AllInputsAccepted() bool {
	return t.allInputsAccepted.WasTriggered()
}

func (t *TransactionWithMetadata) OnAllInputsAccepted(callback func()) {
	t.allInputsAccepted.OnTrigger(callback)
}

func (t *TransactionWithMetadata) Inclusion() mempool.InclusionState {
	return t.inclusion
}

func (t *TransactionWithMetadata) SetCommitted() {
	t.inclusion.setCommitted()
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region Lifecycle ////////////////////////////////////////////////////////////////////////////////////////////////////

func (t *TransactionWithMetadata) Lifecycle() mempool.LifecycleState {
	return t.lifecycle
}

func (t *TransactionWithMetadata) publishInput(index int, input *StateWithMetadata) {
	t.inputs[index] = input

	t.setupInputLifecycle(input)

	if atomic.AddUint64(&t.unsolidInputsCount, ^uint64(0)) == 0 {
		t.lifecycle.setSolid()
	}
}

func (t *TransactionWithMetadata) setExecuted(outputStates []ledger.State) {
	t.mutex.Lock()
	for _, outputState := range outputStates {
		t.outputs = append(t.outputs, NewStateWithMetadata(outputState, t))
	}
	t.mutex.Unlock()

	t.lifecycle.executed.Trigger()
}

func (t *TransactionWithMetadata) setupInputLifecycle(input *StateWithMetadata) {
	input.spentState.increaseSpenderCount()

	t.inclusion.OnAccepted(func() {
		input.spentState.acceptSpend(t)
	})

	t.inclusion.OnCommitted(func() {
		input.spentState.commitSpend(t)
		input.spentState.decreaseSpenderCount()
	})

	t.inclusion.OnEvicted(input.spentState.decreaseSpenderCount)

	input.inclusionState.OnAccepted(func() {
		if atomic.AddUint64(&t.unacceptedInputsCount, ^uint64(0)) == 0 {
			t.allInputsAccepted.Trigger()
		}
	})

	input.inclusionState.OnRejected(t.inclusion.setRejected)

	input.inclusionState.OnEvicted(t.inclusion.setEvicted)

	input.spentState.OnSpendAccepted(func(spender mempool.TransactionWithMetadata) {
		if spender != t {
			t.inclusion.setRejected()
		}
	})

	input.spentState.OnSpendCommitted(func(spender mempool.TransactionWithMetadata) {
		if spender != t {
			t.inclusion.setEvicted()
		}
	})
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
