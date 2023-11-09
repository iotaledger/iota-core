package spenddagv1

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
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
)

// Spend is a spend that is part of a Spend DAG.
type Spend[SpendID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]] struct {
	// ID is the identifier of the Spend.
	ID SpendID

	// Parents is the set of parents of the Spend.
	Parents ds.Set[*Spend[SpendID, ResourceID, VoteRank]]

	// Children is the set of children of the Spend.
	Children ds.Set[*Spend[SpendID, ResourceID, VoteRank]]

	// ConflictSets is the set of ConflictSets that the Spend is part of.
	ConflictSets ds.Set[*ConflictSet[SpendID, ResourceID, VoteRank]]

	// ConflictingSpends is the set of spends that directly conflict with the Spend.
	ConflictingSpends *SortedSpends[SpendID, ResourceID, VoteRank]

	// Weight is the Weight of the Spend.
	Weight *weight.Weight

	// LatestVotes is the set of the latest votes of the Spend.
	LatestVotes *shrinkingmap.ShrinkingMap[account.SeatIndex, *vote.Vote[VoteRank]]

	// AcceptanceStateUpdated is triggered when the AcceptanceState of the Spend is updated.
	AcceptanceStateUpdated *event.Event2[acceptance.State, acceptance.State]

	// PreferredInsteadUpdated is triggered when the preferred instead value of the Spend is updated.
	PreferredInsteadUpdated *event.Event1[*Spend[SpendID, ResourceID, VoteRank]]

	// LikedInsteadAdded is triggered when a liked instead reference is added to the Spend.
	LikedInsteadAdded *event.Event1[*Spend[SpendID, ResourceID, VoteRank]]

	// LikedInsteadRemoved is triggered when a liked instead reference is removed from the Spend.
	LikedInsteadRemoved *event.Event1[*Spend[SpendID, ResourceID, VoteRank]]

	// childUnhookMethods is a mapping of children to their unhook functions.
	childUnhookMethods *shrinkingmap.ShrinkingMap[SpendID, func()]

	// preferredInstead is the preferred instead value of the Spend.
	preferredInstead *Spend[SpendID, ResourceID, VoteRank]

	// evicted
	evicted atomic.Bool

	// preferredInsteadMutex is used to synchronize access to the preferred instead value of the Spend.
	preferredInsteadMutex syncutils.RWMutex

	// likedInstead is the set of liked instead Spends.
	likedInstead ds.Set[*Spend[SpendID, ResourceID, VoteRank]]

	// likedInsteadSources is a mapping of liked instead Spends to the set of parents that inherited them.
	likedInsteadSources *shrinkingmap.ShrinkingMap[SpendID, ds.Set[*Spend[SpendID, ResourceID, VoteRank]]]

	// likedInsteadMutex and structureMutex are sometimes locked in different order by different goroutines, which could result in a deadlock
	//  however, it's impossible to deadlock if we fork all transactions upon booking
	//  deadlock happens when the likedInstead conflict changes and parents are updated at the same time, which is impossible in the current setup
	//  because we won't process votes on a conflict we're just creating.
	// likedInsteadMutex is used to synchronize access to the liked instead value of the Spend.
	likedInsteadMutex syncutils.RWMutex

	// structureMutex is used to synchronize access to the structure of the Spend.
	structureMutex syncutils.RWMutex

	// acceptanceThreshold is the function that is used to retrieve the acceptance threshold of the committee.
	acceptanceThreshold func() int64

	// unhookAcceptanceMonitoring
	unhookAcceptanceMonitoring func()
}

