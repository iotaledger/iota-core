package mempool

import (
	iotago "github.com/iotaledger/iota.go/v4"
)

type Transaction interface {
	// ID returns the identifier of the Transaction.
	ID() (iotago.TransactionID, error)

	// Inputs returns the inputs of the Transaction.
	Inputs() ([]iotago.IndexedUTXOReferencer, error)

	// String returns a human-readable version of the Transaction.
	String() string
}
