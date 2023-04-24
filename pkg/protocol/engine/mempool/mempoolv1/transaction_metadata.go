package mempoolv1

import (
	"sync"

	"golang.org/x/xerrors"
	"iota-core/pkg/protocol/engine/mempool"
	"iota-core/pkg/protocol/engine/vm"

	iotago "github.com/iotaledger/iota.go/v4"
)

type TransactionMetadata struct {
	id              iotago.TransactionID
	inputReferences []mempool.StateReference
	inputs          []*StateMetadata
	outputs         []*StateMetadata
	Transaction     vm.StateTransition

	mutex sync.RWMutex
}

func NewTransactionMetadata(transaction mempool.Transaction) (*TransactionMetadata, error) {
	transactionID, transactionIDErr := transaction.ID()
	if transactionIDErr != nil {
		return nil, xerrors.Errorf("failed to retrieve transaction ID: %w", transactionIDErr)
	}

	inputReferences, inputsErr := transaction.Inputs()
	if inputsErr != nil {
		return nil, xerrors.Errorf("failed to retrieve inputReferences of transaction %s: %w", transactionID, inputsErr)
	}

	return &TransactionMetadata{
		id:              transactionID,
		inputReferences: inputReferences,
		inputs:          make([]*StateMetadata, len(inputReferences)),
		Transaction:     transaction,
	}, nil
}

func (t *TransactionMetadata) ID() iotago.TransactionID {
	return t.id
}

func (t *TransactionMetadata) IsBooked() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return t.outputs != nil
}

func (t *TransactionMetadata) PublishInput(index int, input *StateMetadata) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.inputs[index] = input
}

func (t *TransactionMetadata) PublishOutputStates(outputStates []vm.State) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	for _, outputState := range outputStates {
		t.outputs = append(t.outputs, NewStateMetadata(outputState, t))
	}
}
