package conflictdagv1

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
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	iotago "github.com/iotaledger/iota.go/v4"
)

// ConflictDAG represents a data structure that tracks causal relationships between Conflicts and that allows to
// efficiently manage these Conflicts (and vote on their fate).
type ConflictDAG[ConflictID, ResourceID conflictdag.IDType, VoteRank conflictdag.VoteRankType[VoteRank]] struct {
	// events contains the events of the ConflictDAG.
	events *conflictdag.Events[ConflictID, ResourceID]

	// seatCount is a function that returns the number of seats.
	seatCount func() int

	// conflictsByID is a mapping of ConflictIDs to Conflicts.
	conflictsByID *shrinkingmap.ShrinkingMap[ConflictID, *Conflict[ConflictID, ResourceID, VoteRank]]

	conflictUnhooks *shrinkingmap.ShrinkingMap[ConflictID, func()]

	// conflictSetsByID is a mapping of ResourceIDs to ConflictSets.
	conflictSetsByID *shrinkingmap.ShrinkingMap[ResourceID, *ConflictSet[ConflictID, ResourceID, VoteRank]]

	// pendingTasks is a counter that keeps track of the number of pending tasks.
	pendingTasks *syncutils.Counter

	// mutex is used to synchronize access to the ConflictDAG.
	mutex syncutils.RWMutex

	// votingMutex is used to synchronize voting for different identities.
	votingMutex *syncutils.DAGMutex[account.SeatIndex]
}

// New creates a new ConflictDAG.
func New[ConflictID, ResourceID conflictdag.IDType, VoteRank conflictdag.VoteRankType[VoteRank]](seatCount func() int) *ConflictDAG[ConflictID, ResourceID, VoteRank] {
	return &ConflictDAG[ConflictID, ResourceID, VoteRank]{
		events: conflictdag.NewEvents[ConflictID, ResourceID](),

		seatCount:        seatCount,
		conflictsByID:    shrinkingmap.New[ConflictID, *Conflict[ConflictID, ResourceID, VoteRank]](),
		conflictUnhooks:  shrinkingmap.New[ConflictID, func()](),
		conflictSetsByID: shrinkingmap.New[ResourceID, *ConflictSet[ConflictID, ResourceID, VoteRank]](),
		pendingTasks:     syncutils.NewCounter(),
		votingMutex:      syncutils.NewDAGMutex[account.SeatIndex](),
	}
}

var _ conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, vote.MockedRank] = &ConflictDAG[iotago.TransactionID, iotago.OutputID, vote.MockedRank]{}

// Shutdown shuts down the ConflictDAG.
func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) Shutdown() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.conflictsByID.ForEach(func(conflictID ConflictID, conflict *Conflict[ConflictID, ResourceID, VoteRank]) bool {
		conflict.Shutdown()

		return true
	})
}

// Events returns the events of the ConflictDAG.
func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) Events() *conflictdag.Events[ConflictID, ResourceID] {
	return c.events
}

// CreateConflict creates a new Conflict.
func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) CreateConflict(id ConflictID) {
	if func() (created bool) {
		c.mutex.RLock()
		defer c.mutex.RUnlock()

		_, isNewConflict := c.conflictsByID.GetOrCreate(id, func() *Conflict[ConflictID, ResourceID, VoteRank] {
			newConflict := NewConflict[ConflictID, ResourceID, VoteRank](id, weight.New(), c.pendingTasks, acceptance.ThresholdProvider(func() int64 { return int64(c.seatCount()) }))

			// attach to the acceptance state updated event and propagate that event to the outside.
			// also need to remember the unhook method to properly evict the conflict.
			c.conflictUnhooks.Set(id, newConflict.AcceptanceStateUpdated.Hook(func(oldState, newState acceptance.State) {
				if newState.IsAccepted() {
					c.events.ConflictAccepted.Trigger(newConflict.ID)
					return
				}
				if newState.IsRejected() {
					c.events.ConflictRejected.Trigger(newConflict.ID)
				}
			}).Unhook)

			return newConflict
		})

		return isNewConflict
	}() {
		c.events.ConflictCreated.Trigger(id)
	}
}

