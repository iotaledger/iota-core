package spenddagv1

import (
	"bytes"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/acceptance"
	"github.com/iotaledger/iota-core/pkg/core/weight"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
)

// sortedSpend is a wrapped Spender that contains additional information for the SortedSpenders.
type sortedSpender[SpenderID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]] struct {
	// sortedSet is the SortedSpenders that contains this sortedSpender.
	sortedSet *SortedSpenders[SpenderID, ResourceID, VoteRank]

	// lighterMember is the sortedSpender that is lighter than this one.
	lighterMember *sortedSpender[SpenderID, ResourceID, VoteRank]

	// heavierMember is the sortedSpender that is heavierMember than this one.
	heavierMember *sortedSpender[SpenderID, ResourceID, VoteRank]

	// currentWeight is the current weight of the Spender.
	currentWeight weight.Value

	// queuedWeight is the weight that is queued to be applied to the Spender.
	queuedWeight *weight.Value

	// weightMutex is used to protect the currentWeight and queuedWeight.
	weightMutex syncutils.RWMutex

	// currentPreferredInstead is the current PreferredInstead value of the Spender.
	currentPreferredInstead *Spender[SpenderID, ResourceID, VoteRank]

	// queuedPreferredInstead is the PreferredInstead value that is queued to be applied to the Spender.
	queuedPreferredInstead *Spender[SpenderID, ResourceID, VoteRank]

	// preferredMutex is used to protect the currentPreferredInstead and queuedPreferredInstead.
	preferredInsteadMutex syncutils.RWMutex

	onAcceptanceStateUpdatedHook *event.Hook[func(acceptance.State, acceptance.State)]

	// onWeightUpdatedHook is the hook that is triggered when the weight of the Spender is updated.
	onWeightUpdatedHook *event.Hook[func(weight.Value)]

	// onPreferredUpdatedHook is the hook that is triggered when the PreferredInstead value of the Spender is updated.
	onPreferredUpdatedHook *event.Hook[func(*Spender[SpenderID, ResourceID, VoteRank])]

	// Spender is the wrapped Spender.
	*Spender[SpenderID, ResourceID, VoteRank]
}

// newSortedSpender creates a new sortedSpender.
func newSortedSpender[SpenderID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]](set *SortedSpenders[SpenderID, ResourceID, VoteRank], spender *Spender[SpenderID, ResourceID, VoteRank]) *sortedSpender[SpenderID, ResourceID, VoteRank] {
	s := &sortedSpender[SpenderID, ResourceID, VoteRank]{
		sortedSet:               set,
		currentWeight:           spender.Weight.Value(),
		currentPreferredInstead: spender.PreferredInstead(),
		Spender:                 spender,
	}

	if set.owner != nil {
		s.onAcceptanceStateUpdatedHook = spender.AcceptanceStateUpdated.Hook(s.onAcceptanceStateUpdated)
	}

	s.onWeightUpdatedHook = spender.Weight.OnUpdate.Hook(s.queueWeightUpdate)
	s.onPreferredUpdatedHook = spender.PreferredInsteadUpdated.Hook(s.queuePreferredInsteadUpdate)

	return s
}

// Weight returns the current weight of the sortedSpender.
func (s *sortedSpender[SpenderID, ResourceID, VoteRank]) Weight() weight.Value {
	s.weightMutex.RLock()
	defer s.weightMutex.RUnlock()

	return s.currentWeight
}

// Compare compares the sortedSpend to another sortedSpender.
func (s *sortedSpender[SpenderID, ResourceID, VoteRank]) Compare(other *sortedSpender[SpenderID, ResourceID, VoteRank]) int {
	if result := s.Weight().Compare(other.Weight()); result != weight.Equal {
		return result
	}

	return bytes.Compare(lo.PanicOnErr(s.ID.Bytes()), lo.PanicOnErr(other.ID.Bytes()))
}

