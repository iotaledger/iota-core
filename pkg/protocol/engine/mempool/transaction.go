package mempool

import (
	iotago "github.com/iotaledger/iota.go/v4"
)

type SignedTransaction interface {
	// ID returns the identifier of the Transaction that contains a signature.
	ID() (iotago.SignedTransactionID, error)
}

type Transaction interface {
	// ID returns the identifier of the Transaction.
	ID() (iotago.TransactionID, error)
}
