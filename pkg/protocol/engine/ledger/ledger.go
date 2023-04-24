package ledger

import (
	iotago2 "iota-core/pkg/protocol/engine/vm"

	iotago "github.com/iotaledger/iota.go/v4"
)

type Ledger interface {
	Output(id iotago.OutputID) (output iotago2.State, exists bool)
}