func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) UpdateConflictingResources(id ConflictID, resourceIDs ds.Set[ResourceID]) error {
	joinedConflictSets, err := func() (ds.Set[ResourceID], error) {
		c.mutex.RLock()
		defer c.mutex.RUnlock()

		conflict, exists := c.conflictsByID.Get(id)
		if !exists {
			return nil, ierrors.Errorf("conflict already evicted: %w", conflictdag.ErrEntityEvicted)
		}

		return conflict.JoinConflictSets(c.conflictSets(resourceIDs))
	}()

	if err != nil {
		return ierrors.Errorf("conflict %s failed to join conflict sets: %w", id, err)
	}

	if !joinedConflictSets.IsEmpty() {
		c.events.ConflictingResourcesAdded.Trigger(id, joinedConflictSets)
	}

	return nil
}

// ReadConsistent write locks the ConflictDAG and exposes read-only methods to the callback to perform multiple reads while maintaining the same ConflictDAG state.
func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) ReadConsistent(callback func(conflictDAG conflictdag.ReadLockedConflictDAG[ConflictID, ResourceID, VoteRank]) error) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.pendingTasks.WaitIsZero()

	return callback(c)
}

// UpdateConflictParents updates the parents of the given Conflict and returns an error if the operation failed.
func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) UpdateConflictParents(conflictID ConflictID, addedParentIDs, removedParentIDs ds.Set[ConflictID]) error {
	newParents := ds.NewSet[ConflictID]()

	updated, err := func() (bool, error) {
		c.mutex.RLock()
		defer c.mutex.RUnlock()

		currentConflict, currentConflictExists := c.conflictsByID.Get(conflictID)
		if !currentConflictExists {
			return false, ierrors.Errorf("tried to modify evicted conflict with %s: %w", conflictID, conflictdag.ErrEntityEvicted)
		}

		addedParents := ds.NewSet[*Conflict[ConflictID, ResourceID, VoteRank]]()

		if err := addedParentIDs.ForEach(func(addedParentID ConflictID) error {
			// If we cannot load the parent it is because it has been already evicted
			if addedParent, addedParentExists := c.conflictsByID.Get(addedParentID); addedParentExists {
				addedParents.Add(addedParent)
			}

			return nil
		}); err != nil {
			return false, err
		}

		removedParents, err := c.conflicts(removedParentIDs, !currentConflict.IsRejected())
		if err != nil {
			return false, ierrors.Errorf("failed to update conflict parents: %w", err)
		}

		updated := currentConflict.UpdateParents(addedParents, removedParents)
		if updated {
			_ = currentConflict.Parents.ForEach(func(parentConflict *Conflict[ConflictID, ResourceID, VoteRank]) (err error) {
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
		c.events.ConflictParentsUpdated.Trigger(conflictID, newParents)
	}

	return nil
}

// LikedInstead returns the ConflictIDs of the Conflicts that are liked instead of the Conflicts.
func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) LikedInstead(conflictIDs ds.Set[ConflictID]) ds.Set[ConflictID] {
	likedInstead := ds.NewSet[ConflictID]()
	conflictIDs.Range(func(conflictID ConflictID) {
		if currentConflict, exists := c.conflictsByID.Get(conflictID); exists {
			if likedConflict := heaviestConflict(currentConflict.LikedInstead()); likedConflict != nil {
				likedInstead.Add(likedConflict.ID)
			}
		}
	})

	return likedInstead
}

func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) FutureCone(conflictIDs ds.Set[ConflictID]) (futureCone ds.Set[ConflictID]) {
	futureCone = ds.NewSet[ConflictID]()
	for futureConeWalker := walker.New[*Conflict[ConflictID, ResourceID, VoteRank]]().PushAll(lo.Return1(c.conflicts(conflictIDs, true)).ToSlice()...); futureConeWalker.HasNext(); {
		if conflict := futureConeWalker.Next(); futureCone.Add(conflict.ID) {
			futureConeWalker.PushAll(conflict.Children.ToSlice()...)
		}
	}

	return futureCone
}

func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) ConflictingConflicts(conflictID ConflictID) (conflictingConflicts ds.Set[ConflictID], exists bool) {
	conflict, exists := c.conflictsByID.Get(conflictID)
	if !exists {
		return nil, false
	}

	conflictingConflicts = ds.NewSet[ConflictID]()
	conflict.ConflictingConflicts.Range(func(conflictingConflict *Conflict[ConflictID, ResourceID, VoteRank]) {
		conflictingConflicts.Add(conflictingConflict.ID)
	})

	return conflictingConflicts, true
}

