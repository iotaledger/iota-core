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
type SpendDAG[SpendID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]] struct {
	// events contains the events of the spenddag.
	events *spenddag.Events[SpendID, ResourceID]

	// seatCount is a function that returns the number of seats.
	seatCount func() int

	// spendsByID is a mapping of SpendIDs to Spends.
	spendsByID *shrinkingmap.ShrinkingMap[SpendID, *Spend[SpendID, ResourceID, VoteRank]]

	spendUnhooks *shrinkingmap.ShrinkingMap[SpendID, func()]

	// conflictSetsByID is a mapping of ResourceIDs to ConflictSets.
	conflictSetsByID *shrinkingmap.ShrinkingMap[ResourceID, *ConflictSet[SpendID, ResourceID, VoteRank]]

	// pendingTasks is a counter that keeps track of the number of pending tasks.
	pendingTasks *syncutils.Counter

	// mutex is used to synchronize access to the spenddag.
	mutex syncutils.RWMutex

	// votingMutex is used to synchronize voting for different identities.
	votingMutex *syncutils.DAGMutex[account.SeatIndex]
}

// New creates a new spenddag.
func New[SpendID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]](seatCount func() int) *SpendDAG[SpendID, ResourceID, VoteRank] {
	return &SpendDAG[SpendID, ResourceID, VoteRank]{
		events: spenddag.NewEvents[SpendID, ResourceID](),

		seatCount:        seatCount,
		spendsByID:       shrinkingmap.New[SpendID, *Spend[SpendID, ResourceID, VoteRank]](),
		spendUnhooks:     shrinkingmap.New[SpendID, func()](),
		conflictSetsByID: shrinkingmap.New[ResourceID, *ConflictSet[SpendID, ResourceID, VoteRank]](),
		pendingTasks:     syncutils.NewCounter(),
		votingMutex:      syncutils.NewDAGMutex[account.SeatIndex](),
	}
}

var _ spenddag.SpendDAG[iotago.TransactionID, iotago.OutputID, vote.MockedRank] = &SpendDAG[iotago.TransactionID, iotago.OutputID, vote.MockedRank]{}

// Shutdown shuts down the SpendDAG.
func (c *SpendDAG[SpendID, ResourceID, VoteRank]) Shutdown() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.spendsByID.ForEach(func(spendID SpendID, spend *Spend[SpendID, ResourceID, VoteRank]) bool {
		spend.Shutdown()

		return true
	})
}

// Events returns the events of the spenddag.
func (c *SpendDAG[SpendID, ResourceID, VoteRank]) Events() *spenddag.Events[SpendID, ResourceID] {
	return c.events
}

// CreateSpend creates a new Spend.
func (c *SpendDAG[SpendID, ResourceID, VoteRank]) CreateSpend(id SpendID) {
	if func() (created bool) {
		c.mutex.RLock()
		defer c.mutex.RUnlock()

		_, isNewSpend := c.spendsByID.GetOrCreate(id, func() *Spend[SpendID, ResourceID, VoteRank] {
			newSpend := NewSpend[SpendID, ResourceID, VoteRank](id, weight.New(), c.pendingTasks, acceptance.ThresholdProvider(func() int64 { return int64(c.seatCount()) }))

			// attach to the acceptance state updated event and propagate that event to the outside.
			// also need to remember the unhook method to properly evict the spend.
			c.spendUnhooks.Set(id, newSpend.AcceptanceStateUpdated.Hook(func(_ acceptance.State, newState acceptance.State) {
				if newState.IsAccepted() {
					c.events.SpendAccepted.Trigger(newSpend.ID)
					return
				}
				if newState.IsRejected() {
					c.events.SpendRejected.Trigger(newSpend.ID)
				}
			}).Unhook)

			return newSpend
		})

		return isNewSpend
	}() {
		c.events.SpendCreated.Trigger(id)
	}
}

