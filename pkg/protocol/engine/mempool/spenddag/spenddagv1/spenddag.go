package spenddagv1

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ds/walker"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/acceptance"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/core/weight"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
	iotago "github.com/iotaledger/iota.go/v4"
)

// SpendDAG represents a data structure that tracks causal relationships between Spends and that allows to
// efficiently manage these Spends (and vote on their fate).
type SpendDAG[SpenderID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]] struct {
	// events contains the events of the spenddag.
	events *spenddag.Events[SpenderID, ResourceID]

	// seatCount is a function that returns the number of seats.
	seatCount func() int

	// spendersByID is a mapping of SpenderIDs to Spenders.
	spendersByID *shrinkingmap.ShrinkingMap[SpenderID, *Spender[SpenderID, ResourceID, VoteRank]]

	spendUnhooks *shrinkingmap.ShrinkingMap[SpenderID, func()]

	// spendSetsByID is a mapping of ResourceIDs to SpendSets.
	spendSetsByID *shrinkingmap.ShrinkingMap[ResourceID, *SpendSet[SpenderID, ResourceID, VoteRank]]

	// pendingTasks is a counter that keeps track of the number of pending tasks.
	pendingTasks *syncutils.Counter

	// mutex is used to synchronize access to the spenddag.
	mutex syncutils.RWMutex

	// votingMutex is used to synchronize voting for different identities.
	votingMutex *syncutils.DAGMutex[account.SeatIndex]
}

// New creates a new spenddag.
func New[SpenderID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]](seatCount func() int) *SpendDAG[SpenderID, ResourceID, VoteRank] {
	return &SpendDAG[SpenderID, ResourceID, VoteRank]{
		events: spenddag.NewEvents[SpenderID, ResourceID](),

		seatCount:     seatCount,
		spendersByID:  shrinkingmap.New[SpenderID, *Spender[SpenderID, ResourceID, VoteRank]](),
		spendUnhooks:  shrinkingmap.New[SpenderID, func()](),
		spendSetsByID: shrinkingmap.New[ResourceID, *SpendSet[SpenderID, ResourceID, VoteRank]](),
		pendingTasks:  syncutils.NewCounter(),
		votingMutex:   syncutils.NewDAGMutex[account.SeatIndex](),
	}
}

var _ spenddag.SpendDAG[iotago.TransactionID, iotago.OutputID, vote.MockedRank] = &SpendDAG[iotago.TransactionID, iotago.OutputID, vote.MockedRank]{}

// Shutdown shuts down the SpendDAG.
func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) Shutdown() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.spendersByID.ForEach(func(spenderID SpenderID, spender *Spender[SpenderID, ResourceID, VoteRank]) bool {
		spender.Shutdown()

		return true
	})
}

// Events returns the events of the spenddag.
func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) Events() *spenddag.Events[SpenderID, ResourceID] {
	return c.events
}

// CreateSpender creates a new Spender.
func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) CreateSpender(id SpenderID) {
	if func() (created bool) {
		c.mutex.RLock()
		defer c.mutex.RUnlock()

		_, isNewSpend := c.spendersByID.GetOrCreate(id, func() *Spender[SpenderID, ResourceID, VoteRank] {
			newSpender := NewSpender[SpenderID, ResourceID, VoteRank](id, weight.New(), c.pendingTasks, acceptance.ThresholdProvider(func() int64 { return int64(c.seatCount()) }))

			// attach to the acceptance state updated event and propagate that event to the outside.
			// also need to remember the unhook method to properly evict the spender.
			c.spendUnhooks.Set(id, newSpender.AcceptanceStateUpdated.Hook(func(_ acceptance.State, newState acceptance.State) {
				if newState.IsAccepted() {
					c.events.SpenderAccepted.Trigger(newSpender.ID)
					return
				}
				if newState.IsRejected() {
					c.events.SpenderRejected.Trigger(newSpender.ID)
				}
			}).Unhook)

			return newSpender
		})

		return isNewSpend
	}() {
		c.events.SpenderCreated.Trigger(id)
	}
}

