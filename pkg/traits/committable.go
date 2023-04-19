package traits

import (
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/typedkey"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Committable is a trait that stores information about the latest commitment.
type Committable interface {
	// SetLastCommittedSlot sets the last committed slot.
	SetLastCommittedSlot(index iotago.SlotIndex)

	// LastCommittedSlot returns the last committed slot.
	LastCommittedSlot() (index iotago.SlotIndex)
}

// NewCommittable creates a new Committable trait.
func NewCommittable(store kvstore.KVStore, keyBytes ...byte) (newCommittable Committable) {
	return &committable{
		lastCommittedSlot: typedkey.NewGenericType[iotago.SlotIndex](store, keyBytes...),
	}
}

// committable is the implementation of the Committable trait.
type committable struct {
	lastCommittedSlot *typedkey.GenericType[iotago.SlotIndex]
}

// SetLastCommittedSlot sets the last committed slot.
func (c *committable) SetLastCommittedSlot(index iotago.SlotIndex) {
	if c.lastCommittedSlot.Get() != index {
		c.lastCommittedSlot.Set(index)
	}
}

// LastCommittedSlot returns the last committed slot.
func (c *committable) LastCommittedSlot() iotago.SlotIndex {
	return c.lastCommittedSlot.Get()
}
