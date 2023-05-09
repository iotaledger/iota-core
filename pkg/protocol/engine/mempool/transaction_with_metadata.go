package mempool

import (
	"github.com/iotaledger/hive.go/ds/advancedset"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TransactionWithMetadata interface {
	ID() iotago.TransactionID

	Transaction() Transaction

	Inputs() *advancedset.AdvancedSet[StateWithMetadata]

	Outputs() *advancedset.AdvancedSet[StateWithMetadata]

	Inclusion() InclusionState

	Lifecycle() LifecycleState

	SetCommitted()

	OnEarliestIncludedSlotUpdated(func(prevIndex, newIndex iotago.SlotIndex)) (unsubscribe func())
}
