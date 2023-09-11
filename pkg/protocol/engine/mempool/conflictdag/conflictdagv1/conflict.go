package conflictdagv1

import (
	"bytes"

	"go.uber.org/atomic"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/stringify"
	"github.com/iotaledger/iota-core/pkg/core/acceptance"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/core/weight"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
)

// Conflict is a conflict that is part of a Conflict DAG.
type Conflict[ConflictID, ResourceID conflictdag.IDType, VoteRank conflictdag.VoteRankType[VoteRank]] struct {
	// ID is the identifier of the Conflict.
	ID ConflictID

	// Parents is the set of parents of the Conflict.
	Parents ds.Set[*Conflict[ConflictID, ResourceID, VoteRank]]

	// Children is the set of children of the Conflict.
	Children ds.Set[*Conflict[ConflictID, ResourceID, VoteRank]]

	// ConflictSets is the set of ConflictSets that the Conflict is part of.
	ConflictSets ds.Set[*ConflictSet[ConflictID, ResourceID, VoteRank]]

	// ConflictingConflicts is the set of conflicts that directly conflict with the Conflict.
	ConflictingConflicts *SortedConflicts[ConflictID, ResourceID, VoteRank]

	// Weight is the Weight of the Conflict.
	Weight *weight.Weight

	// LatestVotes is the set of the latest votes of the Conflict.
	LatestVotes *shrinkingmap.ShrinkingMap[account.SeatIndex, *vote.Vote[VoteRank]]

	// AcceptanceStateUpdated is triggered when the AcceptanceState of the Conflict is updated.
	AcceptanceStateUpdated *event.Event2[acceptance.State, acceptance.State]

	// PreferredInsteadUpdated is triggered when the preferred instead value of the Conflict is updated.
	PreferredInsteadUpdated *event.Event1[*Conflict[ConflictID, ResourceID, VoteRank]]

	// LikedInsteadAdded is triggered when a liked instead reference is added to the Conflict.
	LikedInsteadAdded *event.Event1[*Conflict[ConflictID, ResourceID, VoteRank]]

	// LikedInsteadRemoved is triggered when a liked instead reference is removed from the Conflict.
	LikedInsteadRemoved *event.Event1[*Conflict[ConflictID, ResourceID, VoteRank]]

	// childUnhookMethods is a mapping of children to their unhook functions.
	childUnhookMethods *shrinkingmap.ShrinkingMap[ConflictID, func()]

	// preferredInstead is the preferred instead value of the Conflict.
	preferredInstead *Conflict[ConflictID, ResourceID, VoteRank]

	// evicted
	evicted atomic.Bool

	// preferredInsteadMutex is used to synchronize access to the preferred instead value of the Conflict.
	preferredInsteadMutex syncutils.RWMutex

	// likedInstead is the set of liked instead Conflicts.
	likedInstead ds.Set[*Conflict[ConflictID, ResourceID, VoteRank]]

	// likedInsteadSources is a mapping of liked instead Conflicts to the set of parents that inherited them.
	likedInsteadSources *shrinkingmap.ShrinkingMap[ConflictID, ds.Set[*Conflict[ConflictID, ResourceID, VoteRank]]]

	// likedInsteadMutex and structureMutex are sometimes locked in different order by different goroutines, which could result in a deadlock
	//  however, it's impossible to deadlock if we fork all transactions upon booking
	//  deadlock happens when the likedInstead conflict changes and parents are updated at the same time, which is impossible in the current setup
	//  because we won't process votes on a conflict we're just creating.
	// likedInsteadMutex is used to synchronize access to the liked instead value of the Conflict.
	likedInsteadMutex syncutils.RWMutex

	// structureMutex is used to synchronize access to the structure of the Conflict.
	structureMutex syncutils.RWMutex

	// acceptanceThreshold is the function that is used to retrieve the acceptance threshold of the committee.
	acceptanceThreshold func() int64

	// unhookAcceptanceMonitoring
	unhookAcceptanceMonitoring func()
}