func (c *SpendDAG[SpendID, ResourceID, VoteRank]) UpdateConflictingResources(id SpendID, resourceIDs ds.Set[ResourceID]) error {
	joinedConflictSets, err := func() (ds.Set[ResourceID], error) {
		c.mutex.RLock()
		defer c.mutex.RUnlock()

		spend, exists := c.spendsByID.Get(id)
		if !exists {
			return nil, ierrors.Errorf("spend already evicted: %w", spenddag.ErrEntityEvicted)
		}

		return spend.JoinSpendSets(c.conflictSets(resourceIDs))
	}()

	if err != nil {
		return ierrors.Errorf("spend %s failed to join spend sets: %w", id, err)
	}

	if !joinedConflictSets.IsEmpty() {
		c.events.ConflictingResourcesAdded.Trigger(id, joinedConflictSets)
	}

	return nil
}

// ReadConsistent write locks the spenddag and exposes read-only methods to the callback to perform multiple reads while maintaining the same spenddag state.
func (c *SpendDAG[SpendID, ResourceID, VoteRank]) ReadConsistent(callback func(spenddag spenddag.ReadLockedSpendDAG[SpendID, ResourceID, VoteRank]) error) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.pendingTasks.WaitIsZero()

	return callback(c)
}

// UpdateSpendParents updates the parents of the given Spend and returns an error if the operation failed.
func (c *SpendDAG[SpendID, ResourceID, VoteRank]) UpdateSpendParents(spendID SpendID, addedParentIDs ds.Set[SpendID], removedParentIDs ds.Set[SpendID]) error {
	newParents := ds.NewSet[SpendID]()

	updated, err := func() (bool, error) {
		c.mutex.RLock()
		defer c.mutex.RUnlock()

		currentSpend, currentSpendExists := c.spendsByID.Get(spendID)
		if !currentSpendExists {
			return false, ierrors.Errorf("tried to modify evicted spend with %s: %w", spendID, spenddag.ErrEntityEvicted)
		}

		addedParents := ds.NewSet[*Spend[SpendID, ResourceID, VoteRank]]()

		if err := addedParentIDs.ForEach(func(addedParentID SpendID) error {
			// If we cannot load the parent it is because it has been already evicted
			if addedParent, addedParentExists := c.spendsByID.Get(addedParentID); addedParentExists {
				addedParents.Add(addedParent)
			}

			return nil
		}); err != nil {
			return false, err
		}

		removedParents, err := c.spends(removedParentIDs, !currentSpend.IsRejected())
		if err != nil {
			return false, ierrors.Errorf("failed to update spend parents: %w", err)
		}

		updated := currentSpend.UpdateParents(addedParents, removedParents)
		if updated {
			_ = currentSpend.Parents.ForEach(func(parentConflict *Spend[SpendID, ResourceID, VoteRank]) (err error) {
				newParents.Add(parentConflict.ID)
				return nil
			})
		}

		return updated, nil
	}()
	if err != nil {
		return err
	}

	if updated {
		c.events.SpendParentsUpdated.Trigger(spendID, newParents)
	}

	return nil
}

// LikedInstead returns the SpendIDs of the Spends that are liked instead of the Spends.
func (c *SpendDAG[SpendID, ResourceID, VoteRank]) LikedInstead(spendIDs ds.Set[SpendID]) ds.Set[SpendID] {
	likedInstead := ds.NewSet[SpendID]()
	spendIDs.Range(func(spendID SpendID) {
		if currentConflict, exists := c.spendsByID.Get(spendID); exists {
			if likedConflict := heaviestConflict(currentConflict.LikedInstead()); likedConflict != nil {
				likedInstead.Add(likedConflict.ID)
			}
		}
	})

	return likedInstead
}

func (c *SpendDAG[SpendID, ResourceID, VoteRank]) FutureCone(spendIDs ds.Set[SpendID]) (futureCone ds.Set[SpendID]) {
	futureCone = ds.NewSet[SpendID]()
	for futureConeWalker := walker.New[*Spend[SpendID, ResourceID, VoteRank]]().PushAll(lo.Return1(c.spends(spendIDs, true)).ToSlice()...); futureConeWalker.HasNext(); {
		if spend := futureConeWalker.Next(); futureCone.Add(spend.ID) {
			futureConeWalker.PushAll(spend.Children.ToSlice()...)
		}
	}

	return futureCone
}