func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) UpdateSpentResources(id SpenderID, resourceIDs ds.Set[ResourceID]) error {
	joinedSpendSets, err := func() (ds.Set[ResourceID], error) {
		c.mutex.RLock()
		defer c.mutex.RUnlock()

		spender, exists := c.spendersByID.Get(id)
		if !exists {
			return nil, ierrors.Errorf("spender already evicted: %w", spenddag.ErrEntityEvicted)
		}

		return spender.JoinSpendSets(c.spendSets(resourceIDs))
	}()

	if err != nil {
		return ierrors.Errorf("spender %s failed to join spend sets: %w", id, err)
	}

	if !joinedSpendSets.IsEmpty() {
		c.events.SpentResourcesAdded.Trigger(id, joinedSpendSets)
	}

	return nil
}

// ReadConsistent write locks the spenddag and exposes read-only methods to the callback to perform multiple reads while maintaining the same spenddag state.
func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) ReadConsistent(callback func(spenddag spenddag.ReadLockedSpendDAG[SpenderID, ResourceID, VoteRank]) error) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.pendingTasks.WaitIsZero()

	return callback(c)
}

// UpdateSpendParents updates the parents of the given Spend and returns an error if the operation failed.
func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) UpdateSpendParents(spenderID SpenderID, addedParentIDs ds.Set[SpenderID], removedParentIDs ds.Set[SpenderID]) error {
	newParents := ds.NewSet[SpenderID]()

	updated, err := func() (bool, error) {
		c.mutex.RLock()
		defer c.mutex.RUnlock()

		currentSpender, currentSpendExists := c.spendersByID.Get(spenderID)
		if !currentSpendExists {
			return false, ierrors.Errorf("tried to modify evicted spend with %s: %w", spenderID, spenddag.ErrEntityEvicted)
		}

		addedParents := ds.NewSet[*Spender[SpenderID, ResourceID, VoteRank]]()

		if err := addedParentIDs.ForEach(func(addedParentID SpenderID) error {
			// If we cannot load the parent it is because it has been already evicted
			if addedParent, addedParentExists := c.spendersByID.Get(addedParentID); addedParentExists {
				addedParents.Add(addedParent)
			}

			return nil
		}); err != nil {
			return false, err
		}

		removedParents, err := c.spenders(removedParentIDs, !currentSpender.IsRejected())
		if err != nil {
			return false, ierrors.Errorf("failed to update spend parents: %w", err)
		}

		updated := currentSpender.UpdateParents(addedParents, removedParents)
		if updated {
			_ = currentSpender.Parents.ForEach(func(parentSpender *Spender[SpenderID, ResourceID, VoteRank]) (err error) {
				newParents.Add(parentSpender.ID)
				return nil
			})
		}

		return updated, nil
	}()
	if err != nil {
		return err
	}

	if updated {
		c.events.SpenderParentsUpdated.Trigger(spenderID, newParents)
	}

	return nil
}

// LikedInstead returns the SpenderIDs of the Spenders that are liked instead of the Spenders.
func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) LikedInstead(spenderIDs ds.Set[SpenderID]) ds.Set[SpenderID] {
	likedInstead := ds.NewSet[SpenderID]()
	spenderIDs.Range(func(spenderID SpenderID) {
		if currentSpender, exists := c.spendersByID.Get(spenderID); exists {
			if likedSpender := heaviestSpender(currentSpender.LikedInstead()); likedSpender != nil {
				likedInstead.Add(likedSpender.ID)
			}
		}
	})

	return likedInstead
}