// PreferredInstead returns the current preferred instead value of the sortedSpender.
func (s *sortedSpender[SpenderID, ResourceID, VoteRank]) PreferredInstead() *Spender[SpenderID, ResourceID, VoteRank] {
	s.preferredInsteadMutex.RLock()
	defer s.preferredInsteadMutex.RUnlock()

	return s.currentPreferredInstead
}

// IsPreferred returns true if the sortedSpender is preferred instead of its Spenders.
func (s *sortedSpender[SpenderID, ResourceID, VoteRank]) IsPreferred() bool {
	return s.PreferredInstead() == s.Spender
}

// Unhook cleans up the sortedSpender.
func (s *sortedSpender[SpenderID, ResourceID, VoteRank]) Unhook() {
	if s.onAcceptanceStateUpdatedHook != nil {
		s.onAcceptanceStateUpdatedHook.Unhook()
		s.onAcceptanceStateUpdatedHook = nil
	}

	if s.onWeightUpdatedHook != nil {
		s.onWeightUpdatedHook.Unhook()
		s.onWeightUpdatedHook = nil
	}

	if s.onPreferredUpdatedHook != nil {
		s.onPreferredUpdatedHook.Unhook()
		s.onPreferredUpdatedHook = nil
	}
}

func (s *sortedSpender[SpenderID, ResourceID, VoteRank]) onAcceptanceStateUpdated(_ acceptance.State, newState acceptance.State) {
	if newState.IsAccepted() {
		s.sortedSet.owner.setAcceptanceState(acceptance.Rejected)
	}
}

// queueWeightUpdate queues a weight update for the sortedSpender.
func (s *sortedSpender[SpenderID, ResourceID, VoteRank]) queueWeightUpdate(newWeight weight.Value) {
	s.weightMutex.Lock()
	defer s.weightMutex.Unlock()

	if (s.queuedWeight == nil && s.currentWeight == newWeight) || (s.queuedWeight != nil && *s.queuedWeight == newWeight) {
		return
	}

	s.queuedWeight = &newWeight
	s.sortedSet.notifyPendingWeightUpdate(s)
}

// weightUpdateApplied tries to apply a queued weight update to the sortedSpend and returns true if successful.
func (s *sortedSpender[SpenderID, ResourceID, VoteRank]) weightUpdateApplied() bool {
	s.weightMutex.Lock()
	defer s.weightMutex.Unlock()

	if s.queuedWeight == nil {
		return false
	}

	if *s.queuedWeight == s.currentWeight {
		s.queuedWeight = nil

		return false
	}

	s.currentWeight = *s.queuedWeight
	s.queuedWeight = nil

	return true
}

// queuePreferredInsteadUpdate notifies the sortedSet that the preferred instead flag of the Spend was updated.
func (s *sortedSpender[SpenderID, ResourceID, VoteRank]) queuePreferredInsteadUpdate(spender *Spender[SpenderID, ResourceID, VoteRank]) {
	s.preferredInsteadMutex.Lock()
	defer s.preferredInsteadMutex.Unlock()

	if (s.queuedPreferredInstead == nil && s.currentPreferredInstead == spender) ||
		(s.queuedPreferredInstead != nil && s.queuedPreferredInstead == spender) ||
		s.sortedSet.owner.Spender == spender {
		return
	}

	s.queuedPreferredInstead = spender
	s.sortedSet.notifyPendingPreferredInsteadUpdate(s)
}

// preferredInsteadUpdateApplied tries to apply a queued preferred instead update to the sortedSpend and returns
// true if successful.
func (s *sortedSpender[SpenderID, ResourceID, VoteRank]) preferredInsteadUpdateApplied() bool {
	s.preferredInsteadMutex.Lock()
	defer s.preferredInsteadMutex.Unlock()

	if s.queuedPreferredInstead == nil {
		return false
	}

	if s.queuedPreferredInstead == s.currentPreferredInstead {
		s.queuedPreferredInstead = nil

		return false
	}

	s.currentPreferredInstead = s.queuedPreferredInstead
	s.queuedPreferredInstead = nil

	return true
}
