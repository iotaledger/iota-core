package mempoolv1

import (
	"sync"

	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TransactionMetadata struct {
	id              iotago.TransactionID
	inputReferences []ledger.StateReference
	inputs          []*StateMetadata
	outputs         []*StateMetadata
	transaction     mempool.Transaction
	conflictIDs     *advancedset.AdvancedSet[iotago.TransactionID]

	*TransactionLifecycle
	*TransactionInclusion
	*Attachments

	mutex sync.RWMutex
}

func NewTransactionWithMetadata(transaction mempool.Transaction) (*TransactionMetadata, error) {
	transactionID, transactionIDErr := transaction.ID()
	if transactionIDErr != nil {
		return nil, xerrors.Errorf("failed to retrieve transaction ID: %w", transactionIDErr)
	}

	inputReferences, inputsErr := transaction.Inputs()
	if inputsErr != nil {
		return nil, xerrors.Errorf("failed to retrieve inputReferences of transaction %s: %w", transactionID, inputsErr)
	}

	t := &TransactionMetadata{
		id:              transactionID,
		inputReferences: inputReferences,
		inputs:          make([]*StateMetadata, len(inputReferences)),
		transaction:     transaction,
		conflictIDs:     advancedset.New[iotago.TransactionID](),
		Attachments:     NewAttachments(),

		TransactionLifecycle: NewTransactionLifecycle(len(inputReferences)),
		TransactionInclusion: NewTransactionInclusion(len(inputReferences)),
	}

	t.dependsOnAttachments(t.Attachments)

	return t, nil
}

func (t *TransactionMetadata) ID() iotago.TransactionID {
	return t.id
}

func (t *TransactionMetadata) Transaction() mempool.Transaction {
	return t.transaction
}

func (t *TransactionMetadata) Inputs() *advancedset.AdvancedSet[mempool.StateMetadata] {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	inputs := advancedset.New[mempool.StateMetadata]()
	for _, input := range t.inputs {
		inputs.Add(input)
	}

	return inputs
}

func (t *TransactionMetadata) Outputs() *advancedset.AdvancedSet[mempool.StateMetadata] {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	outputs := advancedset.New[mempool.StateMetadata]()
	for _, output := range t.outputs {
		outputs.Add(output)
	}

	return outputs
}

func (t *TransactionMetadata) Lifecycle() mempool.TransactionLifecycle {
	return t.TransactionLifecycle
}

func (t *TransactionMetadata) publishInput(index int, input *StateMetadata) {
	t.inputs[index] = input

	input.dependsOnSpender(t)
	t.dependsOnInput(input)

	t.markInputSolid()
}

func (t *TransactionMetadata) setExecuted(outputStates []ledger.State) {
	t.mutex.Lock()
	for _, outputState := range outputStates {
		t.outputs = append(t.outputs, NewStateMetadata(outputState, t))
	}
	t.mutex.Unlock()

	t.TransactionLifecycle.executed.Trigger()
}