func (c *SpendDAG[SpendID, ResourceID, VoteRank]) ConflictingSpends(spendID SpendID) (conflictingSpends ds.Set[SpendID], exists bool) {
	spend, exists := c.spendsByID.Get(spendID)
	if !exists {
		return nil, false
	}

	conflictingSpends = ds.NewSet[SpendID]()
	spend.ConflictingSpends.Range(func(conflictingConflict *Spend[SpendID, ResourceID, VoteRank]) {
		conflictingSpends.Add(conflictingConflict.ID)
	})

	return conflictingSpends, true
}

func (c *SpendDAG[SpendID, ResourceID, VoteRank]) AllSpendsSupported(seat account.SeatIndex, spendIDs ds.Set[SpendID]) bool {
	return lo.Return1(c.spends(spendIDs, true)).ForEach(func(spend *Spend[SpendID, ResourceID, VoteRank]) (err error) {
		lastVote, exists := spend.LatestVotes.Get(seat)

		return lo.Cond(exists && lastVote.IsLiked(), nil, ierrors.Errorf("spend with %s is not supported by seat %d", spend.ID, seat))
	}) == nil
}

func (c *SpendDAG[SpendID, ResourceID, VoteRank]) SpendVoters(spendID SpendID) (conflictVoters ds.Set[account.SeatIndex]) {
	if spend, exists := c.spendsByID.Get(spendID); exists {
		return spend.Weight.Voters.Clone()
	}

	return ds.NewSet[account.SeatIndex]()
}

func (c *SpendDAG[SpendID, ResourceID, VoteRank]) ConflictSets(spendID SpendID) (spendSets ds.Set[ResourceID], exists bool) {
	spend, exists := c.spendsByID.Get(spendID)
	if !exists {
		return nil, false
	}

	spendSets = ds.NewSet[ResourceID]()
	_ = spend.ConflictSets.ForEach(func(conflictSet *ConflictSet[SpendID, ResourceID, VoteRank]) error {
		spendSets.Add(conflictSet.ID)
		return nil
	})

	return spendSets, true
}

func (c *SpendDAG[SpendID, ResourceID, VoteRank]) SpendParents(spendID SpendID) (spendParents ds.Set[SpendID], exists bool) {
	spend, exists := c.spendsByID.Get(spendID)
	if !exists {
		return nil, false
	}

	spendParents = ds.NewSet[SpendID]()
	_ = spend.Parents.ForEach(func(parent *Spend[SpendID, ResourceID, VoteRank]) error {
		spendParents.Add(parent.ID)
		return nil
	})

	return spendParents, true
}

func (c *SpendDAG[SpendID, ResourceID, VoteRank]) SpendChildren(spendID SpendID) (spendChildren ds.Set[SpendID], exists bool) {
	spend, exists := c.spendsByID.Get(spendID)
	if !exists {
		return nil, false
	}

	spendChildren = ds.NewSet[SpendID]()
	_ = spend.Children.ForEach(func(parent *Spend[SpendID, ResourceID, VoteRank]) error {
		spendChildren.Add(parent.ID)
		return nil
	})

	return spendChildren, true
}

func (c *SpendDAG[SpendID, ResourceID, VoteRank]) ConflictSetMembers(conflictSetID ResourceID) (conflicts ds.Set[SpendID], exists bool) {
	conflictSet, exists := c.conflictSetsByID.Get(conflictSetID)
	if !exists {
		return nil, false
	}

	conflicts = ds.NewSet[SpendID]()
	_ = conflictSet.ForEach(func(parent *Spend[SpendID, ResourceID, VoteRank]) error {
		conflicts.Add(parent.ID)
		return nil
	})

	return conflicts, true
}

func (c *SpendDAG[SpendID, ResourceID, VoteRank]) SpendWeight(spendID SpendID) int64 {
	if spend, exists := c.spendsByID.Get(spendID); exists {
		return spend.Weight.Value().ValidatorsWeight()
	}

	return 0
}

