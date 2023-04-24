package mempool

import (
	"iota-core/pkg/protocol/engine/vm"

	iotago "github.com/iotaledger/iota.go/v4"
)

type Transaction interface {
	vm.StateTransition

	// ID returns the identifier of the Transaction.
	ID() (iotago.TransactionID, error)

	// Inputs returns the inputs of the Transaction.
	Inputs() ([]StateReference, error)

	// String returns a human-readable version of the Transaction.
	String() string
}
