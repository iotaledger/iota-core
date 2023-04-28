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
	conflictIDs     *advancedset.AdvancedSet[iotago.TransactionID]
	inclusionSlot   iotago.SlotIndex

	booked    *promise.Event
	solid     *promise.Event
	executed  *promise.Event
	committed *promise.Event
	evicted   *promise.Event
	invalid   *promise.Event1[error]
	accepted  *promise.Event

	unacceptedInputsCount uint64

	mutex sync.RWMutex
}

func (t *TransactionWithMetadata) decreaseUnacceptedInputsCount() (newValue uint64) {
	return atomic.AddUint64(&t.unacceptedInputsCount, ^uint64(0))
}

func (t *TransactionWithMetadata) AllInputsAccepted() bool {
	return atomic.LoadUint64(&t.unacceptedInputsCount) == 0
}

func NewTransactionMetadata(transaction mempool.Transaction) (*TransactionWithMetadata, error) {
	transactionID, transactionIDErr := transaction.ID()
	if transactionIDErr != nil {
		return nil, xerrors.Errorf("failed to retrieve transaction ID: %w", transactionIDErr)
	}

	inputReferences, inputsErr := transaction.Inputs()
	if inputsErr != nil {
		return nil, xerrors.Errorf("failed to retrieve inputReferences of transaction %s: %w", transactionID, inputsErr)
	}

	return &TransactionWithMetadata{
		id:                    transactionID,
		inputReferences:       inputReferences,
		inputs:                make([]*StateWithMetadata, len(inputReferences)),
		transaction:           transaction,
		conflictIDs:           advancedset.New[iotago.TransactionID](),
		booked:                promise.NewEvent(),
		solid:                 promise.NewEvent(),
		executed:              promise.NewEvent(),
		evicted:               promise.NewEvent(),
		invalid:               promise.NewEvent1[error](),
		accepted:              promise.NewEvent(),
		unacceptedInputsCount: uint64(len(inputReferences)),
	}, nil
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

func (t *TransactionWithMetadata) IsSolid() bool {
	return t.solid.WasTriggered()
}

func (t *TransactionWithMetadata) IsExecuted() bool {
	return t.executed.WasTriggered()
}

func (t *TransactionWithMetadata) IsBooked() bool {
	return t.booked.WasTriggered()
}

func (t *TransactionWithMetadata) IsInvalid() bool {
	return t.invalid.WasTriggered()
}

func (t *TransactionWithMetadata) IsEvicted() bool {
	return t.evicted.WasTriggered()
}

func (t *TransactionWithMetadata) OnSolid(callback func()) {
	t.solid.OnTrigger(callback)
}

func (t *TransactionWithMetadata) OnExecuted(callback func()) {
	t.executed.OnTrigger(callback)
}

func (t *TransactionWithMetadata) OnBooked(callback func()) {
	t.booked.OnTrigger(callback)
}

func (t *TransactionWithMetadata) OnInvalid(callback func(error)) {
	t.invalid.OnTrigger(callback)
}

func (t *TransactionWithMetadata) OnEvicted(callback func()) {
	t.evicted.OnTrigger(callback)
}

func (t *TransactionWithMetadata) OnAccepted(callback func()) {
	t.accepted.OnTrigger(callback)
}

func (t *TransactionWithMetadata) publishInput(index int, input *StateWithMetadata) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.inputs[index] = input

	input.OnSpent(func(spender *TransactionWithMetadata) {
		if spender != t {
			return
		}
	})
}

func (t *TransactionWithMetadata) publishExecutionResult(outputStates []ledger.State) {
	t.mutex.Lock()
	for _, outputState := range outputStates {
		t.outputs = append(t.outputs, NewStateWithMetadata(outputState, t))
	}
	t.mutex.Unlock()

	t.executed.Trigger()
}

func (t *TransactionWithMetadata) triggerBooked() {
	t.booked.Trigger()
}

func (t *TransactionWithMetadata) triggerInvalid(reason error) {
	t.invalid.Trigger(reason)
}

func (t *TransactionWithMetadata) setAccepted() (updated bool) {
	if updated = t.accepted.Trigger(); updated {
		lo.ForEach(t.outputs, (*StateWithMetadata).setAccepted)
	}

	return updated
}

func (t *TransactionWithMetadata) InclusionSlot() iotago.SlotIndex {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return t.inclusionSlot
}

func (t *TransactionWithMetadata) IsIncluded() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return t.inclusionSlot != 0
}

func (t *TransactionWithMetadata) setInclusionSlot(slot iotago.SlotIndex) (previousValue iotago.SlotIndex) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if previousValue = t.inclusionSlot; previousValue == 0 || slot < previousValue {
		t.inclusionSlot = slot
	}

	return previousValue
}
