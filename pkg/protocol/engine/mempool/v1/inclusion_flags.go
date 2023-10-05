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
		// Make sure the oldest orphaned index doesn't get overridden by newer TX spending the orphaned conflict further.
		orphanedSlot: reactive.NewVariable[iotago.SlotIndex](func(currentValue, newValue iotago.SlotIndex) iotago.SlotIndex {
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
	s.accepted.OnUpdate(func(wasAccepted, isAccepted bool) {
		if isAccepted && !wasAccepted {
			callback()
		}
	})
}

// OnPending registers a callback that gets triggered when the entity gets pending.
func (s *inclusionFlags) OnPending(callback func()) {
	s.accepted.OnUpdate(func(wasAccepted, isAccepted bool) {
		if !isAccepted && wasAccepted {
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

// IsCommitted returns true if the entity was committed.
func (s *inclusionFlags) CommittedSlot() (slot iotago.SlotIndex, isCommitted bool) {
	return s.committedSlot.Get(), s.committedSlot.Get() != 0
}

// OnCommitted registers a callback that gets triggered when the entity gets committed.
func (s *inclusionFlags) OnCommittedSlotUpdated(callback func(slot iotago.SlotIndex)) {
	s.committedSlot.OnUpdate(func(_, newValue iotago.SlotIndex) {
		callback(newValue)
	})
}

// IsOrphaned returns true if the entity was orphaned.
func (s *inclusionFlags) OrphanedSlot() (slot iotago.SlotIndex, isOrphaned bool) {
	return s.orphanedSlot.Get(), s.orphanedSlot.Get() != 0
}

// OnOrphaned registers a callback that gets triggered when the entity gets orphaned.
func (s *inclusionFlags) OnOrphanedSlotUpdated(callback func(slot iotago.SlotIndex)) {
	s.orphanedSlot.OnUpdate(func(_, newValue iotago.SlotIndex) {
		callback(newValue)
	})
}
