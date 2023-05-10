package mempoolv1

import (
	"sync"

	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/advancedset"
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
	attachments     *Attachments
	inclusionState  *TransactionInclusion
	lifecycle       *LifecycleState

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

		inclusionState: NewTransactionInclusion(len(inputReferences)),
		lifecycle:      NewLifecycleState(len(inputReferences)),
	}

	t.inclusionState.dependsOnAttachments(t.attachments)

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

func (t *TransactionWithMetadata) Lifecycle() mempool.TransactionLifecycle {
	return t.lifecycle
}

func (t *TransactionWithMetadata) Attachments() mempool.Attachments {
	return t.attachments
}

func (t *TransactionWithMetadata) Inclusion() mempool.TransactionInclusion {
	return t.inclusionState
}

func (t *TransactionWithMetadata) publishInput(index int, input *StateWithMetadata) {
	t.inputs[index] = input

	input.spentState.dependsOnSpender(t)
	t.inclusionState.dependsOnInput(input)

	t.lifecycle.markInputSolid()
}

func (t *TransactionWithMetadata) setExecuted(outputStates []ledger.State) {
	t.mutex.Lock()
	for _, outputState := range outputStates {
		t.outputs = append(t.outputs, NewStateWithMetadata(outputState, t))
	}
	t.mutex.Unlock()

	t.lifecycle.executed.Trigger()
}
