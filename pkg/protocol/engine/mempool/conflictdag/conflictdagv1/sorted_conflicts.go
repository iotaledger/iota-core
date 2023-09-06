package conflictdagv1

import (
	"sync"
	"sync/atomic"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/stringify"
	"github.com/iotaledger/iota-core/pkg/core/weight"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
)

// SortedConflicts is a set of Conflicts that is sorted by their weight.
type SortedConflicts[ConflictID, ResourceID conflictdag.IDType, VoteRank conflictdag.VoteRankType[VoteRank]] struct {
	// owner is the Conflict that owns this SortedConflicts.
	owner *sortedConflict[ConflictID, ResourceID, VoteRank]

	// members is a map of ConflictIDs to their corresponding sortedConflict.
	members *shrinkingmap.ShrinkingMap[ConflictID, *sortedConflict[ConflictID, ResourceID, VoteRank]]

	// heaviestMember is the heaviest member of the SortedConflicts.
	heaviestMember *sortedConflict[ConflictID, ResourceID, VoteRank]

	// heaviestPreferredMember is the heaviest preferred member of the SortedConflicts.
	heaviestPreferredMember *sortedConflict[ConflictID, ResourceID, VoteRank]

	// pendingWeightUpdates is a collection of Conflicts that have a pending weight update.
	pendingWeightUpdates *shrinkingmap.ShrinkingMap[ConflictID, *sortedConflict[ConflictID, ResourceID, VoteRank]]

	// pendingWeightUpdatesSignal is a signal that is used to notify the fixMemberPositionWorker about pending weight
	// updates.
	pendingWeightUpdatesSignal *sync.Cond

	// pendingWeightUpdatesMutex is a mutex that is used to synchronize access to the pendingWeightUpdates.
	pendingWeightUpdatesMutex syncutils.RWMutex

	// pendingPreferredInsteadUpdates is a collection of Conflicts that have a pending preferred instead update.
	pendingPreferredInsteadUpdates *shrinkingmap.ShrinkingMap[ConflictID, *sortedConflict[ConflictID, ResourceID, VoteRank]]

	// pendingPreferredInsteadSignal is a signal that is used to notify the fixPreferredInsteadWorker about pending
	// preferred instead updates.
	pendingPreferredInsteadSignal *sync.Cond

	// pendingPreferredInsteadMutex is a mutex that is used to synchronize access to the pendingPreferredInsteadUpdates.
	pendingPreferredInsteadMutex syncutils.RWMutex

	// pendingUpdatesCounter is a counter that keeps track of the number of pending weight updates.
	pendingUpdatesCounter *syncutils.Counter

	// isShutdown is used to signal that the SortedConflicts is shutting down.
	isShutdown atomic.Bool

	// mutex is used to synchronize access to the SortedConflicts.
	mutex syncutils.RWMutex
}

// NewSortedConflicts creates a new SortedConflicts that is owned by the given Conflict.
func NewSortedConflicts[ConflictID, ResourceID conflictdag.IDType, VoteRank conflictdag.VoteRankType[VoteRank]](owner *Conflict[ConflictID, ResourceID, VoteRank], pendingUpdatesCounter *syncutils.Counter) *SortedConflicts[ConflictID, ResourceID, VoteRank] {
	s := &SortedConflicts[ConflictID, ResourceID, VoteRank]{
		members:                        shrinkingmap.New[ConflictID, *sortedConflict[ConflictID, ResourceID, VoteRank]](),
		pendingWeightUpdates:           shrinkingmap.New[ConflictID, *sortedConflict[ConflictID, ResourceID, VoteRank]](),
		pendingUpdatesCounter:          pendingUpdatesCounter,
		pendingPreferredInsteadUpdates: shrinkingmap.New[ConflictID, *sortedConflict[ConflictID, ResourceID, VoteRank]](),
	}
	s.pendingWeightUpdatesSignal = sync.NewCond(&s.pendingWeightUpdatesMutex)
	s.pendingPreferredInsteadSignal = sync.NewCond(&s.pendingPreferredInsteadMutex)

	s.owner = newSortedConflict[ConflictID, ResourceID, VoteRank](s, owner)
	s.members.Set(owner.ID, s.owner)

	s.heaviestMember = s.owner
	s.heaviestPreferredMember = s.owner

	go s.fixMemberPositionWorker()
	go s.fixHeaviestPreferredMemberWorker()

	return s
}

