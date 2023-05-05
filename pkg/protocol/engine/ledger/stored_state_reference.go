package ledger

import (
	iotago "github.com/iotaledger/iota.go/v4"
)

// StoredStateReference is a reference to a State that is stored in the ledger state.
type StoredStateReference iotago.OutputID

// Type returns the type of the StateReference.
func (l StoredStateReference) Type() StateReferenceType {
	return 0
}

// ReferencedStateID returns the ID of the referenced State in the ledger state.
func (l StoredStateReference) StateID() iotago.OutputID {
	return iotago.OutputID(l)
}