func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) AllConflictsSupported(seat account.SeatIndex, conflictIDs ds.Set[ConflictID]) bool {
	return lo.Return1(c.conflicts(conflictIDs, true)).ForEach(func(conflict *Conflict[ConflictID, ResourceID, VoteRank]) (err error) {
		lastVote, exists := conflict.LatestVotes.Get(seat)

		return lo.Cond(exists && lastVote.IsLiked(), nil, ierrors.Errorf("conflict with %s is not supported by seat %d", conflict.ID, seat))
	}) == nil
}

func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) ConflictVoters(conflictID ConflictID) (conflictVoters ds.Set[account.SeatIndex]) {
	if conflict, exists := c.conflictsByID.Get(conflictID); exists {
		return conflict.Weight.Voters.Clone()
	}

	return ds.NewSet[account.SeatIndex]()
}

func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) ConflictSets(conflictID ConflictID) (conflictSets ds.Set[ResourceID], exists bool) {
	conflict, exists := c.conflictsByID.Get(conflictID)
	if !exists {
		return nil, false
	}

	conflictSets = ds.NewSet[ResourceID]()
	_ = conflict.ConflictSets.ForEach(func(conflictSet *ConflictSet[ConflictID, ResourceID, VoteRank]) error {
		conflictSets.Add(conflictSet.ID)
		return nil
	})

	return conflictSets, true
}

func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) ConflictParents(conflictID ConflictID) (conflictParents ds.Set[ConflictID], exists bool) {
	conflict, exists := c.conflictsByID.Get(conflictID)
	if !exists {
		return nil, false
	}

	conflictParents = ds.NewSet[ConflictID]()
	_ = conflict.Parents.ForEach(func(parent *Conflict[ConflictID, ResourceID, VoteRank]) error {
		conflictParents.Add(parent.ID)
		return nil
	})

	return conflictParents, true
}

func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) ConflictChildren(conflictID ConflictID) (conflictChildren ds.Set[ConflictID], exists bool) {
	conflict, exists := c.conflictsByID.Get(conflictID)
	if !exists {
		return nil, false
	}

	conflictChildren = ds.NewSet[ConflictID]()
	_ = conflict.Children.ForEach(func(parent *Conflict[ConflictID, ResourceID, VoteRank]) error {
		conflictChildren.Add(parent.ID)
		return nil
	})

	return conflictChildren, true
}

func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) ConflictSetMembers(conflictSetID ResourceID) (conflicts ds.Set[ConflictID], exists bool) {
	conflictSet, exists := c.conflictSetsByID.Get(conflictSetID)
	if !exists {
		return nil, false
	}

	conflicts = ds.NewSet[ConflictID]()
	_ = conflictSet.ForEach(func(parent *Conflict[ConflictID, ResourceID, VoteRank]) error {
		conflicts.Add(parent.ID)
		return nil
	})

	return conflicts, true
}

func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) ConflictWeight(conflictID ConflictID) int64 {
	if conflict, exists := c.conflictsByID.Get(conflictID); exists {
		return conflict.Weight.Value().ValidatorsWeight()
	}

	return 0
}

