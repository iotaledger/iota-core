package slottracker

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type SlotTracker struct {
	votesPerIdentity *shrinkingmap.ShrinkingMap[iotago.AccountID, iotago.SlotIndex]
	votersPerSlot    *shrinkingmap.ShrinkingMap[iotago.SlotIndex, ds.Set[iotago.AccountID]]
}

func NewSlotTracker() *SlotTracker {
	return &SlotTracker{
		votesPerIdentity: shrinkingmap.New[iotago.AccountID, iotago.SlotIndex](),
		votersPerSlot:    shrinkingmap.New[iotago.SlotIndex, ds.Set[iotago.AccountID]](),
	}
}

func (s *SlotTracker) slotVoters(slot iotago.SlotIndex) ds.Set[iotago.AccountID] {
	slotVoters, _ := s.votersPerSlot.GetOrCreate(slot, func() ds.Set[iotago.AccountID] {
		return ds.NewSet[iotago.AccountID]()
	})

	return slotVoters
}

func (s *SlotTracker) TrackVotes(slot iotago.SlotIndex, voterID iotago.AccountID, cutoffIndex iotago.SlotIndex) (prevLatestSlot iotago.SlotIndex, latestSlot iotago.SlotIndex, updated bool) {
	slotVoters := s.slotVoters(slot)
	// We tracked a vote for this voter on this slot already.
	if slotVoters.Has(voterID) {
		// We already tracked the voter for this slot, so no need to update anything
		return
	}

	var previousSlot iotago.SlotIndex
	//nolint:revive
	updatedSlot := s.votesPerIdentity.Compute(voterID, func(currentSlot iotago.SlotIndex, exists bool) iotago.SlotIndex {
		previousSlot = currentSlot
		if slot > previousSlot {
			updated = true
			return slot
		}

		return previousSlot
	})

	// The new slot is smaller or equal the previousSlot. There's no need to update votersPerSlot.
	if !updated {
		return previousSlot, previousSlot, false
	}

	for i := lo.Max(cutoffIndex, previousSlot) + 1; i <= updatedSlot; i++ {
		s.slotVoters(i).Add(voterID)
	}

	return previousSlot, updatedSlot, true
}

func (s *SlotTracker) Voters(slot iotago.SlotIndex) []iotago.AccountID {
	slotVoters, exists := s.votersPerSlot.Get(slot)
	if !exists {
		return nil
	}

	return slotVoters.ToSlice()
}

func (s *SlotTracker) EvictSlot(slotToEvict iotago.SlotIndex) {
	identities, exists := s.votersPerSlot.Get(slotToEvict)
	if !exists {
		return
	}

	var identitiesToPrune []iotago.AccountID
	_ = identities.ForEach(func(identity iotago.AccountID) error {
		latestVoteIndex, has := s.votesPerIdentity.Get(identity)
		if !has {
			return nil
		}
		if latestVoteIndex <= slotToEvict {
			identitiesToPrune = append(identitiesToPrune, identity)
		}

		return nil
	})

	for _, id := range identitiesToPrune {
		s.votesPerIdentity.Delete(id)
	}

	s.votersPerSlot.Delete(slotToEvict)
}
