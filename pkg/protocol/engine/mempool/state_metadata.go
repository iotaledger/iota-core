package mempool

import (
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateMetadata interface {
	ID() iotago.OutputID

	State() ledger.State

	PendingSpenderCount() int

	AcceptedSpender() (TransactionMetadata, bool)

	OnAcceptedSpenderUpdated(callback func(spender TransactionMetadata))

	inclusionFlags
}
