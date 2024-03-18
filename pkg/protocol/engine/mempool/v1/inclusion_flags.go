package mempoolv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/runtime/promise"
	iotago "github.com/iotaledger/iota.go/v4"
)

// inclusionFlags represents important flags and events that relate to the inclusion of an entity in the distributed ledger.
type inclusionFlags struct {
	// accepted gets triggered when the entity gets marked as accepted.
	accepted reactive.Variable[bool]

	// committedSlot gets set to the slot in which the entity gets marked as committed.
	committedSlot reactive.Variable[iotago.SlotIndex]

	// rejected gets triggered when the entity gets marked as rejected.
	rejected *promise.Event

	// orphanedSlot gets set to the slot in which the entity gets marked as orphaned.
	orphanedSlot reactive.Variable[iotago.SlotIndex]
}

// newInclusionFlags creates a new inclusionFlags instance.
func newInclusionFlags() *inclusionFlags {
	return &inclusionFlags{
		accepted:      reactive.NewVariable[bool](),
		committedSlot: reactive.NewVariable[iotago.SlotIndex](),
		rejected:      promise.NewEvent(),
		// Make sure the oldest orphaned index doesn't get overridden by newer TX spending the orphaned spender resources further.
		orphanedSlot: reactive.NewVariable[iotago.SlotIndex](func(currentValue iotago.SlotIndex, newValue iotago.SlotIndex) iotago.SlotIndex {
			if currentValue != 0 {
				return currentValue
			}

			return newValue
		}),
	}
}

func (s *inclusionFlags) IsPending() bool {
	return !s.IsAccepted() && !s.IsRejected()
}

// IsAccepted returns true if the entity was accepted.
func (s *inclusionFlags) IsAccepted() bool {
	return s.accepted.Get()
}

// OnAccepted registers a callback that gets triggered when the entity gets accepted.
func (s *inclusionFlags) OnAccepted(callback func()) {
	s.accepted.OnUpdate(func(wasAccepted bool, isAccepted bool) {
		if isAccepted && !wasAccepted {
			callback()
		}
	})
}

// IsRejected returns true if the entity was rejected.
func (s *inclusionFlags) IsRejected() bool {
	return s.rejected.WasTriggered()
}

// OnRejected registers a callback that gets triggered when the entity gets rejected.
func (s *inclusionFlags) OnRejected(callback func()) {
	s.rejected.OnTrigger(callback)
}

// CommittedSlot returns the slot in which the entity is committed and a bool value indicating if the entity was committed.
func (s *inclusionFlags) CommittedSlot() (slot iotago.SlotIndex, isCommitted bool) {
	return s.committedSlot.Get(), s.committedSlot.Get() != 0
}

// OnCommittedSlotUpdated registers a callback that gets triggered when the slot in which the entity is committed gets updated..
func (s *inclusionFlags) OnCommittedSlotUpdated(callback func(slot iotago.SlotIndex)) {
	s.committedSlot.OnUpdate(func(_ iotago.SlotIndex, newValue iotago.SlotIndex) {
		callback(newValue)
	})
}

// OrphanedSlot returns a slot in which the entity has been orphaned and a bool flag indicating whether it was orphaned.
func (s *inclusionFlags) OrphanedSlot() (slot iotago.SlotIndex, isOrphaned bool) {
	return s.orphanedSlot.Get(), s.orphanedSlot.Get() != 0
}

// OnOrphanedSlotUpdated registers a callback that gets triggered when the orphaned slot is updated.
func (s *inclusionFlags) OnOrphanedSlotUpdated(callback func(slot iotago.SlotIndex)) {
	s.orphanedSlot.OnUpdate(func(_ iotago.SlotIndex, newValue iotago.SlotIndex) {
		callback(newValue)
	})
}