// NewSpend creates a new Spend.
func NewSpend[SpendID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]](id SpendID, initialWeight *weight.Weight, pendingTasksCounter *syncutils.Counter, acceptanceThresholdProvider func() int64) *Spend[SpendID, ResourceID, VoteRank] {
	c := &Spend[SpendID, ResourceID, VoteRank]{
		ID:                      id,
		Parents:                 ds.NewSet[*Spend[SpendID, ResourceID, VoteRank]](),
		Children:                ds.NewSet[*Spend[SpendID, ResourceID, VoteRank]](),
		ConflictSets:            ds.NewSet[*ConflictSet[SpendID, ResourceID, VoteRank]](),
		Weight:                  initialWeight,
		LatestVotes:             shrinkingmap.New[account.SeatIndex, *vote.Vote[VoteRank]](),
		AcceptanceStateUpdated:  event.New2[acceptance.State, acceptance.State](),
		PreferredInsteadUpdated: event.New1[*Spend[SpendID, ResourceID, VoteRank]](),
		LikedInsteadAdded:       event.New1[*Spend[SpendID, ResourceID, VoteRank]](),
		LikedInsteadRemoved:     event.New1[*Spend[SpendID, ResourceID, VoteRank]](),

		childUnhookMethods:  shrinkingmap.New[SpendID, func()](),
		acceptanceThreshold: acceptanceThresholdProvider,
		likedInstead:        ds.NewSet[*Spend[SpendID, ResourceID, VoteRank]](),
		likedInsteadSources: shrinkingmap.New[SpendID, ds.Set[*Spend[SpendID, ResourceID, VoteRank]]](),
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

	c.ConflictingSpends = NewSortedSpends(c, pendingTasksCounter)

	return c
}

// JoinConflictSets registers the Spend with the given ConflictSets.
func (c *Spend[SpendID, ResourceID, VoteRank]) JoinSpendSets(conflictSets ds.Set[*ConflictSet[SpendID, ResourceID, VoteRank]]) (joinedConflictSets ds.Set[ResourceID], err error) {
	if conflictSets == nil {
		return ds.NewSet[ResourceID](), nil
	}

	if c.evicted.Load() {
		return nil, ierrors.Errorf("tried to join conflict sets of evicted spend: %w", spenddag.ErrEntityEvicted)
	}

	registerConflictingSpend := func(c *Spend[SpendID, ResourceID, VoteRank], spend *Spend[SpendID, ResourceID, VoteRank]) {
		c.structureMutex.Lock()
		defer c.structureMutex.Unlock()

		if c.ConflictingSpends.Add(spend) {
			if spend.IsAccepted() {
				c.setAcceptanceState(acceptance.Rejected)
			}
		}
	}

	joinedConflictSets = ds.NewSet[ResourceID]()

	return joinedConflictSets, conflictSets.ForEach(func(conflictSet *ConflictSet[SpendID, ResourceID, VoteRank]) error {
		otherConflicts, err := conflictSet.Add(c)
		if err != nil && !ierrors.Is(err, spenddag.ErrAlreadyPartOfConflictSet) {
			return err
		}

		if c.ConflictSets.Add(conflictSet) {
			if otherConflicts != nil {
				otherConflicts.Range(func(otherConflict *Spend[SpendID, ResourceID, VoteRank]) {
					registerConflictingSpend(c, otherConflict)
					registerConflictingSpend(otherConflict, c)
				})

				joinedConflictSets.Add(conflictSet.ID)
			}
		}

		return nil
	})
}

func (c *Spend[SpendID, ResourceID, VoteRank]) removeParent(parent *Spend[SpendID, ResourceID, VoteRank]) (removed bool) {
	if removed = c.Parents.Delete(parent); removed {
		parent.unregisterChild(c)
	}

	return removed
}

// UpdateParents updates the parents of the Spend.
func (c *Spend[SpendID, ResourceID, VoteRank]) UpdateParents(addedParents ds.Set[*Spend[SpendID, ResourceID, VoteRank]], removedParents ds.Set[*Spend[SpendID, ResourceID, VoteRank]]) (updated bool) {
	c.structureMutex.Lock()
	defer c.structureMutex.Unlock()

	if removedParents != nil {
		removedParents.Range(func(removedParent *Spend[SpendID, ResourceID, VoteRank]) {
			updated = c.removeParent(removedParent) || updated
		})
	}

	if addedParents != nil {
		addedParents.Range(func(addedParent *Spend[SpendID, ResourceID, VoteRank]) {
			if c.Parents.Add(addedParent) {
				addedParent.registerChild(c)

				updated = true
			}
		})
	}

	return updated
}

func (c *Spend[SpendID, ResourceID, VoteRank]) ApplyVote(vote *vote.Vote[VoteRank]) {
	// abort if the spend has already been accepted or rejected
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

// IsPending returns true if the Spend is pending.
func (c *Spend[SpendID, ResourceID, VoteRank]) IsPending() bool {
	return c.Weight.Value().AcceptanceState().IsPending()
}

// IsAccepted returns true if the Spend is accepted.
func (c *Spend[SpendID, ResourceID, VoteRank]) IsAccepted() bool {
	return c.Weight.Value().AcceptanceState().IsAccepted()
}

// IsRejected returns true if the Spend is rejected.
func (c *Spend[SpendID, ResourceID, VoteRank]) IsRejected() bool {
	return c.Weight.Value().AcceptanceState().IsRejected()
}

// IsPreferred returns true if the Spend is preferred instead of its conflicting Spend.
func (c *Spend[SpendID, ResourceID, VoteRank]) IsPreferred() bool {
	c.preferredInsteadMutex.RLock()
	defer c.preferredInsteadMutex.RUnlock()

	return c.preferredInstead == c
}

// PreferredInstead returns the preferred instead value of the Spend.
func (c *Spend[SpendID, ResourceID, VoteRank]) PreferredInstead() *Spend[SpendID, ResourceID, VoteRank] {
	c.preferredInsteadMutex.RLock()
	defer c.preferredInsteadMutex.RUnlock()

	return c.preferredInstead
}

// IsLiked returns true if the Spend is liked instead of other conflicting Spends.
func (c *Spend[SpendID, ResourceID, VoteRank]) IsLiked() bool {
	c.likedInsteadMutex.RLock()
	defer c.likedInsteadMutex.RUnlock()

	return c.IsPreferred() && c.likedInstead.IsEmpty()
}

// LikedInstead returns the set of liked instead Spends.
func (c *Spend[SpendID, ResourceID, VoteRank]) LikedInstead() ds.Set[*Spend[SpendID, ResourceID, VoteRank]] {
	c.likedInsteadMutex.RLock()
	defer c.likedInsteadMutex.RUnlock()

	return c.likedInstead.Clone()
}

// Shutdown shuts down the Spend.
func (c *Spend[SpendID, ResourceID, VoteRank]) Shutdown() {
	c.ConflictingSpends.Shutdown()
}

// Evict cleans up the sortedConflict.
func (c *Spend[SpendID, ResourceID, VoteRank]) Evict() (evictedConflicts []SpendID) {
	if firstEvictCall := !c.evicted.Swap(true); !firstEvictCall {
		return nil
	}

	c.unhookAcceptanceMonitoring()

	switch c.Weight.AcceptanceState() {
	case acceptance.Rejected:
		// evict the entire future cone of rejected spends
		c.Children.Range(func(childConflict *Spend[SpendID, ResourceID, VoteRank]) {
			evictedConflicts = append(evictedConflicts, childConflict.Evict()...)
		})
	default:
		// remove evicted spend from parents of children (merge to master)
		c.Children.Range(func(childConflict *Spend[SpendID, ResourceID, VoteRank]) {
			childConflict.structureMutex.Lock()
			defer childConflict.structureMutex.Unlock()

			childConflict.removeParent(c)
		})
	}

	c.structureMutex.Lock()
	defer c.structureMutex.Unlock()

	c.Parents.Range(func(parentConflict *Spend[SpendID, ResourceID, VoteRank]) {
		parentConflict.unregisterChild(c)
	})
	c.Parents.Clear()

	c.ConflictSets.Range(func(conflictSet *ConflictSet[SpendID, ResourceID, VoteRank]) {
		conflictSet.Remove(c)
	})
	c.ConflictSets.Clear()

	for _, conflict := range c.ConflictingSpends.Shutdown() {
		if conflict != c {
			conflict.ConflictingSpends.Remove(c.ID)
			c.ConflictingSpends.Remove(conflict.ID)

			if c.IsAccepted() {
				evictedConflicts = append(evictedConflicts, conflict.Evict()...)
			}
		}
	}

	c.ConflictingSpends.Remove(c.ID)

	c.preferredInsteadMutex.Lock()
	defer c.preferredInsteadMutex.Unlock()
	c.likedInsteadMutex.Lock()
	defer c.likedInsteadMutex.Unlock()

	c.likedInsteadSources.Clear()
	c.preferredInstead = nil

	evictedConflicts = append(evictedConflicts, c.ID)

	return evictedConflicts
}

// Compare compares the Spend to the given other Spend.
func (c *Spend[SpendID, ResourceID, VoteRank]) Compare(other *Spend[SpendID, ResourceID, VoteRank]) int {
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

// String returns a human-readable representation of the Spend.
func (c *Spend[SpendID, ResourceID, VoteRank]) String() string {
	// no need to lock a mutex here, because the Weight is already thread-safe

	return stringify.Struct("Spend",
		stringify.NewStructField("id", c.ID),
		stringify.NewStructField("weight", c.Weight),
	)
}

// registerChild registers the given child Spend.
func (c *Spend[SpendID, ResourceID, VoteRank]) registerChild(child *Spend[SpendID, ResourceID, VoteRank]) {
	c.structureMutex.Lock()
	defer c.structureMutex.Unlock()

	if c.Children.Add(child) {
		// hold likedInsteadMutex while determining our liked instead state
		c.likedInsteadMutex.Lock()
		defer c.likedInsteadMutex.Unlock()

		c.childUnhookMethods.Set(child.ID, lo.Batch(
			c.AcceptanceStateUpdated.Hook(func(_ acceptance.State, newState acceptance.State) {
				if newState.IsRejected() {
					child.setAcceptanceState(newState)
				}
			}).Unhook,

			c.LikedInsteadRemoved.Hook(func(reference *Spend[SpendID, ResourceID, VoteRank]) {
				child.removeInheritedLikedInsteadReference(c, reference)
			}).Unhook,

			c.LikedInsteadAdded.Hook(func(conflict *Spend[SpendID, ResourceID, VoteRank]) {
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

// unregisterChild unregisters the given child Spend.
func (c *Spend[SpendID, ResourceID, VoteRank]) unregisterChild(spend *Spend[SpendID, ResourceID, VoteRank]) {
	c.structureMutex.Lock()
	defer c.structureMutex.Unlock()

	if c.Children.Delete(spend) {
		if unhookFunc, exists := c.childUnhookMethods.Get(spend.ID); exists {
			c.childUnhookMethods.Delete(spend.ID)

			unhookFunc()
		}
	}
}

// addInheritedLikedInsteadReference adds the given reference as a liked instead reference from the given source.
func (c *Spend[SpendID, ResourceID, VoteRank]) addInheritedLikedInsteadReference(source *Spend[SpendID, ResourceID, VoteRank], reference *Spend[SpendID, ResourceID, VoteRank]) {
	c.likedInsteadMutex.Lock()
	defer c.likedInsteadMutex.Unlock()

	// abort if the source already added the reference or if the source already existed
	if sources := lo.Return1(c.likedInsteadSources.GetOrCreate(reference.ID, lo.NoVariadic(ds.NewSet[*Spend[SpendID, ResourceID, VoteRank]]))); !sources.Add(source) || !c.likedInstead.Add(reference) {
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
func (c *Spend[SpendID, ResourceID, VoteRank]) removeInheritedLikedInsteadReference(source *Spend[SpendID, ResourceID, VoteRank], reference *Spend[SpendID, ResourceID, VoteRank]) {
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

// setPreferredInstead sets the preferred instead value of the Spend.
func (c *Spend[SpendID, ResourceID, VoteRank]) setPreferredInstead(preferredInstead *Spend[SpendID, ResourceID, VoteRank]) (previousPreferredInstead *Spend[SpendID, ResourceID, VoteRank]) {
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

// setAcceptanceState sets the acceptance state of the Spend and returns the previous acceptance state (it triggers
// an AcceptanceStateUpdated event if the acceptance state was updated).
func (c *Spend[SpendID, ResourceID, VoteRank]) setAcceptanceState(newState acceptance.State) (previousState acceptance.State) {
	if previousState = c.Weight.SetAcceptanceState(newState); previousState == newState {
		return previousState
	}

	// propagate acceptance to parents first
	if newState.IsAccepted() {
		c.Parents.Range(func(parent *Spend[SpendID, ResourceID, VoteRank]) {
			parent.setAcceptanceState(acceptance.Accepted)
		})
	}

	c.AcceptanceStateUpdated.Trigger(previousState, newState)

	return previousState
}
