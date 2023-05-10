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

	t.attachments.OnAllAttachmentsEvicted(func() {
		if !t.inclusionState.IsCommitted() {
			t.inclusionState.setOrphaned()
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

	t.setupInputLifecycle(input)

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

func (t *TransactionWithMetadata) setupInputLifecycle(input *StateWithMetadata) {
	input.spentState.increaseSpenderCount()

	t.inclusionState.OnAccepted(func() {
		input.spentState.acceptSpend(t)
	})

	t.inclusionState.OnCommitted(func() {
		input.spentState.commitSpend(t)
		input.spentState.decreaseSpenderCount()
	})

	t.inclusionState.OnOrphaned(input.spentState.decreaseSpenderCount)

	input.inclusionState.OnAccepted(t.inclusionState.markInputAccepted)

	input.inclusionState.OnRejected(t.inclusionState.setRejected)

	input.inclusionState.OnOrphaned(t.inclusionState.setOrphaned)

	input.spentState.OnSpendAccepted(func(spender mempool.TransactionWithMetadata) {
		if spender != t {
			t.inclusionState.setRejected()
		}
	})

	input.spentState.OnSpendCommitted(func(spender mempool.TransactionWithMetadata) {
		if spender != t {
			t.inclusionState.setOrphaned()
		}
	})
}
