package mempool

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/reactive"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TransactionMetadata interface {
	ID() iotago.TransactionID

	Transaction() Transaction

	Inputs() ds.Set[StateMetadata]

	Outputs() ds.Set[StateMetadata]

	SpenderIDs() reactive.Set[iotago.TransactionID]

	Commit()

	IsSolid() bool

	OnSolid(callback func())

	IsExecuted() bool

	OnExecuted(callback func())

	IsInvalid() bool

	OnInvalid(callback func(error))

	IsBooked() bool

	OnBooked(callback func())

	ValidAttachments() []iotago.BlockID

	EarliestIncludedAttachment() iotago.BlockID

	OnEarliestIncludedAttachmentUpdated(callback func(prevID, newID iotago.BlockID))

	OnEvicted(callback func())

	inclusionFlags
}

type inclusionFlags interface {
	IsPending() bool

	IsAccepted() bool

	OnAccepted(callback func())

	CommittedSlot() (slot iotago.SlotIndex, isCommitted bool)

	OnCommittedSlotUpdated(callback func(slot iotago.SlotIndex))

	IsRejected() bool

	OnRejected(callback func())

	OrphanedSlot() (slot iotago.SlotIndex, isOrphaned bool)

	OnOrphanedSlotUpdated(callback func(slot iotago.SlotIndex))
}
