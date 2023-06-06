package mempool

import (
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TransactionMetadata interface {
	ID() iotago.TransactionID

	Transaction() Transaction

	Inputs() *advancedset.AdvancedSet[StateMetadata]

	Outputs() *advancedset.AdvancedSet[StateMetadata]

	ConflictIDs() *promise.Set[iotago.TransactionID]

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

	Attachments() []iotago.BlockID

	EarliestIncludedAttachment() iotago.BlockID

	OnEarliestIncludedAttachmentUpdated(func(prevID, newID iotago.BlockID))

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
