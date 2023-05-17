package ledger

import (
	iotago "github.com/iotaledger/iota.go/v4"
)

// StoredStateReference is a reference to a State that is stored in the ledger state.
type StoredStateReference iotago.OutputID

// Type returns the type of the StateReference.
func (l StoredStateReference) Type() iotago.InputType {
	return 0
}

// Size returns the sizeof the StateReference.
func (l StoredStateReference) Size() int {
	return 0
}

// Ref returns the ID of the referenced State in the ledger state.
func (l StoredStateReference) Ref() iotago.OutputID {
	return iotago.OutputID(l)
}

// Index returns the Index of the referenced State.
func (l StoredStateReference) Index() uint16 {
	return iotago.OutputID(l).Index()
}