// CastVotes applies the given votes to the ConflictDAG.
func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) CastVotes(vote *vote.Vote[VoteRank], conflictIDs ds.Set[ConflictID]) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	c.votingMutex.Lock(vote.Voter)
	defer c.votingMutex.Unlock(vote.Voter)

	supportedConflicts, revokedConflicts, err := c.determineVotes(conflictIDs)
	if err != nil {
		return ierrors.Errorf("failed to determine votes: %w", err)
	}

	for supportedConflict := supportedConflicts.Iterator(); supportedConflict.HasNext(); {
		supportedConflict.Next().ApplyVote(vote.WithLiked(true))
	}

	for revokedConflict := revokedConflicts.Iterator(); revokedConflict.HasNext(); {
		revokedConflict.Next().ApplyVote(vote.WithLiked(false))
	}

	return nil
}

func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) AcceptanceState(conflictIDs ds.Set[ConflictID]) acceptance.State {
	lowestObservedState := acceptance.Accepted
	if err := conflictIDs.ForEach(func(conflictID ConflictID) error {
		conflict, exists := c.conflictsByID.Get(conflictID)
		if !exists {
			return ierrors.Errorf("tried to retrieve non-existing conflict: %w", conflictdag.ErrFatal)
		}

		if conflict.IsRejected() {
			lowestObservedState = acceptance.Rejected

			return conflictdag.ErrExpected
		}

		if conflict.IsPending() {
			lowestObservedState = acceptance.Pending
		}

		return nil
	}); err != nil && !ierrors.Is(err, conflictdag.ErrExpected) {
		panic(err)
	}

	return lowestObservedState
}

// UnacceptedConflicts takes a set of ConflictIDs and removes all the accepted Conflicts (leaving only the
// pending or rejected ones behind).
func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) UnacceptedConflicts(conflictIDs ds.Set[ConflictID]) ds.Set[ConflictID] {
	pendingConflictIDs := ds.NewSet[ConflictID]()
	conflictIDs.Range(func(currentConflictID ConflictID) {
		if conflict, exists := c.conflictsByID.Get(currentConflictID); exists && !conflict.IsAccepted() {
			pendingConflictIDs.Add(currentConflictID)
		}
	})

	return pendingConflictIDs
}

// EvictConflict removes conflict with given ConflictID from ConflictDAG.
func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) EvictConflict(conflictID ConflictID) {
	for _, evictedConflictID := range func() []ConflictID {
		c.mutex.RLock()
		defer c.mutex.RUnlock()

		return c.evictConflict(conflictID)
	}() {
		c.events.ConflictEvicted.Trigger(evictedConflictID)
	}
}

func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) evictConflict(conflictID ConflictID) []ConflictID {
	// evicting an already evicted conflict is fine
	conflict, exists := c.conflictsByID.Get(conflictID)
	if !exists {
		return nil
	}

	evictedConflictIDs := conflict.Evict()

	// remove the conflicts from the ConflictDAG dictionary
	for _, evictedConflictID := range evictedConflictIDs {
		c.conflictsByID.Delete(evictedConflictID)
	}

	// unhook the conflict events and remove the unhook method from the storage
	unhookFunc, unhookExists := c.conflictUnhooks.Get(conflictID)
	if unhookExists {
		unhookFunc()
		c.conflictUnhooks.Delete(conflictID)
	}

	return evictedConflictIDs
}

// conflicts returns the Conflicts that are associated with the given ConflictIDs. If ignoreMissing is set to true, it
// will ignore missing Conflicts instead of returning an ErrEntityEvicted error.
func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) conflicts(ids ds.Set[ConflictID], ignoreMissing bool) (ds.Set[*Conflict[ConflictID, ResourceID, VoteRank]], error) {
	conflicts := ds.NewSet[*Conflict[ConflictID, ResourceID, VoteRank]]()

	return conflicts, ids.ForEach(func(id ConflictID) (err error) {
		existingConflict, exists := c.conflictsByID.Get(id)
		if exists {
			conflicts.Add(existingConflict)
		}

		return lo.Cond(exists || ignoreMissing, nil, ierrors.Errorf("tried to retrieve a non-existing conflict with %s: %w", id, conflictdag.ErrEntityEvicted))
	})
}