func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) FutureCone(spenderIDs ds.Set[SpenderID]) (futureCone ds.Set[SpenderID]) {
	futureCone = ds.NewSet[SpenderID]()
	for futureConeWalker := walker.New[*Spender[SpenderID, ResourceID, VoteRank]]().PushAll(lo.Return1(c.spenders(spenderIDs, true)).ToSlice()...); futureConeWalker.HasNext(); {
		if spender := futureConeWalker.Next(); futureCone.Add(spender.ID) {
			futureConeWalker.PushAll(spender.Children.ToSlice()...)
		}
	}

	return futureCone
}

func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) ConflictingSpenders(spenderID SpenderID) (conflictingSpenders ds.Set[SpenderID], exists bool) {
	spender, exists := c.spendersByID.Get(spenderID)
	if !exists {
		return nil, false
	}

	conflictingSpenders = ds.NewSet[SpenderID]()
	spender.ConflictingSpenders.Range(func(conflictingSpender *Spender[SpenderID, ResourceID, VoteRank]) {
		conflictingSpenders.Add(conflictingSpender.ID)
	})

	return conflictingSpenders, true
}

func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) AllSpendsSupported(seat account.SeatIndex, spenderIDs ds.Set[SpenderID]) bool {
	return lo.Return1(c.spenders(spenderIDs, true)).ForEach(func(spender *Spender[SpenderID, ResourceID, VoteRank]) (err error) {
		lastVote, exists := spender.LatestVotes.Get(seat)

		return lo.Cond(exists && lastVote.IsLiked(), nil, ierrors.Errorf("spender %s is not supported by seat %d", spender.ID, seat))
	}) == nil
}

func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) SpendVoters(spenderID SpenderID) (spendVoters ds.Set[account.SeatIndex]) {
	if spender, exists := c.spendersByID.Get(spenderID); exists {
		return spender.Weight.Voters.Clone()
	}

	return ds.NewSet[account.SeatIndex]()
}

func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) SpendSets(spenderID SpenderID) (spendSets ds.Set[ResourceID], exists bool) {
	spender, exists := c.spendersByID.Get(spenderID)
	if !exists {
		return nil, false
	}

	spendSets = ds.NewSet[ResourceID]()
	_ = spender.SpendSets.ForEach(func(spendSet *SpendSet[SpenderID, ResourceID, VoteRank]) error {
		spendSets.Add(spendSet.ID)
		return nil
	})

	return spendSets, true
}

func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) SpendParents(spenderID SpenderID) (spendParents ds.Set[SpenderID], exists bool) {
	spender, exists := c.spendersByID.Get(spenderID)
	if !exists {
		return nil, false
	}

	spendParents = ds.NewSet[SpenderID]()
	_ = spender.Parents.ForEach(func(parent *Spender[SpenderID, ResourceID, VoteRank]) error {
		spendParents.Add(parent.ID)
		return nil
	})

	return spendParents, true
}

func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) SpendChildren(spenderID SpenderID) (spendChildren ds.Set[SpenderID], exists bool) {
	spender, exists := c.spendersByID.Get(spenderID)
	if !exists {
		return nil, false
	}

	spendChildren = ds.NewSet[SpenderID]()
	_ = spender.Children.ForEach(func(parent *Spender[SpenderID, ResourceID, VoteRank]) error {
		spendChildren.Add(parent.ID)
		return nil
	})

	return spendChildren, true
}

func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) SpendSetMembers(spendSetID ResourceID) (spenders ds.Set[SpenderID], exists bool) {
	spendSet, exists := c.spendSetsByID.Get(spendSetID)
	if !exists {
		return nil, false
	}

	spenders = ds.NewSet[SpenderID]()
	_ = spendSet.ForEach(func(parent *Spender[SpenderID, ResourceID, VoteRank]) error {
		spenders.Add(parent.ID)
		return nil
	})

	return spenders, true
}

func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) SpendWeight(spenderID SpenderID) int64 {
	if spender, exists := c.spendersByID.Get(spenderID); exists {
		return spender.Weight.Value().ValidatorsWeight()
	}

	return 0
}

