package slottracker

import (
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/votes/latestvotes"
	iotago "github.com/iotaledger/iota.go/v4"
)

type SlotTracker struct {
	Events *Events

	votesPerIdentity *shrinkingmap.ShrinkingMap[iotago.AccountID, *latestvotes.LatestVotes[iotago.SlotIndex, SlotVotePower]]
	votersPerSlot    *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *advancedset.AdvancedSet[iotago.AccountID]]

	cutoffIndexCallback func() iotago.SlotIndex
}

func NewSlotTracker(cutoffIndexCallback func() iotago.SlotIndex) *SlotTracker {
	return &SlotTracker{
		votesPerIdentity: shrinkingmap.New[iotago.AccountID, *latestvotes.LatestVotes[iotago.SlotIndex, SlotVotePower]](),
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

func (s *SlotTracker) TrackVotes(slotIndex iotago.SlotIndex, voterID iotago.AccountID, power SlotVotePower) {
	slotVoters := s.slotVoters(slotIndex)
	if slotVoters.Has(voterID) {
		// We already tracked the voter for this slot, so no need to update anything
		return
	}

	votersVotes, _ := s.votesPerIdentity.GetOrCreate(voterID, func() *latestvotes.LatestVotes[iotago.SlotIndex, SlotVotePower] {
		return latestvotes.NewLatestVotes[iotago.SlotIndex, SlotVotePower](voterID)
	})

	updated, previousHighestIndex := votersVotes.Store(slotIndex, power)
	if !updated || previousHighestIndex >= slotIndex {
		return
	}

	for i := lo.Max(s.cutoffIndexCallback(), previousHighestIndex) + 1; i <= slotIndex; i++ {
		s.slotVoters(i).Add(voterID)
	}

	s.Events.VotersUpdated.Trigger(&VoterUpdatedEvent{
		Voter:               voterID,
		NewLatestSlotIndex:  slotIndex,
		PrevLatestSlotIndex: previousHighestIndex,
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
		votesForIdentity, has := s.votesPerIdentity.Get(identity)
		if !has {
			return nil
		}
		power, hasPower := votesForIdentity.Power(indexToEvict)
		if !hasPower {
			return nil
		}
		if power.Index <= indexToEvict {
			identitiesToPrune = append(identitiesToPrune, identity)
		}

		return nil
	})

	for _, id := range identitiesToPrune {
		s.votesPerIdentity.Delete(id)
	}

	s.votersPerSlot.Delete(indexToEvict)
}

// region SlotVotePower //////////////////////////////////////////////////////////////////////////////////////////////

type SlotVotePower struct {
	Index iotago.SlotIndex
}

func (p SlotVotePower) Compare(other SlotVotePower) int {
	if other.Index > p.Index {
		return -1
	} else if other.Index < p.Index {
		return 1
	} else {
		return 0
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
