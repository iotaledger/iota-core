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

	AllInputsAccepted() bool

	OnAllInputsAccepted(callback func())

	Commit()

	IsSolid() bool

	OnSolid(func())

	IsExecuted() bool

	OnExecuted(func())

	IsInvalid() bool

	OnInvalid(func(error))

	IsBooked() bool

	OnBooked(func())

	OnEarliestIncludedSlotUpdated(func(prevIndex, newIndex iotago.SlotIndex)) (unsubscribe func())

	EarliestIncludedSlot() iotago.SlotIndex

	Inclusion
}
