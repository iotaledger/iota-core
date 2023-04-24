package mempooltests

import (
	"iota-core/pkg/protocol/engine/mempool"

	iotago "github.com/iotaledger/iota.go/v4"
)

type LedgerStateReference iotago.OutputID

func (l LedgerStateReference) Type() mempool.StateReferenceType {
	return 0
}

func (l LedgerStateReference) ReferencedStateID() iotago.OutputID {
	return iotago.OutputID(l)
}