// CastVotes applies the given votes to the spenddag.
func (c *SpendDAG[SpendID, ResourceID, VoteRank]) CastVotes(vote *vote.Vote[VoteRank], spendIDs ds.Set[SpendID]) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	c.votingMutex.Lock(vote.Voter)
	defer c.votingMutex.Unlock(vote.Voter)

	supportedSpends, revokedSpends, err := c.determineVotes(spendIDs)
	if err != nil {
		return ierrors.Errorf("failed to determine votes: %w", err)
	}

	for supportedSpend := supportedSpends.Iterator(); supportedSpend.HasNext(); {
		supportedSpend.Next().ApplyVote(vote.WithLiked(true))
	}

	for revokedSpend := revokedSpends.Iterator(); revokedSpend.HasNext(); {
		revokedSpend.Next().ApplyVote(vote.WithLiked(false))
	}

	return nil
}

func (c *SpendDAG[SpendID, ResourceID, VoteRank]) AcceptanceState(spendIDs ds.Set[SpendID]) acceptance.State {
	lowestObservedState := acceptance.Accepted
	if err := spendIDs.ForEach(func(spendID SpendID) error {
		spend, exists := c.spendsByID.Get(spendID)
		if !exists {
			return ierrors.Errorf("tried to retrieve non-existing spend: %w", spenddag.ErrFatal)
		}

		if spend.IsRejected() {
			lowestObservedState = acceptance.Rejected

			return spenddag.ErrExpected
		}

		if spend.IsPending() {
			lowestObservedState = acceptance.Pending
		}

		return nil
	}); err != nil && !ierrors.Is(err, spenddag.ErrExpected) {
		panic(err)
	}

	return lowestObservedState
}

func (c *SpendDAG[SpendID, ResourceID, VoteRank]) SetAccepted(spendID SpendID) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if spend, exists := c.spendsByID.Get(spendID); exists {
		spend.setAcceptanceState(acceptance.Accepted)
	}
}

// UnacceptedSpends takes a set of SpendIDs and removes all the accepted Spends (leaving only the
// pending or rejected ones behind).
func (c *SpendDAG[SpendID, ResourceID, VoteRank]) UnacceptedSpends(spendIDs ds.Set[SpendID]) ds.Set[SpendID] {
	pendingSpendIDs := ds.NewSet[SpendID]()
	spendIDs.Range(func(currentSpendID SpendID) {
		if spend, exists := c.spendsByID.Get(currentSpendID); exists && !spend.IsAccepted() {
			pendingSpendIDs.Add(currentSpendID)
		}
	})

	return pendingSpendIDs
}

// EvictConflict removes spend with given SpendID from spenddag.
func (c *SpendDAG[SpendID, ResourceID, VoteRank]) EvictSpend(spendID SpendID) {
	for _, evictedSpendID := range func() []SpendID {
		c.mutex.RLock()
		defer c.mutex.RUnlock()

		return c.evictSpend(spendID)
	}() {
		c.events.SpendEvicted.Trigger(evictedSpendID)
	}
}

func (c *SpendDAG[SpendID, ResourceID, VoteRank]) evictSpend(spendID SpendID) []SpendID {
	// evicting an already evicted spend is fine
	spend, exists := c.spendsByID.Get(spendID)
	if !exists {
		return nil
	}

	evictedSpendIDs := spend.Evict()

	// remove the conflicts from the spenddag dictionary
	for _, evictedSpendID := range evictedSpendIDs {
		c.spendsByID.Delete(evictedSpendID)
	}

	// unhook the spend events and remove the unhook method from the storage
	unhookFunc, unhookExists := c.spendUnhooks.Get(spendID)
	if unhookExists {
		unhookFunc()
		c.spendUnhooks.Delete(spendID)
	}

	return evictedSpendIDs
}

