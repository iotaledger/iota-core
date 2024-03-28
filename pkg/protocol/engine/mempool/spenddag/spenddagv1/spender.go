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

// Spender is a spender of resources that is part of a Spend DAG.
// An example of a spender is a transaction, and the resource it spends is a utxo.
type Spender[SpenderID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]] struct {
	// ID is the identifier of the Spender.
	ID SpenderID

	// Parents is the set of parents of the Spender.
	Parents ds.Set[*Spender[SpenderID, ResourceID, VoteRank]]

	// Children is the set of children of the Spender.
	Children ds.Set[*Spender[SpenderID, ResourceID, VoteRank]]

	// SpendSets is the set of SpendSets that the Spend is part of.
	SpendSets ds.Set[*SpendSet[SpenderID, ResourceID, VoteRank]]

	// ConflictingSpenders is the set of spenders that try to spend one of more of the same resources as this Spender.
	ConflictingSpenders *SortedSpenders[SpenderID, ResourceID, VoteRank]

	// Weight is the Weight of the Spender.
	Weight *weight.Weight

	// LatestVotes is the set of the latest votes of the Spender.
	LatestVotes *shrinkingmap.ShrinkingMap[account.SeatIndex, *vote.Vote[VoteRank]]

	// AcceptanceStateUpdated is triggered when the AcceptanceState of the Spender is updated.
	AcceptanceStateUpdated *event.Event2[acceptance.State, acceptance.State]

	// PreferredInsteadUpdated is triggered when the preferred instead value of the Spender is updated.
	PreferredInsteadUpdated *event.Event1[*Spender[SpenderID, ResourceID, VoteRank]]

	// LikedInsteadAdded is triggered when a liked instead reference is added to the Spender.
	LikedInsteadAdded *event.Event1[*Spender[SpenderID, ResourceID, VoteRank]]

	// LikedInsteadRemoved is triggered when a liked instead reference is removed from the Spender.
	LikedInsteadRemoved *event.Event1[*Spender[SpenderID, ResourceID, VoteRank]]

	// childUnhookMethods is a mapping of children to their unhook functions.
	childUnhookMethods *shrinkingmap.ShrinkingMap[SpenderID, func()]

	// preferredInstead is the preferred instead value of the Spender.
	preferredInstead *Spender[SpenderID, ResourceID, VoteRank]

	// evicted
	evicted atomic.Bool

	// preferredInsteadMutex is used to synchronize access to the preferred instead value of the Spender.
	preferredInsteadMutex syncutils.RWMutex

	// likedInstead is the set of liked instead Spenders.
	likedInstead ds.Set[*Spender[SpenderID, ResourceID, VoteRank]]

	// likedInsteadSources is a mapping of liked instead Spends to the set of parents that inherited them.
	likedInsteadSources *shrinkingmap.ShrinkingMap[SpenderID, ds.Set[*Spender[SpenderID, ResourceID, VoteRank]]]

	// likedInsteadMutex and structureMutex are sometimes locked in different order by different goroutines, which could result in a deadlock
	//  however, it's impossible to deadlock if we fork all transactions upon booking
	//  deadlock happens when the likedInstead spend changes and parents are updated at the same time, which is impossible in the current setup
	//  because we won't process votes on a spend we're just creating.
	// likedInsteadMutex is used to synchronize access to the liked instead value of the Spender.
	likedInsteadMutex syncutils.RWMutex

	// structureMutex is used to synchronize access to the structure of the Spender.
	structureMutex syncutils.RWMutex

	// acceptanceThreshold is the function that is used to retrieve the acceptance threshold of the committee.
	acceptanceThreshold func() int64

	// unhookAcceptanceMonitoring
	unhookAcceptanceMonitoring func()
}

