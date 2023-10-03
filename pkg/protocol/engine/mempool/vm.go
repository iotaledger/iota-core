package mempool

import (
	"context"
)

// VM is the interface that defines the virtual machine that is used to validate and execute transactions.
type VM interface {
	// Inputs returns the referenced inputs of the given transaction.
	Inputs(transaction Transaction) ([]StateReference, error)

	// ValidateSignatures validates the signatures of the given SignedTransaction and returns the execution context.
	ValidateSignatures(signedTransaction SignedTransaction, inputs []State) (executionContext context.Context, err error)

	// Execute executes the transaction in the given execution context and returns the resulting states.
	Execute(executionContext context.Context, transaction Transaction) (outputs []State, err error)
}
