package mempoolv1

import (
	"sync"

	"golang.org/x/xerrors"
	"iota-core/pkg/promise"
	"iota-core/pkg/protocol/engine/ledger"
	"iota-core/pkg/protocol/engine/mempool"

	"github.com/iotaledger/hive.go/ds/advancedset"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TransactionWithMetadata struct {
	id              iotago.TransactionID
	inputReferences []ledger.StateReference
	inputs          []*StateWithMetadata
	outputs         []*StateWithMetadata
	transaction     mempool.Transaction

	booked   *promise.Event
	solid    *promise.Event
	executed *promise.Event
	evicted  *promise.Event
	invalid  *promise.Event1[error]

	mutex sync.RWMutex
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
		id:              transactionID,
		inputReferences: inputReferences,
		inputs:          make([]*StateWithMetadata, len(inputReferences)),
		transaction:     transaction,
		booked:          promise.NewEvent(),
		solid:           promise.NewEvent(),
		executed:        promise.NewEvent(),
		evicted:         promise.NewEvent(),
		invalid:         promise.NewEvent1[error](),
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
