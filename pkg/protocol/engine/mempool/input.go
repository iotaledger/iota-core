package mempool

import (
	"iota-core/pkg/protocol/engine/ledger"
)

type Input interface {
	ID() ledger.OutputID
}