// Add adds the given Conflict to the SortedConflicts.
func (s *SortedConflicts[ConflictID, ResourceID, VoteRank]) Add(conflict *Conflict[ConflictID, ResourceID, VoteRank]) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.isShutdown.Load() {
		return false
	}

	newMember, isNew := s.members.GetOrCreate(conflict.ID, func() *sortedConflict[ConflictID, ResourceID, VoteRank] {
		return newSortedConflict(s, conflict)
	})
	if !isNew {
		return false
	}

	for currentMember := s.heaviestMember; ; currentMember = currentMember.lighterMember {
		comparison := newMember.Compare(currentMember)
		if comparison == weight.Equal {
			panic("different Conflicts should never have the same weight")
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

// ForEach iterates over all Conflicts of the SortedConflicts and calls the given callback for each of them.
func (s *SortedConflicts[ConflictID, ResourceID, VoteRank]) ForEach(callback func(*Conflict[ConflictID, ResourceID, VoteRank]) error, optIncludeOwner ...bool) error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for currentMember := s.heaviestMember; currentMember != nil; currentMember = currentMember.lighterMember {
		if !lo.First(optIncludeOwner) && currentMember == s.owner {
			continue
		}

		if err := callback(currentMember.Conflict); err != nil {
			return err
		}
	}

	return nil
}

// Range iterates over all Conflicts of the SortedConflicts and calls the given callback for each of them (without
// manual error handling).
func (s *SortedConflicts[ConflictID, ResourceID, VoteRank]) Range(callback func(*Conflict[ConflictID, ResourceID, VoteRank]), optIncludeOwner ...bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for currentMember := s.heaviestMember; currentMember != nil; currentMember = currentMember.lighterMember {
		if !lo.First(optIncludeOwner) && currentMember == s.owner {
			continue
		}

		callback(currentMember.Conflict)
	}
}

// Remove removes the Conflict with the given ID from the SortedConflicts.
func (s *SortedConflicts[ConflictID, ResourceID, VoteRank]) Remove(id ConflictID) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	conflict, exists := s.members.Get(id)
	if !exists || !s.members.Delete(id) {
		return false
	}

	conflict.Unhook()

	if conflict.heavierMember != nil {
		conflict.heavierMember.lighterMember = conflict.lighterMember
	}

	if conflict.lighterMember != nil {
		conflict.lighterMember.heavierMember = conflict.heavierMember
	}

	if s.heaviestMember == conflict {
		s.heaviestMember = conflict.lighterMember
	}

	if s.heaviestPreferredMember == conflict {
		s.findLowerHeaviestPreferredMember(conflict.lighterMember)
	}

	conflict.lighterMember = nil
	conflict.heavierMember = nil

	return true
}

// String returns a human-readable representation of the SortedConflicts.
func (s *SortedConflicts[ConflictID, ResourceID, VoteRank]) String() string {
	structBuilder := stringify.NewStructBuilder("SortedConflicts",
		stringify.NewStructField("owner", s.owner.ID),
		stringify.NewStructField("heaviestMember", s.heaviestMember.ID),
		stringify.NewStructField("heaviestPreferredMember", s.heaviestPreferredMember.ID),
	)

	s.Range(func(conflict *Conflict[ConflictID, ResourceID, VoteRank]) {
		structBuilder.AddField(stringify.NewStructField(conflict.ID.String(), conflict))
	}, true)

	return structBuilder.String()
}

// notifyPendingWeightUpdate notifies the SortedConflicts about a pending weight update of the given member.
func (s *SortedConflicts[ConflictID, ResourceID, VoteRank]) notifyPendingWeightUpdate(member *sortedConflict[ConflictID, ResourceID, VoteRank]) {
	s.pendingWeightUpdatesMutex.Lock()
	defer s.pendingWeightUpdatesMutex.Unlock()

	if _, exists := s.pendingWeightUpdates.Get(member.ID); !exists && !s.isShutdown.Load() {
		s.pendingUpdatesCounter.Increase()
		s.pendingWeightUpdates.Set(member.ID, member)
		s.pendingWeightUpdatesSignal.Signal()
	}
}

// fixMemberPositionWorker is a worker that fixes the position of sortedSetMembers that need to be updated.
func (s *SortedConflicts[ConflictID, ResourceID, VoteRank]) fixMemberPositionWorker() {
	for member := s.nextPendingWeightUpdate(); member != nil; member = s.nextPendingWeightUpdate() {
		s.applyWeightUpdate(member)

		s.pendingUpdatesCounter.Decrease()
	}
}

// nextPendingWeightUpdate returns the next member that needs to be updated (or nil if the shutdown flag is set).
func (s *SortedConflicts[ConflictID, ResourceID, VoteRank]) nextPendingWeightUpdate() *sortedConflict[ConflictID, ResourceID, VoteRank] {
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
func (s *SortedConflicts[ConflictID, ResourceID, VoteRank]) applyWeightUpdate(member *sortedConflict[ConflictID, ResourceID, VoteRank]) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.isShutdown.Load() && member.weightUpdateApplied() {
		s.fixMemberPosition(member)
	}
}

