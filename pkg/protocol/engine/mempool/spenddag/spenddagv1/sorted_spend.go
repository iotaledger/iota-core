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

// sortedSpend is a wrapped Spend that contains additional information for the SortedSpends.
type sortedSpend[SpendID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]] struct {
	// sortedSet is the SortedSpends that contains this sortedSpend.
	sortedSet *SortedSpends[SpendID, ResourceID, VoteRank]

	// lighterMember is the sortedSpend that is lighter than this one.
	lighterMember *sortedSpend[SpendID, ResourceID, VoteRank]

	// heavierMember is the sortedSpend that is heavierMember than this one.
	heavierMember *sortedSpend[SpendID, ResourceID, VoteRank]

	// currentWeight is the current weight of the Spend.
	currentWeight weight.Value

	// queuedWeight is the weight that is queued to be applied to the Spend.
	queuedWeight *weight.Value

	// weightMutex is used to protect the currentWeight and queuedWeight.
	weightMutex syncutils.RWMutex

	// currentPreferredInstead is the current PreferredInstead value of the Spend.
	currentPreferredInstead *Spend[SpendID, ResourceID, VoteRank]

	// queuedPreferredInstead is the PreferredInstead value that is queued to be applied to the Spend.
	queuedPreferredInstead *Spend[SpendID, ResourceID, VoteRank]

	// preferredMutex is used to protect the currentPreferredInstead and queuedPreferredInstead.
	preferredInsteadMutex syncutils.RWMutex

	onAcceptanceStateUpdatedHook *event.Hook[func(acceptance.State, acceptance.State)]

	// onWeightUpdatedHook is the hook that is triggered when the weight of the Spend is updated.
	onWeightUpdatedHook *event.Hook[func(weight.Value)]

	// onPreferredUpdatedHook is the hook that is triggered when the PreferredInstead value of the Spend is updated.
	onPreferredUpdatedHook *event.Hook[func(*Spend[SpendID, ResourceID, VoteRank])]

	// Spend is the wrapped Spend.
	*Spend[SpendID, ResourceID, VoteRank]
}

// newSortedSpend creates a new sortedSpend.
func newSortedSpend[SpendID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]](set *SortedSpends[SpendID, ResourceID, VoteRank], spend *Spend[SpendID, ResourceID, VoteRank]) *sortedSpend[SpendID, ResourceID, VoteRank] {
	s := &sortedSpend[SpendID, ResourceID, VoteRank]{
		sortedSet:               set,
		currentWeight:           spend.Weight.Value(),
		currentPreferredInstead: spend.PreferredInstead(),
		Spend:                   spend,
	}

	if set.owner != nil {
		s.onAcceptanceStateUpdatedHook = spend.AcceptanceStateUpdated.Hook(s.onAcceptanceStateUpdated)
	}

	s.onWeightUpdatedHook = spend.Weight.OnUpdate.Hook(s.queueWeightUpdate)
	s.onPreferredUpdatedHook = spend.PreferredInsteadUpdated.Hook(s.queuePreferredInsteadUpdate)

	return s
}

// Weight returns the current weight of the sortedSpend.
func (s *sortedSpend[SpendID, ResourceID, VoteRank]) Weight() weight.Value {
	s.weightMutex.RLock()
	defer s.weightMutex.RUnlock()

	return s.currentWeight
}

// Compare compares the sortedSpend to another sortedSpend.
func (s *sortedSpend[SpendID, ResourceID, VoteRank]) Compare(other *sortedSpend[SpendID, ResourceID, VoteRank]) int {
	if result := s.Weight().Compare(other.Weight()); result != weight.Equal {
		return result
	}

	return bytes.Compare(lo.PanicOnErr(s.ID.Bytes()), lo.PanicOnErr(other.ID.Bytes()))
}

// PreferredInstead returns the current preferred instead value of the sortedSpend.
func (s *sortedSpend[SpendID, ResourceID, VoteRank]) PreferredInstead() *Spend[SpendID, ResourceID, VoteRank] {
	s.preferredInsteadMutex.RLock()
	defer s.preferredInsteadMutex.RUnlock()

	return s.currentPreferredInstead
}

// IsPreferred returns true if the sortedSpend is preferred instead of its Spends.
func (s *sortedSpend[SpendID, ResourceID, VoteRank]) IsPreferred() bool {
	return s.PreferredInstead() == s.Spend
}

// Unhook cleans up the sortedSpend.
func (s *sortedSpend[SpendID, ResourceID, VoteRank]) Unhook() {
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

func (s *sortedSpend[SpendID, ResourceID, VoteRank]) onAcceptanceStateUpdated(_ acceptance.State, newState acceptance.State) {
	if newState.IsAccepted() {
		s.sortedSet.owner.setAcceptanceState(acceptance.Rejected)
	}
}

// queueWeightUpdate queues a weight update for the sortedSpend.
func (s *sortedSpend[SpendID, ResourceID, VoteRank]) queueWeightUpdate(newWeight weight.Value) {
	s.weightMutex.Lock()
	defer s.weightMutex.Unlock()

	if (s.queuedWeight == nil && s.currentWeight == newWeight) || (s.queuedWeight != nil && *s.queuedWeight == newWeight) {
		return
	}

	s.queuedWeight = &newWeight
	s.sortedSet.notifyPendingWeightUpdate(s)
}

// weightUpdateApplied tries to apply a queued weight update to the sortedSpend and returns true if successful.
func (s *sortedSpend[SpendID, ResourceID, VoteRank]) weightUpdateApplied() bool {
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
func (s *sortedSpend[SpendID, ResourceID, VoteRank]) queuePreferredInsteadUpdate(conflict *Spend[SpendID, ResourceID, VoteRank]) {
	s.preferredInsteadMutex.Lock()
	defer s.preferredInsteadMutex.Unlock()

	if (s.queuedPreferredInstead == nil && s.currentPreferredInstead == conflict) ||
		(s.queuedPreferredInstead != nil && s.queuedPreferredInstead == conflict) ||
		s.sortedSet.owner.Spend == conflict {
		return
	}

	s.queuedPreferredInstead = conflict
	s.sortedSet.notifyPendingPreferredInsteadUpdate(s)
}

// preferredInsteadUpdateApplied tries to apply a queued preferred instead update to the sortedSpend and returns
// true if successful.
func (s *sortedSpend[SpendID, ResourceID, VoteRank]) preferredInsteadUpdateApplied() bool {
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
