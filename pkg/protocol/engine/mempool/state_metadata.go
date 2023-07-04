package mempool

import (
	"github.com/iotaledger/iota-core/pkg/core/agential"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateMetadata interface {
	ID() iotago.OutputID

	State() State

	ConflictIDs() *agential.SetReceptor[iotago.TransactionID]

	PendingSpenderCount() int

	AcceptedSpender() (TransactionMetadata, bool)

	OnAcceptedSpenderUpdated(callback func(spender TransactionMetadata))

	inclusionFlags
}
