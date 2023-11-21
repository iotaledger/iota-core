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

// SortedSpenders is a set of Spenders that is sorted by their weight.
type SortedSpenders[SpenderID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]] struct {
	// owner is the Spend that owns this SortedSpends.
	owner *sortedSpender[SpenderID, ResourceID, VoteRank]

	// members is a map of SpenderIDs to their corresponding sortedSpender.
	members *shrinkingmap.ShrinkingMap[SpenderID, *sortedSpender[SpenderID, ResourceID, VoteRank]]

	// heaviestMember is the heaviest member of the SortedSpends.
	heaviestMember *sortedSpender[SpenderID, ResourceID, VoteRank]

	// heaviestPreferredMember is the heaviest preferred member of the SortedSpends.
	heaviestPreferredMember *sortedSpender[SpenderID, ResourceID, VoteRank]

	// pendingWeightUpdates is a collection of Spenders that have a pending weight update.
	pendingWeightUpdates *shrinkingmap.ShrinkingMap[SpenderID, *sortedSpender[SpenderID, ResourceID, VoteRank]]

	// pendingWeightUpdatesSignal is a signal that is used to notify the fixMemberPositionWorker about pending weight
	// updates.
	pendingWeightUpdatesSignal *sync.Cond

	// pendingWeightUpdatesMutex is a mutex that is used to synchronize access to the pendingWeightUpdates.
	pendingWeightUpdatesMutex syncutils.RWMutex

	// pendingPreferredInsteadUpdates is a collection of Spenders that have a pending preferred instead update.
	pendingPreferredInsteadUpdates *shrinkingmap.ShrinkingMap[SpenderID, *sortedSpender[SpenderID, ResourceID, VoteRank]]

	// pendingPreferredInsteadSignal is a signal that is used to notify the fixPreferredInsteadWorker about pending
	// preferred instead updates.
	pendingPreferredInsteadSignal *sync.Cond

	// pendingPreferredInsteadMutex is a mutex that is used to synchronize access to the pendingPreferredInsteadUpdates.
	pendingPreferredInsteadMutex syncutils.RWMutex

	// pendingUpdatesCounter is a counter that keeps track of the number of pending weight updates.
	pendingUpdatesCounter *syncutils.Counter

	// isShutdown is used to signal that the SortedSpenders is shutting down.
	isShutdown atomic.Bool

	// mutex is used to synchronize access to the SortedSpends.
	mutex syncutils.RWMutex
}

// NewSortedSpenders creates a new SortedSpenders that is owned by the given Spender.
func NewSortedSpenders[SpenderID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]](owner *Spender[SpenderID, ResourceID, VoteRank], pendingUpdatesCounter *syncutils.Counter) *SortedSpenders[SpenderID, ResourceID, VoteRank] {
	s := &SortedSpenders[SpenderID, ResourceID, VoteRank]{
		members:                        shrinkingmap.New[SpenderID, *sortedSpender[SpenderID, ResourceID, VoteRank]](),
		pendingWeightUpdates:           shrinkingmap.New[SpenderID, *sortedSpender[SpenderID, ResourceID, VoteRank]](),
		pendingUpdatesCounter:          pendingUpdatesCounter,
		pendingPreferredInsteadUpdates: shrinkingmap.New[SpenderID, *sortedSpender[SpenderID, ResourceID, VoteRank]](),
	}
	s.pendingWeightUpdatesSignal = sync.NewCond(&s.pendingWeightUpdatesMutex)
	s.pendingPreferredInsteadSignal = sync.NewCond(&s.pendingPreferredInsteadMutex)

	s.owner = newSortedSpender[SpenderID, ResourceID, VoteRank](s, owner)
	s.members.Set(owner.ID, s.owner)

	s.heaviestMember = s.owner
	s.heaviestPreferredMember = s.owner

	go s.fixMemberPositionWorker()
	go s.fixHeaviestPreferredMemberWorker()

	return s
}

// Add adds the given Spend to the SortedSpends.
func (s *SortedSpenders[SpenderID, ResourceID, VoteRank]) Add(spender *Spender[SpenderID, ResourceID, VoteRank]) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.isShutdown.Load() {
		return false
	}

	newMember, isNew := s.members.GetOrCreate(spender.ID, func() *sortedSpender[SpenderID, ResourceID, VoteRank] {
		return newSortedSpender(s, spender)
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

		s.owner.setPreferredInstead(spender)
	}

	return true
}

