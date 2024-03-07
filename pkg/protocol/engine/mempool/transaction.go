package mempool

import (
	iotago "github.com/iotaledger/iota.go/v4"
)

type SignedTransaction interface {
	// ID returns the identifier of the Transaction that contains a signature.
	ID() (iotago.SignedTransactionID, error)
	// MustID works like ID but panics if the SignedTransactionID can't be computed.
	MustID() iotago.SignedTransactionID
}

type Transaction interface {
	// ID returns the identifier of the Transaction.
	ID() (iotago.TransactionID, error)
	// MustID works like ID but panics if the TransactionID can't be computed.
	MustID() iotago.TransactionID
}
