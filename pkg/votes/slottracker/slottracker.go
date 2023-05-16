package slottracker

import (
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type SlotTracker struct {
	Events *Events

	votesPerIdentity *shrinkingmap.ShrinkingMap[iotago.AccountID, iotago.SlotIndex]
	votersPerSlot    *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *advancedset.AdvancedSet[iotago.AccountID]]

	cutoffIndexCallback func() iotago.SlotIndex
}

func NewSlotTracker(cutoffIndexCallback func() iotago.SlotIndex) *SlotTracker {
	return &SlotTracker{
		votesPerIdentity: shrinkingmap.New[iotago.AccountID, iotago.SlotIndex](),
		votersPerSlot:    shrinkingmap.New[iotago.SlotIndex, *advancedset.AdvancedSet[iotago.AccountID]](),

		cutoffIndexCallback: cutoffIndexCallback,
		Events:              NewEvents(),
	}
}

func (s *SlotTracker) slotVoters(slotIndex iotago.SlotIndex) *advancedset.AdvancedSet[iotago.AccountID] {
	slotVoters, _ := s.votersPerSlot.GetOrCreate(slotIndex, func() *advancedset.AdvancedSet[iotago.AccountID] {
		return advancedset.New[iotago.AccountID]()
	})

	return slotVoters
}

func (s *SlotTracker) TrackVotes(slotIndex iotago.SlotIndex, voterID iotago.AccountID) {
	slotVoters := s.slotVoters(slotIndex)
	// We tracked a vote for this voter on this slot already.
	if slotVoters.Has(voterID) {
		// We already tracked the voter for this slot, so no need to update anything
		return
	}

	var previousIndex iotago.SlotIndex
	var updated bool
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
		return
	}

	for i := lo.Max(s.cutoffIndexCallback(), previousIndex) + 1; i <= updatedIndex; i++ {
		s.slotVoters(i).Add(voterID)
	}

	s.Events.VotersUpdated.Trigger(&VoterUpdatedEvent{
		Voter:               voterID,
		NewLatestSlotIndex:  updatedIndex,
		PrevLatestSlotIndex: previousIndex,
	})
}

func (s *SlotTracker) Voters(slotIndex iotago.SlotIndex) *advancedset.AdvancedSet[iotago.AccountID] {
	voters := advancedset.New[iotago.AccountID]()

	slotVoters, exists := s.votersPerSlot.Get(slotIndex)
	if !exists {
		return voters
	}

	_ = slotVoters.ForEach(func(identityID iotago.AccountID) error {
		voters.Add(identityID)
		return nil
	})

	return voters
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
