package spenddagv1

import (
	"sync"
	"sync/atomic"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/stringify"
	"github.com/iotaledger/iota-core/pkg/core/weight"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
)

// SortedSpends is a set of Spends that is sorted by their weight.
type SortedSpends[SpendID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]] struct {
	// owner is the Spend that owns this SortedSpends.
	owner *sortedSpend[SpendID, ResourceID, VoteRank]

	// members is a map of SpendIDs to their corresponding sortedSpend.
	members *shrinkingmap.ShrinkingMap[SpendID, *sortedSpend[SpendID, ResourceID, VoteRank]]

	// heaviestMember is the heaviest member of the SortedSpends.
	heaviestMember *sortedSpend[SpendID, ResourceID, VoteRank]

	// heaviestPreferredMember is the heaviest preferred member of the SortedSpends.
	heaviestPreferredMember *sortedSpend[SpendID, ResourceID, VoteRank]

	// pendingWeightUpdates is a collection of Spends that have a pending weight update.
	pendingWeightUpdates *shrinkingmap.ShrinkingMap[SpendID, *sortedSpend[SpendID, ResourceID, VoteRank]]

	// pendingWeightUpdatesSignal is a signal that is used to notify the fixMemberPositionWorker about pending weight
	// updates.
	pendingWeightUpdatesSignal *sync.Cond

	// pendingWeightUpdatesMutex is a mutex that is used to synchronize access to the pendingWeightUpdates.
	pendingWeightUpdatesMutex syncutils.RWMutex

	// pendingPreferredInsteadUpdates is a collection of Spends that have a pending preferred instead update.
	pendingPreferredInsteadUpdates *shrinkingmap.ShrinkingMap[SpendID, *sortedSpend[SpendID, ResourceID, VoteRank]]

	// pendingPreferredInsteadSignal is a signal that is used to notify the fixPreferredInsteadWorker about pending
	// preferred instead updates.
	pendingPreferredInsteadSignal *sync.Cond

	// pendingPreferredInsteadMutex is a mutex that is used to synchronize access to the pendingPreferredInsteadUpdates.
	pendingPreferredInsteadMutex syncutils.RWMutex

	// pendingUpdatesCounter is a counter that keeps track of the number of pending weight updates.
	pendingUpdatesCounter *syncutils.Counter

	// isShutdown is used to signal that the SortedSpends is shutting down.
	isShutdown atomic.Bool

	// mutex is used to synchronize access to the SortedSpends.
	mutex syncutils.RWMutex
}

// NewSortedSpends creates a new SortedSpends that is owned by the given Spend.
func NewSortedSpends[SpendID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]](owner *Spend[SpendID, ResourceID, VoteRank], pendingUpdatesCounter *syncutils.Counter) *SortedSpends[SpendID, ResourceID, VoteRank] {
	s := &SortedSpends[SpendID, ResourceID, VoteRank]{
		members:                        shrinkingmap.New[SpendID, *sortedSpend[SpendID, ResourceID, VoteRank]](),
		pendingWeightUpdates:           shrinkingmap.New[SpendID, *sortedSpend[SpendID, ResourceID, VoteRank]](),
		pendingUpdatesCounter:          pendingUpdatesCounter,
		pendingPreferredInsteadUpdates: shrinkingmap.New[SpendID, *sortedSpend[SpendID, ResourceID, VoteRank]](),
	}
	s.pendingWeightUpdatesSignal = sync.NewCond(&s.pendingWeightUpdatesMutex)
	s.pendingPreferredInsteadSignal = sync.NewCond(&s.pendingPreferredInsteadMutex)

	s.owner = newSortedSpend[SpendID, ResourceID, VoteRank](s, owner)
	s.members.Set(owner.ID, s.owner)

	s.heaviestMember = s.owner
	s.heaviestPreferredMember = s.owner

	go s.fixMemberPositionWorker()
	go s.fixHeaviestPreferredMemberWorker()

	return s
}

