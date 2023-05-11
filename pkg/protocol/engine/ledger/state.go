package ledger

import iotago "github.com/iotaledger/iota.go/v4"

type State interface {
	// ID returns the identifier of the State.
	ID() iotago.OutputID
}
