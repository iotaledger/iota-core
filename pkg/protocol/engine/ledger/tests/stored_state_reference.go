package ledgertests

import (
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

// StoredStateReference is a reference to a State that is stored in the ledger state.
type StoredStateReference iotago.OutputID

func (l StoredStateReference) StateID() iotago.Identifier {
	return iotago.IdentifierFromData(lo.PanicOnErr(l.OutputID().Bytes()))
}

// Type returns the type of the StateReference.
func (l StoredStateReference) Type() iotago.StateType {
	return 0
}

// Size returns the size of the StateReference.
func (l StoredStateReference) Size() int {
	return 0
}

// WorkScore returns the workscore of the StateReference.
func (l StoredStateReference) WorkScore(_ *iotago.WorkScoreStructure) (iotago.WorkScore, error) {
	return 0, nil
}

// OutputID returns the ID of the referenced State in the ledger state.
func (l StoredStateReference) OutputID() iotago.OutputID {
	return iotago.OutputID(l)
}

// Index returns the Index of the referenced State.
func (l StoredStateReference) Index() uint16 {
	return iotago.OutputID(l).Index()
}