// CastVotes applies the given votes to the spenddag.
func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) CastVotes(vote *vote.Vote[VoteRank], spenderIDs ds.Set[SpenderID]) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	c.votingMutex.Lock(vote.Voter)
	defer c.votingMutex.Unlock(vote.Voter)

	supportedSpenders, revokedSpenders, err := c.determineVotes(spenderIDs)
	if err != nil {
		return ierrors.Errorf("failed to determine votes: %w", err)
	}

	for supportedSpender := supportedSpenders.Iterator(); supportedSpender.HasNext(); {
		supportedSpender.Next().ApplyVote(vote.WithLiked(true))
	}

	for revokedSpender := revokedSpenders.Iterator(); revokedSpender.HasNext(); {
		revokedSpender.Next().ApplyVote(vote.WithLiked(false))
	}

	return nil
}

func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) AcceptanceState(spenderIDs ds.Set[SpenderID]) acceptance.State {
	lowestObservedState := acceptance.Accepted
	if err := spenderIDs.ForEach(func(spenderID SpenderID) error {
		spender, exists := c.spendersByID.Get(spenderID)
		if !exists {
			return ierrors.Errorf("tried to retrieve non-existing spend: %w", spenddag.ErrFatal)
		}

		if spender.IsRejected() {
			lowestObservedState = acceptance.Rejected

			return spenddag.ErrExpected
		}

		if spender.IsPending() {
			lowestObservedState = acceptance.Pending
		}

		return nil
	}); err != nil && !ierrors.Is(err, spenddag.ErrExpected) {
		panic(err)
	}

	return lowestObservedState
}

func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) SetAccepted(spenderID SpenderID) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if spender, exists := c.spendersByID.Get(spenderID); exists {
		spender.setAcceptanceState(acceptance.Accepted)
	}
}

// UnacceptedSpends takes a set of SpenderIDs and removes all the accepted Spends (leaving only the
// pending or rejected ones behind).
func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) UnacceptedSpenders(spenderIDs ds.Set[SpenderID]) ds.Set[SpenderID] {
	pendingSpenderIDs := ds.NewSet[SpenderID]()
	spenderIDs.Range(func(currentSpenderID SpenderID) {
		if spender, exists := c.spendersByID.Get(currentSpenderID); exists && !spender.IsAccepted() {
			pendingSpenderIDs.Add(currentSpenderID)
		}
	})

	return pendingSpenderIDs
}

// EvictSpend removes spend with given SpenderID from spenddag.
func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) EvictSpender(spenderID SpenderID) {
	for _, evictedSpenderID := range func() []SpenderID {
		c.mutex.RLock()
		defer c.mutex.RUnlock()

		return c.evictSpender(spenderID)
	}() {
		c.events.SpenderEvicted.Trigger(evictedSpenderID)
	}
}

func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) evictSpender(spenderID SpenderID) []SpenderID {
	// evicting an already evicted spend is fine
	spender, exists := c.spendersByID.Get(spenderID)
	if !exists {
		return nil
	}

	evictedSpenderIDs := spender.Evict()

	// remove the spenders from the spenddag dictionary
	for _, evictedSpenderID := range evictedSpenderIDs {
		c.spendersByID.Delete(evictedSpenderID)
	}

	// unhook the spend events and remove the unhook method from the storage
	unhookFunc, unhookExists := c.spendUnhooks.Get(spenderID)
	if unhookExists {
		unhookFunc()
		c.spendUnhooks.Delete(spenderID)
	}

	return evictedSpenderIDs
}

// spenders returns the Spenders that are associated with the given SpenderIDs. If ignoreMissing is set to true, it
// will ignore missing Spenders instead of returning an ErrEntityEvicted error.
func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) spenders(ids ds.Set[SpenderID], ignoreMissing bool) (ds.Set[*Spender[SpenderID, ResourceID, VoteRank]], error) {
	spenders := ds.NewSet[*Spender[SpenderID, ResourceID, VoteRank]]()

	return spenders, ids.ForEach(func(id SpenderID) (err error) {
		existingSpend, exists := c.spendersByID.Get(id)
		if exists {
			spenders.Add(existingSpend)
		}

		return lo.Cond(exists || ignoreMissing, nil, ierrors.Errorf("tried to retrieve a non-existing spend with %s: %w", id, spenddag.ErrEntityEvicted))
	})
}

