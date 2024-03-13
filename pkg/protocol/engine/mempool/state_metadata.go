package mempool

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateMetadata interface {
	State() State

	SpenderIDs() reactive.Set[iotago.TransactionID]

	PendingSpenderCount() int

	AcceptedSpender() (TransactionMetadata, bool)

	OnAcceptedSpenderUpdated(callback func(spender TransactionMetadata))

	EarliestIncludedAttachment() iotago.BlockID

	OnEarliestIncludedAttachmentUpdated(func(prevID, newID iotago.BlockID))

	inclusionFlags
}
