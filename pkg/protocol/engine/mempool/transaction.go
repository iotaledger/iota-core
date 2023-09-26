package mempool

import (
	iotago "github.com/iotaledger/iota.go/v4"
)

type Transaction interface {
	// ID returns the identifier of the Transaction.
	ID(iotago.API) (iotago.TransactionID, error)

	// EssenceID returns the essence identifier of the Transaction.
	//	EssenceID(iotago.API) (iotago.TransactionEssenceID, error)

	// Inputs returns the inputs of the Transaction.
	Inputs() ([]*iotago.UTXOInput, error)

	// CommitmentInput returns the commitment input of the Transaction, if present.
	CommitmentInput() *iotago.CommitmentInput

	// ContextInputs returns the context inputs of the Transaction.
	ContextInputs() (iotago.TransactionContextInputs, error)

	// String returns a human-readable version of the Transaction.
	String() string
}