// NewSpend creates a new Spender.
func NewSpender[SpenderID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]](id SpenderID, initialWeight *weight.Weight, pendingTasksCounter *syncutils.Counter, acceptanceThresholdProvider func() int64) *Spender[SpenderID, ResourceID, VoteRank] {
	c := &Spender[SpenderID, ResourceID, VoteRank]{
		ID:                      id,
		Parents:                 ds.NewSet[*Spender[SpenderID, ResourceID, VoteRank]](),
		Children:                ds.NewSet[*Spender[SpenderID, ResourceID, VoteRank]](),
		SpendSets:               ds.NewSet[*SpendSet[SpenderID, ResourceID, VoteRank]](),
		Weight:                  initialWeight,
		LatestVotes:             shrinkingmap.New[account.SeatIndex, *vote.Vote[VoteRank]](),
		AcceptanceStateUpdated:  event.New2[acceptance.State, acceptance.State](),
		PreferredInsteadUpdated: event.New1[*Spender[SpenderID, ResourceID, VoteRank]](),
		LikedInsteadAdded:       event.New1[*Spender[SpenderID, ResourceID, VoteRank]](),
		LikedInsteadRemoved:     event.New1[*Spender[SpenderID, ResourceID, VoteRank]](),

		childUnhookMethods:  shrinkingmap.New[SpenderID, func()](),
		acceptanceThreshold: acceptanceThresholdProvider,
		likedInstead:        ds.NewSet[*Spender[SpenderID, ResourceID, VoteRank]](),
		likedInsteadSources: shrinkingmap.New[SpenderID, ds.Set[*Spender[SpenderID, ResourceID, VoteRank]]](),
	}

	c.preferredInstead = c

	c.unhookAcceptanceMonitoring = c.Weight.OnUpdate.Hook(func(value weight.Value) {
		if threshold := c.acceptanceThreshold(); value.AcceptanceState().IsPending() && value.ValidatorsWeight() >= threshold && value.AttestorsWeight() >= threshold {
			c.setAcceptanceState(acceptance.Accepted)
		}
	}).Unhook

	// in case the initial weight is enough to accept the spend, accept it immediately
	if threshold := c.acceptanceThreshold(); initialWeight.AcceptanceState().IsPending() && initialWeight.Value().ValidatorsWeight() >= threshold && initialWeight.Value().AttestorsWeight() >= threshold {
		c.setAcceptanceState(acceptance.Accepted)
	}

	c.ConflictingSpenders = NewSortedSpenders(c, pendingTasksCounter)

	return c
}

// JoinSpendSets registers the Spender with the given SpendSets.
func (c *Spender[SpenderID, ResourceID, VoteRank]) JoinSpendSets(spendSets ds.Set[*SpendSet[SpenderID, ResourceID, VoteRank]]) (joinedSpendSets ds.Set[ResourceID], err error) {
	if spendSets == nil {
		return ds.NewSet[ResourceID](), nil
	}

	if c.evicted.Load() {
		return nil, ierrors.WithMessage(spenddag.ErrEntityEvicted, "tried to join spend sets of evicted spender")
	}

	registerConflictingSpender := func(c *Spender[SpenderID, ResourceID, VoteRank], spender *Spender[SpenderID, ResourceID, VoteRank]) {
		c.structureMutex.Lock()
		defer c.structureMutex.Unlock()

		if c.ConflictingSpenders.Add(spender) {
			if spender.IsAccepted() {
				c.setAcceptanceState(acceptance.Rejected)
			}
		}
	}

	joinedSpendSets = ds.NewSet[ResourceID]()

	return joinedSpendSets, spendSets.ForEach(func(spendSet *SpendSet[SpenderID, ResourceID, VoteRank]) error {
		otherConflicts, err := spendSet.Add(c)
		if err != nil && !ierrors.Is(err, spenddag.ErrAlreadyPartOfSpendSet) {
			return err
		}

		if c.SpendSets.Add(spendSet) {
			if otherConflicts != nil {
				otherConflicts.Range(func(otherConflict *Spender[SpenderID, ResourceID, VoteRank]) {
					registerConflictingSpender(c, otherConflict)
					registerConflictingSpender(otherConflict, c)
				})

				joinedSpendSets.Add(spendSet.ID)
			}
		}

		return nil
	})
}

func (c *Spender[SpenderID, ResourceID, VoteRank]) removeParent(parent *Spender[SpenderID, ResourceID, VoteRank]) (removed bool) {
	if removed = c.Parents.Delete(parent); removed {
		parent.unregisterChild(c)
	}

	return removed
}

// UpdateParents updates the parents of the Spender.
func (c *Spender[SpenderID, ResourceID, VoteRank]) UpdateParents(addedParents ds.Set[*Spender[SpenderID, ResourceID, VoteRank]], removedParents ds.Set[*Spender[SpenderID, ResourceID, VoteRank]]) (updated bool) {
	c.structureMutex.Lock()
	defer c.structureMutex.Unlock()

	if removedParents != nil {
		removedParents.Range(func(removedParent *Spender[SpenderID, ResourceID, VoteRank]) {
			updated = c.removeParent(removedParent) || updated
		})
	}

	if addedParents != nil {
		addedParents.Range(func(addedParent *Spender[SpenderID, ResourceID, VoteRank]) {
			if c.Parents.Add(addedParent) {
				addedParent.registerChild(c)

				updated = true
			}
		})
	}

	return updated
}

