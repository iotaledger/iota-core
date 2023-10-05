package mempool

import iotago "github.com/iotaledger/iota.go/v4"

type State interface {
	StateID() StateID

	Type() iotago.StateType

	IsReadOnly() bool
}
