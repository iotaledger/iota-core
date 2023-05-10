package mempool

import iotago "github.com/iotaledger/iota.go/v4"

type Attachments interface {
	OnEarliestIncludedSlotUpdated(func(prevIndex, newIndex iotago.SlotIndex)) (unsubscribe func())

	EarliestIncludedSlot() iotago.SlotIndex
}