// spendSets returns the SpendSets that are associated with the given ResourceIDs. If createMissing is set to
// true, it will create an empty SpendSets for each missing ResourceID.
func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) spendSets(resourceIDs ds.Set[ResourceID]) ds.Set[*SpendSet[SpenderID, ResourceID, VoteRank]] {
	spendSets := ds.NewSet[*SpendSet[SpenderID, ResourceID, VoteRank]]()

	resourceIDs.Range(func(resourceID ResourceID) {
		spendSets.Add(lo.Return1(c.spendSetsByID.GetOrCreate(resourceID, c.spendSetFactory(resourceID))))
	})

	return spendSets
}

// determineVotes determines the Spends that are supported and revoked by the given SpenderIDs.
func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) determineVotes(spenderIDs ds.Set[SpenderID]) (supportedSpenders ds.Set[*Spender[SpenderID, ResourceID, VoteRank]], revokedSpenders ds.Set[*Spender[SpenderID, ResourceID, VoteRank]], err error) {
	supportedSpenders = ds.NewSet[*Spender[SpenderID, ResourceID, VoteRank]]()
	revokedSpenders = ds.NewSet[*Spender[SpenderID, ResourceID, VoteRank]]()

	revokedWalker := walker.New[*Spender[SpenderID, ResourceID, VoteRank]]()
	revokeSpend := func(revokedSpender *Spender[SpenderID, ResourceID, VoteRank]) error {
		if revokedSpenders.Add(revokedSpender) {
			if supportedSpenders.Has(revokedSpender) {
				return ierrors.Errorf("applied conflicting votes (%s is supported and revoked)", revokedSpender.ID)
			}

			revokedWalker.PushAll(revokedSpender.Children.ToSlice()...)
		}

		return nil
	}

	supportedWalker := walker.New[*Spender[SpenderID, ResourceID, VoteRank]]()
	supportSpender := func(supportedSpender *Spender[SpenderID, ResourceID, VoteRank]) error {
		if supportedSpenders.Add(supportedSpender) {
			if err := supportedSpender.ConflictingSpenders.ForEach(revokeSpend); err != nil {
				return ierrors.Errorf("failed to collect conflicting spenders: %w", err)
			}

			supportedWalker.PushAll(supportedSpender.Parents.ToSlice()...)
		}

		return nil
	}

	for supportedWalker.PushAll(lo.Return1(c.spenders(spenderIDs, true)).ToSlice()...); supportedWalker.HasNext(); {
		if err := supportSpender(supportedWalker.Next()); err != nil {
			return nil, nil, ierrors.Errorf("failed to collect supported spenders: %w", err)
		}
	}

	for revokedWalker.HasNext() {
		if revokedSpender := revokedWalker.Next(); revokedSpenders.Add(revokedSpender) {
			revokedWalker.PushAll(revokedSpender.Children.ToSlice()...)
		}
	}

	return supportedSpenders, revokedSpenders, nil
}

func (c *SpendDAG[SpenderID, ResourceID, VoteRank]) spendSetFactory(resourceID ResourceID) func() *SpendSet[SpenderID, ResourceID, VoteRank] {
	return func() *SpendSet[SpenderID, ResourceID, VoteRank] {
		spendSet := NewSpendSet[SpenderID, ResourceID, VoteRank](resourceID)

		spendSet.OnAllMembersEvicted(func(prevValue bool, newValue bool) {
			if newValue && !prevValue {
				c.spendSetsByID.Delete(spendSet.ID)
			}
		})

		return spendSet
	}
}