// ForEach iterates over all Spends of the SortedSpends and calls the given callback for each of them.
func (s *SortedSpenders[SpenderID, ResourceID, VoteRank]) ForEach(callback func(*Spender[SpenderID, ResourceID, VoteRank]) error, optIncludeOwner ...bool) error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for currentMember := s.heaviestMember; currentMember != nil; currentMember = currentMember.lighterMember {
		if !lo.First(optIncludeOwner) && currentMember == s.owner {
			continue
		}

		if err := callback(currentMember.Spender); err != nil {
			return err
		}
	}

	return nil
}

// Range iterates over all Spends of the SortedSpends and calls the given callback for each of them (without
// manual error handling).
func (s *SortedSpenders[SpenderID, ResourceID, VoteRank]) Range(callback func(*Spender[SpenderID, ResourceID, VoteRank]), optIncludeOwner ...bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for currentMember := s.heaviestMember; currentMember != nil; currentMember = currentMember.lighterMember {
		if !lo.First(optIncludeOwner) && currentMember == s.owner {
			continue
		}

		callback(currentMember.Spender)
	}
}

// Remove removes the Spend with the given ID from the SortedSpends.
func (s *SortedSpenders[SpenderID, ResourceID, VoteRank]) Remove(id SpenderID) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	sortedSpender, exists := s.members.Get(id)
	if !exists || !s.members.Delete(id) {
		return false
	}

	sortedSpender.Unhook()

	if sortedSpender.heavierMember != nil {
		sortedSpender.heavierMember.lighterMember = sortedSpender.lighterMember
	}

	if sortedSpender.lighterMember != nil {
		sortedSpender.lighterMember.heavierMember = sortedSpender.heavierMember
	}

	if s.heaviestMember == sortedSpender {
		s.heaviestMember = sortedSpender.lighterMember
	}

	if s.heaviestPreferredMember == sortedSpender {
		s.findLowerHeaviestPreferredMember(sortedSpender.lighterMember)
	}

	sortedSpender.lighterMember = nil
	sortedSpender.heavierMember = nil

	return true
}

// String returns a human-readable representation of the SortedSpends.
func (s *SortedSpenders[SpenderID, ResourceID, VoteRank]) String() string {
	structBuilder := stringify.NewStructBuilder("SortedSpends",
		stringify.NewStructField("owner", s.owner.ID),
		stringify.NewStructField("heaviestMember", s.heaviestMember.ID),
		stringify.NewStructField("heaviestPreferredMember", s.heaviestPreferredMember.ID),
	)

	s.Range(func(spender *Spender[SpenderID, ResourceID, VoteRank]) {
		structBuilder.AddField(stringify.NewStructField(spender.ID.String(), spender))
	}, true)

	return structBuilder.String()
}

// notifyPendingWeightUpdate notifies the SortedSpends about a pending weight update of the given member.
func (s *SortedSpenders[SpenderID, ResourceID, VoteRank]) notifyPendingWeightUpdate(member *sortedSpender[SpenderID, ResourceID, VoteRank]) {
	s.pendingWeightUpdatesMutex.Lock()
	defer s.pendingWeightUpdatesMutex.Unlock()

	if _, exists := s.pendingWeightUpdates.Get(member.ID); !exists && !s.isShutdown.Load() {
		s.pendingUpdatesCounter.Increase()
		s.pendingWeightUpdates.Set(member.ID, member)
		s.pendingWeightUpdatesSignal.Signal()
	}
}

// fixMemberPositionWorker is a worker that fixes the position of sortedSetMembers that need to be updated.
func (s *SortedSpenders[SpenderID, ResourceID, VoteRank]) fixMemberPositionWorker() {
	for member := s.nextPendingWeightUpdate(); member != nil; member = s.nextPendingWeightUpdate() {
		s.applyWeightUpdate(member)

		s.pendingUpdatesCounter.Decrease()
	}
}

// nextPendingWeightUpdate returns the next member that needs to be updated (or nil if the shutdown flag is set).
func (s *SortedSpenders[SpenderID, ResourceID, VoteRank]) nextPendingWeightUpdate() *sortedSpender[SpenderID, ResourceID, VoteRank] {
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
func (s *SortedSpenders[SpenderID, ResourceID, VoteRank]) applyWeightUpdate(member *sortedSpender[SpenderID, ResourceID, VoteRank]) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.isShutdown.Load() && member.weightUpdateApplied() {
		s.fixMemberPosition(member)
	}
}