// fixMemberPosition fixes the position of the given member in the SortedConflicts.
func (s *SortedConflicts[ConflictID, ResourceID, VoteRank]) fixMemberPosition(member *sortedConflict[ConflictID, ResourceID, VoteRank]) {
	preferredConflict := member.PreferredInstead()
	memberIsPreferred := member.IsPreferred()

	// the member needs to be moved up in the list
	for currentMember := member.heavierMember; currentMember != nil && currentMember.Compare(member) == weight.Lighter; currentMember = member.heavierMember {
		s.swapNeighbors(member, currentMember)

		if currentMember == s.heaviestPreferredMember && (preferredConflict == currentMember.Conflict || memberIsPreferred || member == s.owner) {
			s.heaviestPreferredMember = member
			s.owner.setPreferredInstead(member.Conflict)
		}
	}

	// the member needs to be moved down in the list
	for currentMember := member.lighterMember; currentMember != nil && currentMember.Compare(member) == weight.Heavier; currentMember = member.lighterMember {
		s.swapNeighbors(currentMember, member)

		if member == s.heaviestPreferredMember && (currentMember.IsPreferred() || currentMember.PreferredInstead() == member.Conflict || currentMember == s.owner) {
			s.heaviestPreferredMember = currentMember
			s.owner.setPreferredInstead(currentMember.Conflict)
		}
	}
}

// notifyPreferredInsteadUpdate notifies the SortedConflicts about a member that changed its preferred instead flag.
func (s *SortedConflicts[ConflictID, ResourceID, VoteRank]) notifyPendingPreferredInsteadUpdate(member *sortedConflict[ConflictID, ResourceID, VoteRank]) {
	s.pendingPreferredInsteadMutex.Lock()
	defer s.pendingPreferredInsteadMutex.Unlock()

	if _, exists := s.pendingPreferredInsteadUpdates.Get(member.ID); !exists && !s.isShutdown.Load() {
		s.pendingUpdatesCounter.Increase()
		s.pendingPreferredInsteadUpdates.Set(member.ID, member)
		s.pendingPreferredInsteadSignal.Signal()
	}
}

// fixMemberPositionWorker is a worker that fixes the position of sortedSetMembers that need to be updated.
func (s *SortedConflicts[ConflictID, ResourceID, VoteRank]) fixHeaviestPreferredMemberWorker() {
	for member := s.nextPendingPreferredMemberUpdate(); member != nil; member = s.nextPendingPreferredMemberUpdate() {
		s.applyPreferredInsteadUpdate(member)

		s.pendingUpdatesCounter.Decrease()
	}
}

// nextPendingWeightUpdate returns the next member that needs to be updated (or nil if the shutdown flag is set).
func (s *SortedConflicts[ConflictID, ResourceID, VoteRank]) nextPendingPreferredMemberUpdate() *sortedConflict[ConflictID, ResourceID, VoteRank] {
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
func (s *SortedConflicts[ConflictID, ResourceID, VoteRank]) applyPreferredInsteadUpdate(member *sortedConflict[ConflictID, ResourceID, VoteRank]) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.isShutdown.Load() && member.preferredInsteadUpdateApplied() {
		s.fixHeaviestPreferredMember(member)
	}
}

// fixHeaviestPreferredMember fixes the heaviest preferred member of the SortedConflicts after updating the given member.
func (s *SortedConflicts[ConflictID, ResourceID, VoteRank]) fixHeaviestPreferredMember(member *sortedConflict[ConflictID, ResourceID, VoteRank]) {
	if member.IsPreferred() {
		if member.Compare(s.heaviestPreferredMember) == weight.Heavier {
			s.heaviestPreferredMember = member
			s.owner.setPreferredInstead(member.Conflict)
		}

		return
	}

	if s.heaviestPreferredMember == member {
		s.findLowerHeaviestPreferredMember(member)
	}
}

func (s *SortedConflicts[ConflictID, ResourceID, VoteRank]) findLowerHeaviestPreferredMember(member *sortedConflict[ConflictID, ResourceID, VoteRank]) {
	for currentMember := member; currentMember != nil; currentMember = currentMember.lighterMember {
		if currentMember == s.owner || currentMember.IsPreferred() || currentMember.PreferredInstead() == member.Conflict {
			s.heaviestPreferredMember = currentMember
			s.owner.setPreferredInstead(currentMember.Conflict)

			return
		}
	}

	s.heaviestPreferredMember = nil
}

// swapNeighbors swaps the given members in the SortedConflicts.
func (s *SortedConflicts[ConflictID, ResourceID, VoteRank]) swapNeighbors(heavierMember, lighterMember *sortedConflict[ConflictID, ResourceID, VoteRank]) {
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

func (s *SortedConflicts[ConflictID, ResourceID, VoteRank]) Shutdown() []*Conflict[ConflictID, ResourceID, VoteRank] {
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

	return lo.Map(s.members.Values(), func(conflict *sortedConflict[ConflictID, ResourceID, VoteRank]) *Conflict[ConflictID, ResourceID, VoteRank] {
		return conflict.Conflict
	})
}
