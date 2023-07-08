package slottracker

import (
	"github.com/iotaledger/hive.go/ds/set"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type SlotTracker struct {
	votesPerIdentity *shrinkingmap.ShrinkingMap[iotago.AccountID, iotago.SlotIndex]
	votersPerSlot    *shrinkingmap.ShrinkingMap[iotago.SlotIndex, set.Set[iotago.AccountID]]
}

func NewSlotTracker() *SlotTracker {
	return &SlotTracker{
		votesPerIdentity: shrinkingmap.New[iotago.AccountID, iotago.SlotIndex](),
		votersPerSlot:    shrinkingmap.New[iotago.SlotIndex, set.Set[iotago.AccountID]](),
	}
}

func (s *SlotTracker) slotVoters(slotIndex iotago.SlotIndex) set.Set[iotago.AccountID] {
	slotVoters, _ := s.votersPerSlot.GetOrCreate(slotIndex, func() set.Set[iotago.AccountID] {
		return set.New[iotago.AccountID]()
	})

	return slotVoters
}

func (s *SlotTracker) TrackVotes(slotIndex iotago.SlotIndex, voterID iotago.AccountID, cutoffIndex iotago.SlotIndex) (prevLatestSlot iotago.SlotIndex, latestSlot iotago.SlotIndex, updated bool) {
	slotVoters := s.slotVoters(slotIndex)
	// We tracked a vote for this voter on this slot already.
	if slotVoters.Has(voterID) {
		// We already tracked the voter for this slot, so no need to update anything
		return
	}

	var previousIndex iotago.SlotIndex
	updatedIndex := s.votesPerIdentity.Compute(voterID, func(currentValue iotago.SlotIndex, exists bool) iotago.SlotIndex {
		previousIndex = currentValue
		if slotIndex > previousIndex {
			updated = true
			return slotIndex
		}

		return previousIndex
	})

	// The new slotIndex is smaller or equal the previousIndex. There's no need to update votersPerSlot.
	if !updated {
		return previousIndex, previousIndex, false
	}

	for i := lo.Max(cutoffIndex, previousIndex) + 1; i <= updatedIndex; i++ {
		s.slotVoters(i).Add(voterID)
	}

	return previousIndex, updatedIndex, true
}

func (s *SlotTracker) Voters(slotIndex iotago.SlotIndex) []iotago.AccountID {
	slotVoters, exists := s.votersPerSlot.Get(slotIndex)
	if !exists {
		return nil
	}

	return slotVoters.ToSlice()
}

func (s *SlotTracker) EvictSlot(indexToEvict iotago.SlotIndex) {
	identities, exists := s.votersPerSlot.Get(indexToEvict)
	if !exists {
		return
	}

	var identitiesToPrune []iotago.AccountID
	_ = identities.ForEach(func(identity iotago.AccountID) error {
		latestVoteIndex, has := s.votesPerIdentity.Get(identity)
		if !has {
			return nil
		}
		if latestVoteIndex <= indexToEvict {
			identitiesToPrune = append(identitiesToPrune, identity)
		}

		return nil
	})

	for _, id := range identitiesToPrune {
		s.votesPerIdentity.Delete(id)
	}

	s.votersPerSlot.Delete(indexToEvict)
}
