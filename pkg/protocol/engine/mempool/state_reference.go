package mempool

import iotago "github.com/iotaledger/iota.go/v4"

type StateReference interface {
	ReferencedStateID() iotago.Identifier

	// Type returns the type of Input.
	Type() iotago.StateType
}