// NewConflict creates a new Conflict.
func NewConflict[ConflictID, ResourceID conflictdag.IDType, VoteRank conflictdag.VoteRankType[VoteRank]](id ConflictID, initialWeight *weight.Weight, pendingTasksCounter *syncutils.Counter, acceptanceThresholdProvider func() int64) *Conflict[ConflictID, ResourceID, VoteRank] {
	c := &Conflict[ConflictID, ResourceID, VoteRank]{
		ID:                      id,
		Parents:                 ds.NewSet[*Conflict[ConflictID, ResourceID, VoteRank]](),
		Children:                ds.NewSet[*Conflict[ConflictID, ResourceID, VoteRank]](),
		ConflictSets:            ds.NewSet[*ConflictSet[ConflictID, ResourceID, VoteRank]](),
		Weight:                  initialWeight,
		LatestVotes:             shrinkingmap.New[account.SeatIndex, *vote.Vote[VoteRank]](),
		AcceptanceStateUpdated:  event.New2[acceptance.State, acceptance.State](),
		PreferredInsteadUpdated: event.New1[*Conflict[ConflictID, ResourceID, VoteRank]](),
		LikedInsteadAdded:       event.New1[*Conflict[ConflictID, ResourceID, VoteRank]](),
		LikedInsteadRemoved:     event.New1[*Conflict[ConflictID, ResourceID, VoteRank]](),

		childUnhookMethods:  shrinkingmap.New[ConflictID, func()](),
		acceptanceThreshold: acceptanceThresholdProvider,
		likedInstead:        ds.NewSet[*Conflict[ConflictID, ResourceID, VoteRank]](),
		likedInsteadSources: shrinkingmap.New[ConflictID, ds.Set[*Conflict[ConflictID, ResourceID, VoteRank]]](),
	}

	c.preferredInstead = c

	c.unhookAcceptanceMonitoring = c.Weight.OnUpdate.Hook(func(value weight.Value) {
		if value.AcceptanceState().IsPending() && value.ValidatorsWeight() >= c.acceptanceThreshold() {
			c.setAcceptanceState(acceptance.Accepted)
		}
	}).Unhook

	// in case the initial weight is enough to accept the conflict, accept it immediately
	if threshold := c.acceptanceThreshold(); initialWeight.Value().ValidatorsWeight() >= threshold {
		c.setAcceptanceState(acceptance.Accepted)
	}

	c.ConflictingConflicts = NewSortedConflicts(c, pendingTasksCounter)

	return c
}

// JoinConflictSets registers the Conflict with the given ConflictSets.
func (c *Conflict[ConflictID, ResourceID, VoteRank]) JoinConflictSets(conflictSets ds.Set[*ConflictSet[ConflictID, ResourceID, VoteRank]]) (joinedConflictSets ds.Set[ResourceID], err error) {
	if conflictSets == nil {
		return ds.NewSet[ResourceID](), nil
	}

	if c.evicted.Load() {
		return nil, ierrors.Errorf("tried to join conflict sets of evicted conflict: %w", conflictdag.ErrEntityEvicted)
	}

	registerConflictingConflict := func(c, conflict *Conflict[ConflictID, ResourceID, VoteRank]) {
		c.structureMutex.Lock()
		defer c.structureMutex.Unlock()

		if c.ConflictingConflicts.Add(conflict) {
			if conflict.IsAccepted() {
				c.setAcceptanceState(acceptance.Rejected)
			}
		}
	}

	joinedConflictSets = ds.NewSet[ResourceID]()

	return joinedConflictSets, conflictSets.ForEach(func(conflictSet *ConflictSet[ConflictID, ResourceID, VoteRank]) error {
		otherConflicts, err := conflictSet.Add(c)
		if err != nil && !ierrors.Is(err, conflictdag.ErrAlreadyPartOfConflictSet) {
			return err
		}

		if c.ConflictSets.Add(conflictSet) {
			if otherConflicts != nil {
				otherConflicts.Range(func(otherConflict *Conflict[ConflictID, ResourceID, VoteRank]) {
					registerConflictingConflict(c, otherConflict)
					registerConflictingConflict(otherConflict, c)
				})

				joinedConflictSets.Add(conflictSet.ID)
			}
		}

		return nil
	})
}

func (c *Conflict[ConflictID, ResourceID, VoteRank]) removeParent(parent *Conflict[ConflictID, ResourceID, VoteRank]) (removed bool) {
	if removed = c.Parents.Delete(parent); removed {
		parent.unregisterChild(c)
	}

	return removed
}