func (c *Spender[SpenderID, ResourceID, VoteRank]) ApplyVote(vote *vote.Vote[VoteRank]) {
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

	// track attestors when we reach the acceptance threshold
	if c.Weight.Value().ValidatorsWeight() >= c.acceptanceThreshold() {
		if vote.IsLiked() {
			c.Weight.AddAttestor(vote.Voter)
		} else {
			c.Weight.DeleteAttestor(vote.Voter)
		}
	} else {
		c.Weight.ResetAttestors()
	}

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
func (c *Spender[SpenderID, ResourceID, VoteRank]) IsPending() bool {
	return c.Weight.Value().AcceptanceState().IsPending()
}

// IsAccepted returns true if the Spend is accepted.
func (c *Spender[SpenderID, ResourceID, VoteRank]) IsAccepted() bool {
	return c.Weight.Value().AcceptanceState().IsAccepted()
}

// IsRejected returns true if the Spend is rejected.
func (c *Spender[SpenderID, ResourceID, VoteRank]) IsRejected() bool {
	return c.Weight.Value().AcceptanceState().IsRejected()
}

// IsPreferred returns true if the Spend is preferred instead of its conflicting Spender.
func (c *Spender[SpenderID, ResourceID, VoteRank]) IsPreferred() bool {
	c.preferredInsteadMutex.RLock()
	defer c.preferredInsteadMutex.RUnlock()

	return c.preferredInstead == c
}

// PreferredInstead returns the preferred instead value of the Spender.
func (c *Spender[SpenderID, ResourceID, VoteRank]) PreferredInstead() *Spender[SpenderID, ResourceID, VoteRank] {
	c.preferredInsteadMutex.RLock()
	defer c.preferredInsteadMutex.RUnlock()

	return c.preferredInstead
}

// IsLiked returns true if the Spend is liked instead of other conflicting Spenders.
func (c *Spender[SpenderID, ResourceID, VoteRank]) IsLiked() bool {
	c.likedInsteadMutex.RLock()
	defer c.likedInsteadMutex.RUnlock()

	return c.IsPreferred() && c.likedInstead.IsEmpty()
}

// LikedInstead returns the set of liked instead Spenders.
func (c *Spender[SpenderID, ResourceID, VoteRank]) LikedInstead() ds.Set[*Spender[SpenderID, ResourceID, VoteRank]] {
	c.likedInsteadMutex.RLock()
	defer c.likedInsteadMutex.RUnlock()

	return c.likedInstead.Clone()
}

// Shutdown shuts down the Spender.
func (c *Spender[SpenderID, ResourceID, VoteRank]) Shutdown() {
	c.ConflictingSpenders.Shutdown()
}

// Evict cleans up the sortedSpender.
func (c *Spender[SpenderID, ResourceID, VoteRank]) Evict() (evictedSpends []SpenderID) {
	if firstEvictCall := !c.evicted.Swap(true); !firstEvictCall {
		return nil
	}

	c.unhookAcceptanceMonitoring()

	switch c.Weight.AcceptanceState() {
	case acceptance.Rejected:
		// evict the entire future cone of rejected spenders
		c.Children.Range(func(childSpender *Spender[SpenderID, ResourceID, VoteRank]) {
			evictedSpends = append(evictedSpends, childSpender.Evict()...)
		})
	default:
		// remove evicted spender from parents of children (merge to master)
		c.Children.Range(func(childSpender *Spender[SpenderID, ResourceID, VoteRank]) {
			childSpender.structureMutex.Lock()
			defer childSpender.structureMutex.Unlock()

			childSpender.removeParent(c)
		})
	}

	c.structureMutex.Lock()
	defer c.structureMutex.Unlock()

	c.Parents.Range(func(parentSpender *Spender[SpenderID, ResourceID, VoteRank]) {
		parentSpender.unregisterChild(c)
	})
	c.Parents.Clear()

	c.SpendSets.Range(func(spendSet *SpendSet[SpenderID, ResourceID, VoteRank]) {
		spendSet.Remove(c)
	})
	c.SpendSets.Clear()

	for _, spender := range c.ConflictingSpenders.Shutdown() {
		if spender != c {
			spender.ConflictingSpenders.Remove(c.ID)
			c.ConflictingSpenders.Remove(spender.ID)

			if c.IsAccepted() {
				evictedSpends = append(evictedSpends, spender.Evict()...)
			}
		}
	}

	c.ConflictingSpenders.Remove(c.ID)

	c.preferredInsteadMutex.Lock()
	defer c.preferredInsteadMutex.Unlock()
	c.likedInsteadMutex.Lock()
	defer c.likedInsteadMutex.Unlock()

	c.likedInsteadSources.Clear()
	c.preferredInstead = nil

	evictedSpends = append(evictedSpends, c.ID)

	return evictedSpends
}

// Compare compares the Spender to the given other Spender.
func (c *Spender[SpenderID, ResourceID, VoteRank]) Compare(other *Spender[SpenderID, ResourceID, VoteRank]) int {
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

// String returns a human-readable representation of the Spender.
func (c *Spender[SpenderID, ResourceID, VoteRank]) String() string {
	// no need to lock a mutex here, because the Weight is already thread-safe

	return stringify.Struct("Spender",
		stringify.NewStructField("id", c.ID),
		stringify.NewStructField("weight", c.Weight),
	)
}

// registerChild registers the given child Spender.
func (c *Spender[SpenderID, ResourceID, VoteRank]) registerChild(child *Spender[SpenderID, ResourceID, VoteRank]) {
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

			c.LikedInsteadRemoved.Hook(func(reference *Spender[SpenderID, ResourceID, VoteRank]) {
				child.removeInheritedLikedInsteadReference(c, reference)
			}).Unhook,

			c.LikedInsteadAdded.Hook(func(spend *Spender[SpenderID, ResourceID, VoteRank]) {
				child.structureMutex.Lock()
				defer child.structureMutex.Unlock()

				child.addInheritedLikedInsteadReference(c, spend)
			}).Unhook,
		))

		for spenders := c.likedInstead.Iterator(); spenders.HasNext(); {
			child.addInheritedLikedInsteadReference(c, spenders.Next())
		}

		if c.IsRejected() {
			child.setAcceptanceState(acceptance.Rejected)
		}
	}
}

