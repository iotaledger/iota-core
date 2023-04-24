package ledger

import (
	"iota-core/pkg/protocol/engine/mempool/promise"
	"iota-core/pkg/protocol/engine/vm"

	iotago "github.com/iotaledger/iota.go/v4"
)

type StateResolver func(stateID iotago.OutputID) *promise.Promise[vm.State]