// UpdateParents updates the parents of the Conflict.
func (c *Conflict[ConflictID, ResourceID, VoteRank]) UpdateParents(addedParents, removedParents ds.Set[*Conflict[ConflictID, ResourceID, VoteRank]]) (updated bool) {
	c.structureMutex.Lock()
	defer c.structureMutex.Unlock()

	if removedParents != nil {
		removedParents.Range(func(removedParent *Conflict[ConflictID, ResourceID, VoteRank]) {
			updated = c.removeParent(removedParent) || updated
		})
	}

	if addedParents != nil {
		addedParents.Range(func(addedParent *Conflict[ConflictID, ResourceID, VoteRank]) {
			if c.Parents.Add(addedParent) {
				addedParent.registerChild(c)

				updated = true
			}
		})
	}

	return updated
}

func (c *Conflict[ConflictID, ResourceID, VoteRank]) ApplyVote(vote *vote.Vote[VoteRank]) {
	// abort if the conflict has already been accepted or rejected
	if !c.Weight.AcceptanceState().IsPending() {
		return
	}

	// abort if we have another vote from the same validator with higher power
	latestVote, exists := c.LatestVotes.Get(vote.Voter)
	if exists && latestVote.Rank.Compare(vote.Rank) >= 0 {
		return
	}

	// update the latest vote
	c.LatestVotes.Set(vote.Voter, vote)

	// abort if the vote does not change the opinion of the validator
	if exists && latestVote.IsLiked() == vote.IsLiked() {
		return
	}

	if vote.IsLiked() {
		c.Weight.AddVoter(vote.Voter)
	} else {
		c.Weight.DeleteVoter(vote.Voter)
	}
}

// IsPending returns true if the Conflict is pending.
func (c *Conflict[ConflictID, ResourceID, VoteRank]) IsPending() bool {
	return c.Weight.Value().AcceptanceState().IsPending()
}

// IsAccepted returns true if the Conflict is accepted.
func (c *Conflict[ConflictID, ResourceID, VoteRank]) IsAccepted() bool {
	return c.Weight.Value().AcceptanceState().IsAccepted()
}

// IsRejected returns true if the Conflict is rejected.
func (c *Conflict[ConflictID, ResourceID, VoteRank]) IsRejected() bool {
	return c.Weight.Value().AcceptanceState().IsRejected()
}

// IsPreferred returns true if the Conflict is preferred instead of its conflicting Conflicts.
func (c *Conflict[ConflictID, ResourceID, VoteRank]) IsPreferred() bool {
	c.preferredInsteadMutex.RLock()
	defer c.preferredInsteadMutex.RUnlock()

	return c.preferredInstead == c
}

// PreferredInstead returns the preferred instead value of the Conflict.
func (c *Conflict[ConflictID, ResourceID, VoteRank]) PreferredInstead() *Conflict[ConflictID, ResourceID, VoteRank] {
	c.preferredInsteadMutex.RLock()
	defer c.preferredInsteadMutex.RUnlock()

	return c.preferredInstead
}

// IsLiked returns true if the Conflict is liked instead of other conflicting Conflicts.
func (c *Conflict[ConflictID, ResourceID, VoteRank]) IsLiked() bool {
	c.likedInsteadMutex.RLock()
	defer c.likedInsteadMutex.RUnlock()

	return c.IsPreferred() && c.likedInstead.IsEmpty()
}

// LikedInstead returns the set of liked instead Conflicts.
func (c *Conflict[ConflictID, ResourceID, VoteRank]) LikedInstead() ds.Set[*Conflict[ConflictID, ResourceID, VoteRank]] {
	c.likedInsteadMutex.RLock()
	defer c.likedInsteadMutex.RUnlock()

	return c.likedInstead.Clone()
}

// Shutdown shuts down the Conflict.
func (c *Conflict[ConflictID, ResourceID, VoteRank]) Shutdown() {
	c.ConflictingConflicts.Shutdown()
}