// Add adds the given Spend to the SortedSpends.
func (s *SortedSpends[SpendID, ResourceID, VoteRank]) Add(conflict *Spend[SpendID, ResourceID, VoteRank]) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.isShutdown.Load() {
		return false
	}

	newMember, isNew := s.members.GetOrCreate(conflict.ID, func() *sortedSpend[SpendID, ResourceID, VoteRank] {
		return newSortedSpend(s, conflict)
	})
	if !isNew {
		return false
	}

	for currentMember := s.heaviestMember; ; currentMember = currentMember.lighterMember {
		comparison := newMember.Compare(currentMember)
		if comparison == weight.Equal {
			panic("different Spends should never have the same weight")
		}

		if comparison == weight.Heavier {
			if currentMember.heavierMember != nil {
				currentMember.heavierMember.lighterMember = newMember
			}

			newMember.lighterMember = currentMember
			newMember.heavierMember = currentMember.heavierMember
			currentMember.heavierMember = newMember

			if currentMember == s.heaviestMember {
				s.heaviestMember = newMember
			}

			break
		}

		if currentMember.lighterMember == nil {
			currentMember.lighterMember = newMember
			newMember.heavierMember = currentMember

			break
		}
	}

	if newMember.IsPreferred() && newMember.Compare(s.heaviestPreferredMember) == weight.Heavier {
		s.heaviestPreferredMember = newMember

		s.owner.setPreferredInstead(conflict)
	}

	return true
}

// ForEach iterates over all Spends of the SortedSpends and calls the given callback for each of them.
func (s *SortedSpends[SpendID, ResourceID, VoteRank]) ForEach(callback func(*Spend[SpendID, ResourceID, VoteRank]) error, optIncludeOwner ...bool) error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for currentMember := s.heaviestMember; currentMember != nil; currentMember = currentMember.lighterMember {
		if !lo.First(optIncludeOwner) && currentMember == s.owner {
			continue
		}

		if err := callback(currentMember.Spend); err != nil {
			return err
		}
	}

	return nil
}

// Range iterates over all Spends of the SortedSpends and calls the given callback for each of them (without
// manual error handling).
func (s *SortedSpends[SpendID, ResourceID, VoteRank]) Range(callback func(*Spend[SpendID, ResourceID, VoteRank]), optIncludeOwner ...bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for currentMember := s.heaviestMember; currentMember != nil; currentMember = currentMember.lighterMember {
		if !lo.First(optIncludeOwner) && currentMember == s.owner {
			continue
		}

		callback(currentMember.Spend)
	}
}

// Remove removes the Spend with the given ID from the SortedSpends.
func (s *SortedSpends[SpendID, ResourceID, VoteRank]) Remove(id SpendID) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	spend, exists := s.members.Get(id)
	if !exists || !s.members.Delete(id) {
		return false
	}

	spend.Unhook()

	if spend.heavierMember != nil {
		spend.heavierMember.lighterMember = spend.lighterMember
	}

	if spend.lighterMember != nil {
		spend.lighterMember.heavierMember = spend.heavierMember
	}

	if s.heaviestMember == spend {
		s.heaviestMember = spend.lighterMember
	}

	if s.heaviestPreferredMember == spend {
		s.findLowerHeaviestPreferredMember(spend.lighterMember)
	}

	spend.lighterMember = nil
	spend.heavierMember = nil

	return true
}

// String returns a human-readable representation of the SortedSpends.
func (s *SortedSpends[SpendID, ResourceID, VoteRank]) String() string {
	structBuilder := stringify.NewStructBuilder("SortedSpends",
		stringify.NewStructField("owner", s.owner.ID),
		stringify.NewStructField("heaviestMember", s.heaviestMember.ID),
		stringify.NewStructField("heaviestPreferredMember", s.heaviestPreferredMember.ID),
	)

	s.Range(func(spend *Spend[SpendID, ResourceID, VoteRank]) {
		structBuilder.AddField(stringify.NewStructField(spend.ID.String(), spend))
	}, true)

	return structBuilder.String()
}

// notifyPendingWeightUpdate notifies the SortedSpends about a pending weight update of the given member.
func (s *SortedSpends[SpendID, ResourceID, VoteRank]) notifyPendingWeightUpdate(member *sortedSpend[SpendID, ResourceID, VoteRank]) {
	s.pendingWeightUpdatesMutex.Lock()
	defer s.pendingWeightUpdatesMutex.Unlock()

	if _, exists := s.pendingWeightUpdates.Get(member.ID); !exists && !s.isShutdown.Load() {
		s.pendingUpdatesCounter.Increase()
		s.pendingWeightUpdates.Set(member.ID, member)
		s.pendingWeightUpdatesSignal.Signal()
	}
}

