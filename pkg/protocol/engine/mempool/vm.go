package mempool

import (
	"context"

	iotago "github.com/iotaledger/iota.go/v4"
)

// VM is the interface that defines the virtual machine that is used to validate and execute transactions.
type VM interface {
	// TransactionInputs returns the inputs of the given transaction.
	TransactionInputs(transaction Transaction) ([]iotago.Input, error)

	// ValidateSignatures validates the signatures of the given SignedTransaction and returns the execution context.
	ValidateSignatures(signedTransaction SignedTransaction, resolvedInputs []State) (executionContext context.Context, err error)

	// ExecuteTransaction executes the transaction in the given execution context and returns the resulting states.
	ExecuteTransaction(executionContext context.Context, transaction Transaction) (outputs []State, err error)
}