// Evict cleans up the sortedConflict.
func (c *Conflict[ConflictID, ResourceID, VoteRank]) Evict() (evictedConflicts []ConflictID) {
	if firstEvictCall := !c.evicted.Swap(true); !firstEvictCall {
		return nil
	}

	c.unhookAcceptanceMonitoring()

	switch c.Weight.AcceptanceState() {
	case acceptance.Rejected:
		// evict the entire future cone of rejected conflicts
		c.Children.Range(func(childConflict *Conflict[ConflictID, ResourceID, VoteRank]) {
			evictedConflicts = append(evictedConflicts, childConflict.Evict()...)
		})
	default:
		// remove evicted conflict from parents of children (merge to master)
		c.Children.Range(func(childConflict *Conflict[ConflictID, ResourceID, VoteRank]) {
			childConflict.structureMutex.Lock()
			defer childConflict.structureMutex.Unlock()

			childConflict.removeParent(c)
		})
	}

	c.structureMutex.Lock()
	defer c.structureMutex.Unlock()

	c.Parents.Range(func(parentConflict *Conflict[ConflictID, ResourceID, VoteRank]) {
		parentConflict.unregisterChild(c)
	})
	c.Parents.Clear()

	c.ConflictSets.Range(func(conflictSet *ConflictSet[ConflictID, ResourceID, VoteRank]) {
		conflictSet.Remove(c)
	})
	c.ConflictSets.Clear()

	for _, conflict := range c.ConflictingConflicts.Shutdown() {
		if conflict != c {
			conflict.ConflictingConflicts.Remove(c.ID)
			c.ConflictingConflicts.Remove(conflict.ID)

			if c.IsAccepted() {
				evictedConflicts = append(evictedConflicts, conflict.Evict()...)
			}
		}
	}

	c.ConflictingConflicts.Remove(c.ID)

	c.preferredInsteadMutex.Lock()
	defer c.preferredInsteadMutex.Unlock()
	c.likedInsteadMutex.Lock()
	defer c.likedInsteadMutex.Unlock()

	c.likedInsteadSources.Clear()
	c.preferredInstead = nil

	evictedConflicts = append(evictedConflicts, c.ID)

	return evictedConflicts
}

// Compare compares the Conflict to the given other Conflict.
func (c *Conflict[ConflictID, ResourceID, VoteRank]) Compare(other *Conflict[ConflictID, ResourceID, VoteRank]) int {
	// no need to lock a mutex here, because the Weight is already thread-safe

	if c == other {
		return weight.Equal
	}

	if other == nil {
		return weight.Heavier
	}

	if c == nil {
		return weight.Lighter
	}

	if result := c.Weight.Compare(other.Weight); result != weight.Equal {
		return result
	}

	return bytes.Compare(lo.PanicOnErr(c.ID.Bytes()), lo.PanicOnErr(other.ID.Bytes()))
}

// String returns a human-readable representation of the Conflict.
func (c *Conflict[ConflictID, ResourceID, VoteRank]) String() string {
	// no need to lock a mutex here, because the Weight is already thread-safe

	return stringify.Struct("Conflict",
		stringify.NewStructField("id", c.ID),
		stringify.NewStructField("weight", c.Weight),
	)
}

// registerChild registers the given child Conflict.
func (c *Conflict[ConflictID, ResourceID, VoteRank]) registerChild(child *Conflict[ConflictID, ResourceID, VoteRank]) {
	c.structureMutex.Lock()
	defer c.structureMutex.Unlock()

	if c.Children.Add(child) {
		// hold likedInsteadMutex while determining our liked instead state
		c.likedInsteadMutex.Lock()
		defer c.likedInsteadMutex.Unlock()

		c.childUnhookMethods.Set(child.ID, lo.Batch(
			c.AcceptanceStateUpdated.Hook(func(_, newState acceptance.State) {
				if newState.IsRejected() {
					child.setAcceptanceState(newState)
				}
			}).Unhook,

			c.LikedInsteadRemoved.Hook(func(reference *Conflict[ConflictID, ResourceID, VoteRank]) {
				child.removeInheritedLikedInsteadReference(c, reference)
			}).Unhook,

			c.LikedInsteadAdded.Hook(func(conflict *Conflict[ConflictID, ResourceID, VoteRank]) {
				child.structureMutex.Lock()
				defer child.structureMutex.Unlock()

				child.addInheritedLikedInsteadReference(c, conflict)
			}).Unhook,
		))

		for conflicts := c.likedInstead.Iterator(); conflicts.HasNext(); {
			child.addInheritedLikedInsteadReference(c, conflicts.Next())
		}

		if c.IsRejected() {
			child.setAcceptanceState(acceptance.Rejected)
		}
	}
}

// unregisterChild unregisters the given child Conflict.
func (c *Conflict[ConflictID, ResourceID, VoteRank]) unregisterChild(conflict *Conflict[ConflictID, ResourceID, VoteRank]) {
	c.structureMutex.Lock()
	defer c.structureMutex.Unlock()

	if c.Children.Delete(conflict) {
		if unhookFunc, exists := c.childUnhookMethods.Get(conflict.ID); exists {
			c.childUnhookMethods.Delete(conflict.ID)

			unhookFunc()
		}
	}
}

