package mempool

import (
	iotago "github.com/iotaledger/iota.go/v4"
)

// StateReference is a reference to a State (e.g. an identifier in the ledger state, a proof of inclusion against a
// merkle root or another source of information).
type StateReference interface {
	// ReferencedStateID returns the ID of the referenced State in the replicated ledger.
	ReferencedStateID() iotago.OutputID
}
