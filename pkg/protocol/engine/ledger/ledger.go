package ledger

import (
	iotago2 "iota-core/pkg/iotago"

	iotago "github.com/iotaledger/iota.go/v4"
)

type Ledger interface {
	Output(id iotago.OutputID) (output iotago2.Output, exists bool)
}