// fixMemberPosition fixes the position of the given member in the SortedSpends.
func (s *SortedSpenders[SpenderID, ResourceID, VoteRank]) fixMemberPosition(member *sortedSpender[SpenderID, ResourceID, VoteRank]) {
	preferredSpend := member.PreferredInstead()
	memberIsPreferred := member.IsPreferred()

	// the member needs to be moved up in the list
	for currentMember := member.heavierMember; currentMember != nil && currentMember.Compare(member) == weight.Lighter; currentMember = member.heavierMember {
		s.swapNeighbors(member, currentMember)

		if currentMember == s.heaviestPreferredMember && (preferredSpend == currentMember.Spender || memberIsPreferred || member == s.owner) {
			s.heaviestPreferredMember = member
			s.owner.setPreferredInstead(member.Spender)
		}
	}

	// the member needs to be moved down in the list
	for currentMember := member.lighterMember; currentMember != nil && currentMember.Compare(member) == weight.Heavier; currentMember = member.lighterMember {
		s.swapNeighbors(currentMember, member)

		if member == s.heaviestPreferredMember && (currentMember.IsPreferred() || currentMember.PreferredInstead() == member.Spender || currentMember == s.owner) {
			s.heaviestPreferredMember = currentMember
			s.owner.setPreferredInstead(currentMember.Spender)
		}
	}
}

// notifyPreferredInsteadUpdate notifies the SortedSpends about a member that changed its preferred instead flag.
func (s *SortedSpenders[SpenderID, ResourceID, VoteRank]) notifyPendingPreferredInsteadUpdate(member *sortedSpender[SpenderID, ResourceID, VoteRank]) {
	s.pendingPreferredInsteadMutex.Lock()
	defer s.pendingPreferredInsteadMutex.Unlock()

	if _, exists := s.pendingPreferredInsteadUpdates.Get(member.ID); !exists && !s.isShutdown.Load() {
		s.pendingUpdatesCounter.Increase()
		s.pendingPreferredInsteadUpdates.Set(member.ID, member)
		s.pendingPreferredInsteadSignal.Signal()
	}
}

// fixMemberPositionWorker is a worker that fixes the position of sortedSetMembers that need to be updated.
func (s *SortedSpenders[SpenderID, ResourceID, VoteRank]) fixHeaviestPreferredMemberWorker() {
	for member := s.nextPendingPreferredMemberUpdate(); member != nil; member = s.nextPendingPreferredMemberUpdate() {
		s.applyPreferredInsteadUpdate(member)

		s.pendingUpdatesCounter.Decrease()
	}
}

// nextPendingWeightUpdate returns the next member that needs to be updated (or nil if the shutdown flag is set).
func (s *SortedSpenders[SpenderID, ResourceID, VoteRank]) nextPendingPreferredMemberUpdate() *sortedSpender[SpenderID, ResourceID, VoteRank] {
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
func (s *SortedSpenders[SpenderID, ResourceID, VoteRank]) applyPreferredInsteadUpdate(member *sortedSpender[SpenderID, ResourceID, VoteRank]) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.isShutdown.Load() && member.preferredInsteadUpdateApplied() {
		s.fixHeaviestPreferredMember(member)
	}
}

// fixHeaviestPreferredMember fixes the heaviest preferred member of the SortedSpends after updating the given member.
func (s *SortedSpenders[SpenderID, ResourceID, VoteRank]) fixHeaviestPreferredMember(member *sortedSpender[SpenderID, ResourceID, VoteRank]) {
	if member.IsPreferred() {
		if member.Compare(s.heaviestPreferredMember) == weight.Heavier {
			s.heaviestPreferredMember = member
			s.owner.setPreferredInstead(member.Spender)
		}

		return
	}

	if s.heaviestPreferredMember == member {
		s.findLowerHeaviestPreferredMember(member)
	}
}

func (s *SortedSpenders[SpenderID, ResourceID, VoteRank]) findLowerHeaviestPreferredMember(member *sortedSpender[SpenderID, ResourceID, VoteRank]) {
	for currentMember := member; currentMember != nil; currentMember = currentMember.lighterMember {
		if currentMember == s.owner || currentMember.IsPreferred() || currentMember.PreferredInstead() == member.Spender {
			s.heaviestPreferredMember = currentMember
			s.owner.setPreferredInstead(currentMember.Spender)

			return
		}
	}

	s.heaviestPreferredMember = nil
}

// swapNeighbors swaps the given members in the SortedSpends.
func (s *SortedSpenders[SpenderID, ResourceID, VoteRank]) swapNeighbors(heavierMember *sortedSpender[SpenderID, ResourceID, VoteRank], lighterMember *sortedSpender[SpenderID, ResourceID, VoteRank]) {
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

func (s *SortedSpenders[SpenderID, ResourceID, VoteRank]) Shutdown() []*Spender[SpenderID, ResourceID, VoteRank] {
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

	return lo.Map(s.members.Values(), func(sortedSpender *sortedSpender[SpenderID, ResourceID, VoteRank]) *Spender[SpenderID, ResourceID, VoteRank] {
		return sortedSpender.Spender
	})
}