// unregisterChild unregisters the given child Spender.
func (c *Spender[SpenderID, ResourceID, VoteRank]) unregisterChild(spender *Spender[SpenderID, ResourceID, VoteRank]) {
	c.structureMutex.Lock()
	defer c.structureMutex.Unlock()

	if c.Children.Delete(spender) {
		if unhookFunc, exists := c.childUnhookMethods.Get(spender.ID); exists {
			c.childUnhookMethods.Delete(spender.ID)

			unhookFunc()
		}
	}
}

// addInheritedLikedInsteadReference adds the given reference as a liked instead reference from the given source.
func (c *Spender[SpenderID, ResourceID, VoteRank]) addInheritedLikedInsteadReference(source *Spender[SpenderID, ResourceID, VoteRank], reference *Spender[SpenderID, ResourceID, VoteRank]) {
	c.likedInsteadMutex.Lock()
	defer c.likedInsteadMutex.Unlock()

	// abort if the source already added the reference or if the source already existed
	if sources := lo.Return1(c.likedInsteadSources.GetOrCreate(reference.ID, lo.NoVariadic(ds.NewSet[*Spender[SpenderID, ResourceID, VoteRank]]))); !sources.Add(source) || !c.likedInstead.Add(reference) {
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
func (c *Spender[SpenderID, ResourceID, VoteRank]) removeInheritedLikedInsteadReference(source *Spender[SpenderID, ResourceID, VoteRank], reference *Spender[SpenderID, ResourceID, VoteRank]) {
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

// setPreferredInstead sets the preferred instead value of the Spender.
func (c *Spender[SpenderID, ResourceID, VoteRank]) setPreferredInstead(preferredInstead *Spender[SpenderID, ResourceID, VoteRank]) (previousPreferredInstead *Spender[SpenderID, ResourceID, VoteRank]) {
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
func (c *Spender[SpenderID, ResourceID, VoteRank]) setAcceptanceState(newState acceptance.State) (previousState acceptance.State) {
	if previousState = c.Weight.SetAcceptanceState(newState); previousState == newState {
		return previousState
	}

	// propagate acceptance to parents first
	if newState.IsAccepted() {
		c.Parents.Range(func(parent *Spender[SpenderID, ResourceID, VoteRank]) {
			parent.setAcceptanceState(acceptance.Accepted)
		})
	}

	c.AcceptanceStateUpdated.Trigger(previousState, newState)

	return previousState
}
