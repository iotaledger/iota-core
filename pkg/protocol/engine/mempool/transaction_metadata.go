package mempool

import (
	"github.com/iotaledger/hive.go/ds/advancedset"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TransactionMetadata interface {
	ID() iotago.TransactionID

	Transaction() Transaction

	Inputs() *advancedset.AdvancedSet[StateMetadata]

	Outputs() *advancedset.AdvancedSet[StateMetadata]

	Commit()

	IsSolid() bool

	OnSolid(func())

	IsExecuted() bool

	OnExecuted(func())

	IsInvalid() bool

	OnInvalid(func(error))

	IsBooked() bool

	OnBooked(func())

	IsConflicting() bool

	OnConflicting(func())

	OnEarliestIncludedAttachmentUpdated(func(prevID, newID iotago.BlockID))

	EarliestIncludedAttachment() iotago.BlockID

	inclusionFlags
}

type inclusionFlags interface {
	IsPending() bool

	OnPending(callback func())

	IsAccepted() bool

	OnAccepted(callback func())

	IsCommitted() bool

	OnCommitted(callback func())

	IsRejected() bool

	OnRejected(callback func())

	IsOrphaned() bool

	OnOrphaned(callback func())
}