// fixMemberPositionWorker is a worker that fixes the position of sortedSetMembers that need to be updated.
func (s *SortedSpends[SpendID, ResourceID, VoteRank]) fixMemberPositionWorker() {
	for member := s.nextPendingWeightUpdate(); member != nil; member = s.nextPendingWeightUpdate() {
		s.applyWeightUpdate(member)

		s.pendingUpdatesCounter.Decrease()
	}
}

// nextPendingWeightUpdate returns the next member that needs to be updated (or nil if the shutdown flag is set).
func (s *SortedSpends[SpendID, ResourceID, VoteRank]) nextPendingWeightUpdate() *sortedSpend[SpendID, ResourceID, VoteRank] {
	s.pendingWeightUpdatesMutex.Lock()
	defer s.pendingWeightUpdatesMutex.Unlock()

	for !s.isShutdown.Load() && s.pendingWeightUpdates.Size() == 0 {
		s.pendingWeightUpdatesSignal.Wait()
	}

	if !s.isShutdown.Load() {
		if _, member, exists := s.pendingWeightUpdates.Pop(); exists {
			return member
		}
	}

	return nil
}

// applyWeightUpdate applies the weight update of the given member.
func (s *SortedSpends[SpendID, ResourceID, VoteRank]) applyWeightUpdate(member *sortedSpend[SpendID, ResourceID, VoteRank]) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.isShutdown.Load() && member.weightUpdateApplied() {
		s.fixMemberPosition(member)
	}
}

// fixMemberPosition fixes the position of the given member in the SortedSpends.
func (s *SortedSpends[SpendID, ResourceID, VoteRank]) fixMemberPosition(member *sortedSpend[SpendID, ResourceID, VoteRank]) {
	preferredConflict := member.PreferredInstead()
	memberIsPreferred := member.IsPreferred()

	// the member needs to be moved up in the list
	for currentMember := member.heavierMember; currentMember != nil && currentMember.Compare(member) == weight.Lighter; currentMember = member.heavierMember {
		s.swapNeighbors(member, currentMember)

		if currentMember == s.heaviestPreferredMember && (preferredConflict == currentMember.Spend || memberIsPreferred || member == s.owner) {
			s.heaviestPreferredMember = member
			s.owner.setPreferredInstead(member.Spend)
		}
	}

	// the member needs to be moved down in the list
	for currentMember := member.lighterMember; currentMember != nil && currentMember.Compare(member) == weight.Heavier; currentMember = member.lighterMember {
		s.swapNeighbors(currentMember, member)

		if member == s.heaviestPreferredMember && (currentMember.IsPreferred() || currentMember.PreferredInstead() == member.Spend || currentMember == s.owner) {
			s.heaviestPreferredMember = currentMember
			s.owner.setPreferredInstead(currentMember.Spend)
		}
	}
}

// notifyPreferredInsteadUpdate notifies the SortedSpends about a member that changed its preferred instead flag.
func (s *SortedSpends[SpendID, ResourceID, VoteRank]) notifyPendingPreferredInsteadUpdate(member *sortedSpend[SpendID, ResourceID, VoteRank]) {
	s.pendingPreferredInsteadMutex.Lock()
	defer s.pendingPreferredInsteadMutex.Unlock()

	if _, exists := s.pendingPreferredInsteadUpdates.Get(member.ID); !exists && !s.isShutdown.Load() {
		s.pendingUpdatesCounter.Increase()
		s.pendingPreferredInsteadUpdates.Set(member.ID, member)
		s.pendingPreferredInsteadSignal.Signal()
	}
}

// fixMemberPositionWorker is a worker that fixes the position of sortedSetMembers that need to be updated.
func (s *SortedSpends[SpendID, ResourceID, VoteRank]) fixHeaviestPreferredMemberWorker() {
	for member := s.nextPendingPreferredMemberUpdate(); member != nil; member = s.nextPendingPreferredMemberUpdate() {
		s.applyPreferredInsteadUpdate(member)

		s.pendingUpdatesCounter.Decrease()
	}
}

