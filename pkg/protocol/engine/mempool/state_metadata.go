package mempool

import (
	"github.com/iotaledger/iota-core/pkg/core/promise"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateMetadata interface {
	ID() iotago.OutputID

	State() State

	ConflictIDs() *promise.Set[iotago.TransactionID]

	PendingSpenderCount() int

	AcceptedSpender() (TransactionMetadata, bool)

	OnAcceptedSpenderUpdated(callback func(spender TransactionMetadata))

	inclusionFlags
}
