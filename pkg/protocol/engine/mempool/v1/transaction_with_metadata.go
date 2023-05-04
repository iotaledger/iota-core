package mempoolv1

import (
	"sync"
	"sync/atomic"

	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/lo"
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

	unacceptedInputsCount uint64
	allInputsAccepted     *promise.Event
	accepted              *promise.Event
	rejected              *promise.Event
	committed             *promise.Event
	evicted               *promise.Event

	stored             *promise.Event
	unsolidInputsCount uint64
	solid              *promise.Event
	executed           *promise.Event
	invalid            *promise.Event1[error]
	booked             *promise.Event

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

		booked:            promise.NewEvent(),
		solid:             promise.NewEvent(),
		executed:          promise.NewEvent(),
		evicted:           promise.NewEvent(),
		allInputsAccepted: promise.NewEvent(),
		accepted:          promise.NewEvent(),
		committed:         promise.NewEvent(),
		rejected:          promise.NewEvent(),
		stored:            promise.NewEvent(),
		invalid:           promise.NewEvent1[error](),

		unsolidInputsCount:    uint64(len(inputReferences)),
		unacceptedInputsCount: uint64(len(inputReferences)),
	}

	t.attachments.OnAllAttachmentsEvicted(t.setEvicted)

	return t, nil
}

func (t *TransactionWithMetadata) ID() iotago.TransactionID {
	return t.id
}

func (t *TransactionWithMetadata) Transaction() mempool.Transaction {
	return t.transaction
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

// region InclusionState ///////////////////////////////////////////////////////////////////////////////////////////////

func (t *TransactionWithMetadata) AllInputsAccepted() bool {
	return t.allInputsAccepted.WasTriggered()
}

func (t *TransactionWithMetadata) OnAllInputsAccepted(callback func()) {
	t.allInputsAccepted.OnTrigger(callback)
}

func (t *TransactionWithMetadata) IsAccepted() bool {
	return t.accepted.WasTriggered()
}

func (t *TransactionWithMetadata) OnAccepted(callback func()) {
	t.accepted.OnTrigger(callback)
}

func (t *TransactionWithMetadata) IsRejected() bool {
	return t.rejected.WasTriggered()
}

func (t *TransactionWithMetadata) OnRejected(callback func()) {
	t.rejected.OnTrigger(callback)
}

func (t *TransactionWithMetadata) IsCommitted() bool {
	return t.committed.WasTriggered()
}

func (t *TransactionWithMetadata) OnCommitted(callback func()) {
	t.committed.OnTrigger(callback)
}

func (t *TransactionWithMetadata) IsEvicted() bool {
	return t.evicted.WasTriggered()
}

func (t *TransactionWithMetadata) OnEvicted(callback func()) {
	t.evicted.OnTrigger(callback)
}

func (t *TransactionWithMetadata) setAccepted() {
	if t.accepted.Trigger() {
		lo.ForEach(t.outputs, (*StateWithMetadata).setAccepted)
	}
}

func (t *TransactionWithMetadata) setRejected() {
	if t.rejected.Trigger() {
		lo.ForEach(t.outputs, (*StateWithMetadata).setRejected)
	}
}

func (t *TransactionWithMetadata) setCommitted() {
	if t.committed.Trigger() {
		lo.ForEach(t.outputs, (*StateWithMetadata).setCommitted)
	}
}

func (t *TransactionWithMetadata) setEvicted() {
	if t.evicted.Trigger() {
		lo.ForEach(t.outputs, (*StateWithMetadata).setEvicted)
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region Lifecycle ////////////////////////////////////////////////////////////////////////////////////////////////////

func (t *TransactionWithMetadata) IsStored() bool {
	return t.stored.WasTriggered()
}

func (t *TransactionWithMetadata) OnStored(callback func()) {
	t.stored.OnTrigger(callback)
}

func (t *TransactionWithMetadata) IsSolid() bool {
	return t.solid.WasTriggered()
}

func (t *TransactionWithMetadata) OnSolid(callback func()) {
	t.solid.OnTrigger(callback)
}

func (t *TransactionWithMetadata) IsExecuted() bool {
	return t.executed.WasTriggered()
}

func (t *TransactionWithMetadata) OnExecuted(callback func()) {
	t.executed.OnTrigger(callback)
}

func (t *TransactionWithMetadata) IsInvalid() bool {
	return t.invalid.WasTriggered()
}

func (t *TransactionWithMetadata) OnInvalid(callback func(error)) {
	t.invalid.OnTrigger(callback)
}

func (t *TransactionWithMetadata) IsBooked() bool {
	return t.booked.WasTriggered()
}

func (t *TransactionWithMetadata) OnBooked(callback func()) {
	t.booked.OnTrigger(callback)
}

func (t *TransactionWithMetadata) setStored() {
	t.stored.Trigger()
}

func (t *TransactionWithMetadata) publishInput(index int, input *StateWithMetadata) {
	t.inputs[index] = input

	t.setupInputLifecycle(input)

	if atomic.AddUint64(&t.unsolidInputsCount, ^uint64(0)) == 0 {
		t.solid.Trigger()
	}
}

func (t *TransactionWithMetadata) setBooked() {
	t.booked.Trigger()
}

func (t *TransactionWithMetadata) setExecuted(outputStates []ledger.State) {
	t.mutex.Lock()
	for _, outputState := range outputStates {
		t.outputs = append(t.outputs, NewStateWithMetadata(outputState, t))
	}
	t.mutex.Unlock()

	t.executed.Trigger()
}

func (t *TransactionWithMetadata) setInvalid(reason error) {
	t.invalid.Trigger(reason)
}

func (t *TransactionWithMetadata) setupInputLifecycle(input *StateWithMetadata) {
	input.increaseConsumerCount()

	t.OnAccepted(func() {
		input.acceptSpend(t)
	})

	t.OnCommitted(func() {
		input.commitSpend(t)
		input.decreaseConsumerCount()
	})

	t.OnEvicted(input.decreaseConsumerCount)

	input.OnAccepted(func() {
		if atomic.AddUint64(&t.unacceptedInputsCount, ^uint64(0)) == 0 {
			t.allInputsAccepted.Trigger()
		}
	})

	input.OnRejected(t.setRejected)

	input.OnEvicted(t.setEvicted)

	input.OnSpendAccepted(func(spender *TransactionWithMetadata) {
		if spender != t {
			t.setRejected()
		}
	})

	input.OnSpendCommitted(func(spender *TransactionWithMetadata) {
		if spender != t {
			t.setEvicted()
		}
	})
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
