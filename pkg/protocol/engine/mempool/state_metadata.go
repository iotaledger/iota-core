package mempool

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	iotago "github.com/iotaledger/iota.go/v4"
)

type StateMetadata interface {
	CreatingTransaction() TransactionMetadata

	State() State

	SpenderIDs() reactive.Set[iotago.TransactionID]

	PendingSpenderCount() int

	AcceptedSpender() (TransactionMetadata, bool)

	OnAcceptedSpenderUpdated(callback func(spender TransactionMetadata))

	InclusionSlot() iotago.SlotIndex

	OnInclusionSlotUpdated(callback func(prevSlot iotago.SlotIndex, newSlot iotago.SlotIndex))

	inclusionFlags
}