// spends returns the Spends that are associated with the given SpendIDs. If ignoreMissing is set to true, it
// will ignore missing Spends instead of returning an ErrEntityEvicted error.
func (c *SpendDAG[SpendID, ResourceID, VoteRank]) spends(ids ds.Set[SpendID], ignoreMissing bool) (ds.Set[*Spend[SpendID, ResourceID, VoteRank]], error) {
	spends := ds.NewSet[*Spend[SpendID, ResourceID, VoteRank]]()

	return spends, ids.ForEach(func(id SpendID) (err error) {
		existingSpend, exists := c.spendsByID.Get(id)
		if exists {
			spends.Add(existingSpend)
		}

		return lo.Cond(exists || ignoreMissing, nil, ierrors.Errorf("tried to retrieve a non-existing spend with %s: %w", id, spenddag.ErrEntityEvicted))
	})
}

// conflictSets returns the ConflictSets that are associated with the given ResourceIDs. If createMissing is set to
// true, it will create an empty ConflictSets for each missing ResourceID.
func (c *SpendDAG[SpendID, ResourceID, VoteRank]) conflictSets(resourceIDs ds.Set[ResourceID]) ds.Set[*ConflictSet[SpendID, ResourceID, VoteRank]] {
	conflictSets := ds.NewSet[*ConflictSet[SpendID, ResourceID, VoteRank]]()

	resourceIDs.Range(func(resourceID ResourceID) {
		conflictSets.Add(lo.Return1(c.conflictSetsByID.GetOrCreate(resourceID, c.conflictSetFactory(resourceID))))
	})

	return conflictSets
}

// determineVotes determines the Spends that are supported and revoked by the given SpendIDs.
func (c *SpendDAG[SpendID, ResourceID, VoteRank]) determineVotes(spendIDs ds.Set[SpendID]) (supportedSpends ds.Set[*Spend[SpendID, ResourceID, VoteRank]], revokedSpends ds.Set[*Spend[SpendID, ResourceID, VoteRank]], err error) {
	supportedSpends = ds.NewSet[*Spend[SpendID, ResourceID, VoteRank]]()
	revokedSpends = ds.NewSet[*Spend[SpendID, ResourceID, VoteRank]]()

	revokedWalker := walker.New[*Spend[SpendID, ResourceID, VoteRank]]()
	revokeConflict := func(revokedConflict *Spend[SpendID, ResourceID, VoteRank]) error {
		if revokedSpends.Add(revokedConflict) {
			if supportedSpends.Has(revokedConflict) {
				return ierrors.Errorf("applied conflicting votes (%s is supported and revoked)", revokedConflict.ID)
			}

			revokedWalker.PushAll(revokedConflict.Children.ToSlice()...)
		}

		return nil
	}

	supportedWalker := walker.New[*Spend[SpendID, ResourceID, VoteRank]]()
	supportConflict := func(supportedConflict *Spend[SpendID, ResourceID, VoteRank]) error {
		if supportedSpends.Add(supportedConflict) {
			if err := supportedConflict.ConflictingSpends.ForEach(revokeConflict); err != nil {
				return ierrors.Errorf("failed to collect conflicting conflicts: %w", err)
			}

			supportedWalker.PushAll(supportedConflict.Parents.ToSlice()...)
		}

		return nil
	}

	for supportedWalker.PushAll(lo.Return1(c.spends(spendIDs, true)).ToSlice()...); supportedWalker.HasNext(); {
		if err := supportConflict(supportedWalker.Next()); err != nil {
			return nil, nil, ierrors.Errorf("failed to collect supported conflicts: %w", err)
		}
	}

	for revokedWalker.HasNext() {
		if revokedConflict := revokedWalker.Next(); revokedSpends.Add(revokedConflict) {
			revokedWalker.PushAll(revokedConflict.Children.ToSlice()...)
		}
	}

	return supportedSpends, revokedSpends, nil
}

func (c *SpendDAG[SpendID, ResourceID, VoteRank]) conflictSetFactory(resourceID ResourceID) func() *ConflictSet[SpendID, ResourceID, VoteRank] {
	return func() *ConflictSet[SpendID, ResourceID, VoteRank] {
		conflictSet := NewConflictSet[SpendID, ResourceID, VoteRank](resourceID)

		conflictSet.OnAllMembersEvicted(func(prevValue bool, newValue bool) {
			if newValue && !prevValue {
				c.conflictSetsByID.Delete(conflictSet.ID)
			}
		})

		return conflictSet
	}
}
