package mempool

import (
	"iota-core/pkg/protocol/engine/ledger"

	iotago "github.com/iotaledger/iota.go/v4"
)

type StateWithMetadata interface {
	ID() iotago.OutputID

	State() ledger.State
}