// addInheritedLikedInsteadReference adds the given reference as a liked instead reference from the given source.
func (c *Conflict[ConflictID, ResourceID, VoteRank]) addInheritedLikedInsteadReference(source, reference *Conflict[ConflictID, ResourceID, VoteRank]) {
	c.likedInsteadMutex.Lock()
	defer c.likedInsteadMutex.Unlock()

	// abort if the source already added the reference or if the source already existed
	if sources := lo.Return1(c.likedInsteadSources.GetOrCreate(reference.ID, lo.NoVariadic(ds.NewSet[*Conflict[ConflictID, ResourceID, VoteRank]]))); !sources.Add(source) || !c.likedInstead.Add(reference) {
		return
	}

	// remove the "preferred instead reference" (that might have been set as a default)
	if preferredInstead := c.PreferredInstead(); c.likedInstead.Delete(preferredInstead) {
		c.LikedInsteadRemoved.Trigger(preferredInstead)
	}

	// trigger within the scope of the lock to ensure the correct queueing order
	c.LikedInsteadAdded.Trigger(reference)
}

// removeInheritedLikedInsteadReference removes the given reference as a liked instead reference from the given source.
func (c *Conflict[ConflictID, ResourceID, VoteRank]) removeInheritedLikedInsteadReference(source, reference *Conflict[ConflictID, ResourceID, VoteRank]) {
	c.likedInsteadMutex.Lock()
	defer c.likedInsteadMutex.Unlock()

	// abort if the reference did not exist
	if sources, sourcesExist := c.likedInsteadSources.Get(reference.ID); !sourcesExist || !sources.Delete(source) || !sources.IsEmpty() || !c.likedInsteadSources.Delete(reference.ID) || !c.likedInstead.Delete(reference) {
		return
	}

	// trigger within the scope of the lock to ensure the correct queueing order
	c.LikedInsteadRemoved.Trigger(reference)

	// fall back to preferred instead if not preferred and parents are liked
	if preferredInstead := c.PreferredInstead(); c.likedInstead.IsEmpty() && preferredInstead != c {
		c.likedInstead.Add(preferredInstead)

		// trigger within the scope of the lock to ensure the correct queueing order
		c.LikedInsteadAdded.Trigger(preferredInstead)
	}
}

// setPreferredInstead sets the preferred instead value of the Conflict.
func (c *Conflict[ConflictID, ResourceID, VoteRank]) setPreferredInstead(preferredInstead *Conflict[ConflictID, ResourceID, VoteRank]) (previousPreferredInstead *Conflict[ConflictID, ResourceID, VoteRank]) {
	c.likedInsteadMutex.Lock()
	defer c.likedInsteadMutex.Unlock()

	if func() (updated bool) {
		c.preferredInsteadMutex.Lock()
		defer c.preferredInsteadMutex.Unlock()

		if previousPreferredInstead, updated = c.preferredInstead, previousPreferredInstead != preferredInstead; updated {
			c.preferredInstead = preferredInstead
			c.PreferredInsteadUpdated.Trigger(preferredInstead)
		}

		return updated
	}() {
		if c.likedInstead.Delete(previousPreferredInstead) {
			// trigger within the scope of the lock to ensure the correct queueing order
			c.LikedInsteadRemoved.Trigger(previousPreferredInstead)
		}

		if !c.IsPreferred() && c.likedInstead.IsEmpty() {
			c.likedInstead.Add(preferredInstead)

			// trigger within the scope of the lock to ensure the correct queueing order
			c.LikedInsteadAdded.Trigger(preferredInstead)
		}
	}

	return previousPreferredInstead
}

// setAcceptanceState sets the acceptance state of the Conflict and returns the previous acceptance state (it triggers
// an AcceptanceStateUpdated event if the acceptance state was updated).
func (c *Conflict[ConflictID, ResourceID, VoteRank]) setAcceptanceState(newState acceptance.State) (previousState acceptance.State) {
	if previousState = c.Weight.SetAcceptanceState(newState); previousState == newState {
		return previousState
	}

	// propagate acceptance to parents first
	if newState.IsAccepted() {
		c.Parents.Range(func(parent *Conflict[ConflictID, ResourceID, VoteRank]) {
			parent.setAcceptanceState(acceptance.Accepted)
		})
	}

	c.AcceptanceStateUpdated.Trigger(previousState, newState)

	return previousState
}
