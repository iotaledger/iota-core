package ledger

import (
	"iota-core/pkg/core/promise"

	iotago "github.com/iotaledger/iota.go/v4"
)

// StateReference is a reference to a State (e.g. an identifier in the ledger state, a proof of inclusion against a
// merkle root or another source of information).
type StateReference interface {
	Type() StateReferenceType

	// StateID returns the ID of the referenced State in the replicated ledger.
	StateID() iotago.OutputID
}

// StateReferenceResolver is a function that resolves a StateReference to a State.
type StateReferenceResolver func(reference StateReference) *promise.Promise[State]

// StateReferenceType is the type of StateReference.
type StateReferenceType = uint16
