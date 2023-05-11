package mempool

import (
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateMetadata interface {
	ID() iotago.OutputID

	State() ledger.State

	IsSpent() bool

	OnDoubleSpent(callback func())

	OnSpendAccepted(callback func(spender TransactionMetadata))

	OnSpendCommitted(callback func(spender TransactionMetadata))

	AllSpendersRemoved() bool

	OnAllSpendersRemoved(callback func()) (unsubscribe func())

	SpenderCount() uint64

	HasNoSpenders() bool

	inclusionFlags
}
