package mempoolv1

import (
	"sync"

	"golang.org/x/xerrors"
	"iota-core/pkg/protocol/engine/ledger"
	"iota-core/pkg/protocol/engine/mempool"

	"github.com/iotaledger/hive.go/ds/advancedset"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TransactionWithMetadata struct {
	id              iotago.TransactionID
	inputReferences []mempool.StateReference
	inputs          []*StateWithMetadata
	outputs         []*StateWithMetadata
	transaction     mempool.Transaction

	booked bool

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

func (t *TransactionWithMetadata) IsStored() bool {
	return t != nil
}

func (t *TransactionWithMetadata) IsSolid() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return t.inputs != nil
}

func (t *TransactionWithMetadata) IsBooked() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return t.booked
}

func (t *TransactionWithMetadata) IsExecuted() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return t.outputs != nil
}

func (t *TransactionWithMetadata) PublishInput(index int, input *StateWithMetadata) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.inputs[index] = input
}

func (t *TransactionWithMetadata) PublishOutputStates(outputStates []ledger.State) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	for _, outputState := range outputStates {
		t.outputs = append(t.outputs, NewStateWithMetadata(outputState, t))
	}
}

func (t *TransactionWithMetadata) setBooked() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.booked = true
}
