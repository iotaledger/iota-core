package ledger

import (
	"iota-core/pkg/promise"

	iotago "github.com/iotaledger/iota.go/v4"
)

type StateResolver func(stateID iotago.OutputID) *promise.Promise[State]