// conflictSets returns the ConflictSets that are associated with the given ResourceIDs. If createMissing is set to
// true, it will create an empty ConflictSet for each missing ResourceID.
func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) conflictSets(resourceIDs ds.Set[ResourceID]) ds.Set[*ConflictSet[ConflictID, ResourceID, VoteRank]] {
	conflictSets := ds.NewSet[*ConflictSet[ConflictID, ResourceID, VoteRank]]()

	resourceIDs.Range(func(resourceID ResourceID) {
		conflictSets.Add(lo.Return1(c.conflictSetsByID.GetOrCreate(resourceID, c.conflictSetFactory(resourceID))))
	})

	return conflictSets
}

// determineVotes determines the Conflicts that are supported and revoked by the given ConflictIDs.
func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) determineVotes(conflictIDs ds.Set[ConflictID]) (supportedConflicts, revokedConflicts ds.Set[*Conflict[ConflictID, ResourceID, VoteRank]], err error) {
	supportedConflicts = ds.NewSet[*Conflict[ConflictID, ResourceID, VoteRank]]()
	revokedConflicts = ds.NewSet[*Conflict[ConflictID, ResourceID, VoteRank]]()

	revokedWalker := walker.New[*Conflict[ConflictID, ResourceID, VoteRank]]()
	revokeConflict := func(revokedConflict *Conflict[ConflictID, ResourceID, VoteRank]) error {
		if revokedConflicts.Add(revokedConflict) {
			if supportedConflicts.Has(revokedConflict) {
				return ierrors.Errorf("applied conflicting votes (%s is supported and revoked)", revokedConflict.ID)
			}

			revokedWalker.PushAll(revokedConflict.Children.ToSlice()...)
		}

		return nil
	}

	supportedWalker := walker.New[*Conflict[ConflictID, ResourceID, VoteRank]]()
	supportConflict := func(supportedConflict *Conflict[ConflictID, ResourceID, VoteRank]) error {
		if supportedConflicts.Add(supportedConflict) {
			if err := supportedConflict.ConflictingConflicts.ForEach(revokeConflict); err != nil {
				return ierrors.Errorf("failed to collect conflicting conflicts: %w", err)
			}

			supportedWalker.PushAll(supportedConflict.Parents.ToSlice()...)
		}

		return nil
	}

	for supportedWalker.PushAll(lo.Return1(c.conflicts(conflictIDs, true)).ToSlice()...); supportedWalker.HasNext(); {
		if err := supportConflict(supportedWalker.Next()); err != nil {
			return nil, nil, ierrors.Errorf("failed to collect supported conflicts: %w", err)
		}
	}

	for revokedWalker.HasNext() {
		if revokedConflict := revokedWalker.Next(); revokedConflicts.Add(revokedConflict) {
			revokedWalker.PushAll(revokedConflict.Children.ToSlice()...)
		}
	}

	return supportedConflicts, revokedConflicts, nil
}

func (c *ConflictDAG[ConflictID, ResourceID, VoteRank]) conflictSetFactory(resourceID ResourceID) func() *ConflictSet[ConflictID, ResourceID, VoteRank] {
	return func() *ConflictSet[ConflictID, ResourceID, VoteRank] {
		conflictSet := NewConflictSet[ConflictID, ResourceID, VoteRank](resourceID)

		conflictSet.OnAllMembersEvicted(func(prevValue, newValue bool) {
			if newValue && !prevValue {
				c.conflictSetsByID.Delete(conflictSet.ID)
			}
		})

		return conflictSet
	}
}