// nextPendingWeightUpdate returns the next member that needs to be updated (or nil if the shutdown flag is set).
func (s *SortedSpends[SpendID, ResourceID, VoteRank]) nextPendingPreferredMemberUpdate() *sortedSpend[SpendID, ResourceID, VoteRank] {
	s.pendingPreferredInsteadMutex.Lock()
	defer s.pendingPreferredInsteadMutex.Unlock()

	for !s.isShutdown.Load() && s.pendingPreferredInsteadUpdates.Size() == 0 {
		s.pendingPreferredInsteadSignal.Wait()
	}

	if !s.isShutdown.Load() {
		if _, member, exists := s.pendingPreferredInsteadUpdates.Pop(); exists {
			return member
		}
	}

	return nil
}

// applyPreferredInsteadUpdate applies the preferred instead update of the given member.
func (s *SortedSpends[SpendID, ResourceID, VoteRank]) applyPreferredInsteadUpdate(member *sortedSpend[SpendID, ResourceID, VoteRank]) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.isShutdown.Load() && member.preferredInsteadUpdateApplied() {
		s.fixHeaviestPreferredMember(member)
	}
}

// fixHeaviestPreferredMember fixes the heaviest preferred member of the SortedSpends after updating the given member.
func (s *SortedSpends[SpendID, ResourceID, VoteRank]) fixHeaviestPreferredMember(member *sortedSpend[SpendID, ResourceID, VoteRank]) {
	if member.IsPreferred() {
		if member.Compare(s.heaviestPreferredMember) == weight.Heavier {
			s.heaviestPreferredMember = member
			s.owner.setPreferredInstead(member.Spend)
		}

		return
	}

	if s.heaviestPreferredMember == member {
		s.findLowerHeaviestPreferredMember(member)
	}
}

func (s *SortedSpends[SpendID, ResourceID, VoteRank]) findLowerHeaviestPreferredMember(member *sortedSpend[SpendID, ResourceID, VoteRank]) {
	for currentMember := member; currentMember != nil; currentMember = currentMember.lighterMember {
		if currentMember == s.owner || currentMember.IsPreferred() || currentMember.PreferredInstead() == member.Spend {
			s.heaviestPreferredMember = currentMember
			s.owner.setPreferredInstead(currentMember.Spend)

			return
		}
	}

	s.heaviestPreferredMember = nil
}

// swapNeighbors swaps the given members in the SortedSpends.
func (s *SortedSpends[SpendID, ResourceID, VoteRank]) swapNeighbors(heavierMember *sortedSpend[SpendID, ResourceID, VoteRank], lighterMember *sortedSpend[SpendID, ResourceID, VoteRank]) {
	if heavierMember.lighterMember != nil {
		heavierMember.lighterMember.heavierMember = lighterMember
	}
	if lighterMember.heavierMember != nil {
		lighterMember.heavierMember.lighterMember = heavierMember
	}

	lighterMember.lighterMember = heavierMember.lighterMember
	heavierMember.heavierMember = lighterMember.heavierMember
	lighterMember.heavierMember = heavierMember
	heavierMember.lighterMember = lighterMember

	if s.heaviestMember == lighterMember {
		s.heaviestMember = heavierMember
	}
}

func (s *SortedSpends[SpendID, ResourceID, VoteRank]) Shutdown() []*Spend[SpendID, ResourceID, VoteRank] {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.pendingWeightUpdatesMutex.Lock()
	defer s.pendingWeightUpdatesMutex.Unlock()
	s.pendingPreferredInsteadMutex.Lock()
	defer s.pendingPreferredInsteadMutex.Unlock()

	s.isShutdown.Store(true)

	s.pendingUpdatesCounter.Update(-s.pendingWeightUpdates.Size())
	s.pendingUpdatesCounter.Update(-s.pendingPreferredInsteadUpdates.Size())

	s.pendingPreferredInsteadSignal.Broadcast()
	s.pendingWeightUpdatesSignal.Broadcast()

	return lo.Map(s.members.Values(), func(spend *sortedSpend[SpendID, ResourceID, VoteRank]) *Spend[SpendID, ResourceID, VoteRank] {
		return spend.Spend
	})
}
