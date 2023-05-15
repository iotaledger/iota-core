package conflictdagv1

import (
	"sync"

	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ds/walker"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/acceptance"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/core/weight"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	iotago "github.com/iotaledger/iota.go/v4"
)

// ConflictDAG represents a data structure that tracks causal relationships between Conflicts and that allows to
// efficiently manage these Conflicts (and vote on their fate).
type ConflictDAG[ConflictID, ResourceID conflictdag.IDType, VotePower conflictdag.VotePowerType[VotePower]] struct {
	// events contains the events of the ConflictDAG.
	events *conflictdag.Events[ConflictID, ResourceID]

	// committeeSet is the set of validators that are allowed to vote on Conflicts.
	committeeSet *account.SelectedAccounts[iotago.AccountID, *iotago.AccountID]

	// conflictsByID is a mapping of ConflictIDs to Conflicts.
	conflictsByID *shrinkingmap.ShrinkingMap[ConflictID, *Conflict[ConflictID, ResourceID, VotePower]]

	conflictUnhooks *shrinkingmap.ShrinkingMap[ConflictID, func()]

	// conflictSetsByID is a mapping of ResourceIDs to ConflictSets.
	conflictSetsByID *shrinkingmap.ShrinkingMap[ResourceID, *ConflictSet[ConflictID, ResourceID, VotePower]]

	// pendingTasks is a counter that keeps track of the number of pending tasks.
	pendingTasks *syncutils.Counter

	// mutex is used to synchronize access to the ConflictDAG.
	mutex sync.RWMutex

	// votingMutex is used to synchronize voting for different identities.
	votingMutex *syncutils.DAGMutex[iotago.AccountID]
}

// New creates a new ConflictDAG.
func New[ConflictID, ResourceID conflictdag.IDType, VotePower conflictdag.VotePowerType[VotePower]](committeeSet *account.SelectedAccounts[iotago.AccountID, *iotago.AccountID]) *ConflictDAG[ConflictID, ResourceID, VotePower] {
	return &ConflictDAG[ConflictID, ResourceID, VotePower]{
		events: conflictdag.NewEvents[ConflictID, ResourceID](),

		committeeSet:     committeeSet,
		conflictsByID:    shrinkingmap.New[ConflictID, *Conflict[ConflictID, ResourceID, VotePower]](),
		conflictUnhooks:  shrinkingmap.New[ConflictID, func()](),
		conflictSetsByID: shrinkingmap.New[ResourceID, *ConflictSet[ConflictID, ResourceID, VotePower]](),
		pendingTasks:     syncutils.NewCounter(),
		votingMutex:      syncutils.NewDAGMutex[iotago.AccountID](),
	}
}

var _ conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, vote.MockedPower] = &ConflictDAG[iotago.TransactionID, iotago.OutputID, vote.MockedPower]{}

// Events returns the events of the ConflictDAG.
func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) Events() *conflictdag.Events[ConflictID, ResourceID] {
	return c.events
}

// CreateConflict creates a new Conflict that is conflicting over the given ResourceIDs and that has the given parents.
func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) CreateConflict(id ConflictID, resourceIDs *advancedset.AdvancedSet[ResourceID], initialAcceptanceState acceptance.State) error {
	err := func() error {
		c.mutex.RLock()
		defer c.mutex.RUnlock()

		conflictSets, err := c.conflictSets(resourceIDs, true /*!initialAcceptanceState.IsRejected()*/)
		if err != nil {
			return xerrors.Errorf("failed to create ConflictSet: %w", err)
		}

		if _, isNew := c.conflictsByID.GetOrCreate(id, func() *Conflict[ConflictID, ResourceID, VotePower] {
			initialWeight := weight.New(c.committeeSet)
			initialWeight.SetAcceptanceState(initialAcceptanceState)

			newConflict := NewConflict[ConflictID, ResourceID, VotePower](id, conflictSets, initialWeight, c.pendingTasks, acceptance.ThresholdProvider(c.committeeSet.TotalWeight))

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
		}); !isNew {
			return xerrors.Errorf("tried to create conflict with %s twice: %w", id, conflictdag.ErrConflictExists)
		}

		return nil
	}()

	if err == nil {
		c.events.ConflictCreated.Trigger(id)
	}

	return err
}

// ReadConsistent write locks the ConflictDAG and exposes read-only methods to the callback to perform multiple reads while maintaining the same ConflictDAG state.
func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) ReadConsistent(callback func(conflictDAG conflictdag.ReadLockedConflictDAG[ConflictID, ResourceID, VotePower]) error) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.pendingTasks.WaitIsZero()

	return callback(c)
}

// JoinConflictSets adds the Conflict to the given ConflictSets and returns true if the conflict membership was modified during this operation.
func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) JoinConflictSets(conflictID ConflictID, resourceIDs *advancedset.AdvancedSet[ResourceID]) error {
	joinedConflictSets, err := func() (*advancedset.AdvancedSet[ResourceID], error) {
		c.mutex.RLock()
		defer c.mutex.RUnlock()

		currentConflict, exists := c.conflictsByID.Get(conflictID)
		if !exists {
			return nil, xerrors.Errorf("tried to modify evicted conflict with %s: %w", conflictID, conflictdag.ErrEntityEvicted)
		}

		conflictSets, err := c.conflictSets(resourceIDs, true /*!currentConflict.IsRejected()*/)
		if err != nil {
			return nil, xerrors.Errorf("failed to join conflict sets: %w", err)
		}

		joinedConflictSets, err := currentConflict.JoinConflictSets(conflictSets)
		if err != nil {
			return nil, xerrors.Errorf("failed to join conflict sets: %w", err)
		}

		return joinedConflictSets, nil
	}()
	if err != nil {
		return err
	}

	if !joinedConflictSets.IsEmpty() {
		c.events.ConflictingResourcesAdded.Trigger(conflictID, joinedConflictSets)
	}

	return nil
}

// UpdateConflictParents updates the parents of the given Conflict and returns an error if the operation failed.
func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) UpdateConflictParents(conflictID ConflictID, addedParentIDs, removedParentIDs *advancedset.AdvancedSet[ConflictID]) error {
	newParents := advancedset.New[ConflictID]()

	updated, err := func() (bool, error) {
		c.mutex.RLock()
		defer c.mutex.RUnlock()

		currentConflict, currentConflictExists := c.conflictsByID.Get(conflictID)
		if !currentConflictExists {
			return false, xerrors.Errorf("tried to modify evicted conflict with %s: %w", conflictID, conflictdag.ErrEntityEvicted)
		}
		addedParents := advancedset.New[*Conflict[ConflictID, ResourceID, VotePower]]()

		if err := addedParentIDs.ForEach(func(addedParentID ConflictID) error {
			addedParent, addedParentExists := c.conflictsByID.Get(addedParentID)
			if !addedParentExists {
				if !currentConflict.IsRejected() {
					// UpdateConflictParents is only called when a Conflict is forked, which means that the added parent
					// must exist (unless it was forked on top of a rejected branch, just before eviction).
					return xerrors.Errorf("tried to add non-existent parent with %s: %w", addedParentID, conflictdag.ErrFatal)
				}

				return xerrors.Errorf("tried to add evicted parent with %s to rejected conflict with %s: %w", addedParentID, conflictID, conflictdag.ErrEntityEvicted)
			}

			addedParents.Add(addedParent)

			return nil
		}); err != nil {
			return false, err
		}

		removedParents, err := c.conflicts(removedParentIDs, !currentConflict.IsRejected())
		if err != nil {
			return false, xerrors.Errorf("failed to update conflict parents: %w", err)
		}

		updated := currentConflict.UpdateParents(addedParents, removedParents)
		if updated {
			_ = currentConflict.Parents.ForEach(func(parentConflict *Conflict[ConflictID, ResourceID, VotePower]) (err error) {
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
func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) LikedInstead(conflictIDs *advancedset.AdvancedSet[ConflictID]) *advancedset.AdvancedSet[ConflictID] {
	likedInstead := advancedset.New[ConflictID]()
	conflictIDs.Range(func(conflictID ConflictID) {
		if currentConflict, exists := c.conflictsByID.Get(conflictID); exists {
			if likedConflict := heaviestConflict(currentConflict.LikedInstead()); likedConflict != nil {
				likedInstead.Add(likedConflict.ID)
			}
		}
	})

	return likedInstead
}

func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) FutureCone(conflictIDs *advancedset.AdvancedSet[ConflictID]) (futureCone *advancedset.AdvancedSet[ConflictID]) {
	futureCone = advancedset.New[ConflictID]()
	for futureConeWalker := walker.New[*Conflict[ConflictID, ResourceID, VotePower]]().PushAll(lo.Return1(c.conflicts(conflictIDs, true)).Slice()...); futureConeWalker.HasNext(); {
		if conflict := futureConeWalker.Next(); futureCone.Add(conflict.ID) {
			futureConeWalker.PushAll(conflict.Children.Slice()...)
		}
	}

	return futureCone
}

func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) ConflictingConflicts(conflictID ConflictID) (conflictingConflicts *advancedset.AdvancedSet[ConflictID], exists bool) {
	conflict, exists := c.conflictsByID.Get(conflictID)
	if !exists {
		return nil, false
	}

	conflictingConflicts = advancedset.New[ConflictID]()
	conflict.ConflictingConflicts.Range(func(conflictingConflict *Conflict[ConflictID, ResourceID, VotePower]) {
		conflictingConflicts.Add(conflictingConflict.ID)
	})

	return conflictingConflicts, true
}

func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) AllConflictsSupported(issuerID iotago.AccountID, conflictIDs *advancedset.AdvancedSet[ConflictID]) bool {
	return lo.Return1(c.conflicts(conflictIDs, true)).ForEach(func(conflict *Conflict[ConflictID, ResourceID, VotePower]) (err error) {
		lastVote, exists := conflict.LatestVotes.Get(issuerID)

		return lo.Cond(exists && lastVote.IsLiked(), nil, xerrors.Errorf("conflict with %s is not supported by %s", conflict.ID, issuerID))
	}) == nil
}

func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) ConflictVoters(conflictID ConflictID) (conflictVoters map[iotago.AccountID]int64) {
	conflictVoters = make(map[iotago.AccountID]int64)

	conflict, exists := c.conflictsByID.Get(conflictID)
	if exists {
		_ = conflict.Weight.Voters.ForEach(func(id iotago.AccountID, weight int64) error {
			conflictVoters[id] = weight
			return nil
		})
	}

	return conflictVoters
}

func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) ConflictSets(conflictID ConflictID) (conflictSets *advancedset.AdvancedSet[ResourceID], exists bool) {
	conflict, exists := c.conflictsByID.Get(conflictID)
	if !exists {
		return nil, false
	}

	conflictSets = advancedset.New[ResourceID]()
	_ = conflict.ConflictSets.ForEach(func(conflictSet *ConflictSet[ConflictID, ResourceID, VotePower]) error {
		conflictSets.Add(conflictSet.ID)
		return nil
	})

	return conflictSets, true
}

func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) ConflictParents(conflictID ConflictID) (conflictParents *advancedset.AdvancedSet[ConflictID], exists bool) {
	conflict, exists := c.conflictsByID.Get(conflictID)
	if !exists {
		return nil, false
	}

	conflictParents = advancedset.New[ConflictID]()
	_ = conflict.Parents.ForEach(func(parent *Conflict[ConflictID, ResourceID, VotePower]) error {
		conflictParents.Add(parent.ID)
		return nil
	})

	return conflictParents, true
}

func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) ConflictChildren(conflictID ConflictID) (conflictChildren *advancedset.AdvancedSet[ConflictID], exists bool) {
	conflict, exists := c.conflictsByID.Get(conflictID)
	if !exists {
		return nil, false
	}

	conflictChildren = advancedset.New[ConflictID]()
	_ = conflict.Children.ForEach(func(parent *Conflict[ConflictID, ResourceID, VotePower]) error {
		conflictChildren.Add(parent.ID)
		return nil
	})

	return conflictChildren, true
}

func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) ConflictSetMembers(conflictSetID ResourceID) (conflicts *advancedset.AdvancedSet[ConflictID], exists bool) {
	conflictSet, exists := c.conflictSetsByID.Get(conflictSetID)
	if !exists {
		return nil, false
	}

	conflicts = advancedset.New[ConflictID]()
	_ = conflictSet.ForEach(func(parent *Conflict[ConflictID, ResourceID, VotePower]) error {
		conflicts.Add(parent.ID)
		return nil
	})

	return conflicts, true
}

func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) ConflictWeight(conflictID ConflictID) int64 {
	if conflict, exists := c.conflictsByID.Get(conflictID); exists {
		return conflict.Weight.Value().ValidatorsWeight()
	}

	return 0
}

// CastVotes applies the given votes to the ConflictDAG.
func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) CastVotes(vote *vote.Vote[VotePower], conflictIDs *advancedset.AdvancedSet[ConflictID]) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	c.votingMutex.Lock(vote.Voter)
	defer c.votingMutex.Unlock(vote.Voter)
	supportedConflicts, revokedConflicts, err := c.determineVotes(conflictIDs)
	if err != nil {
		return xerrors.Errorf("failed to determine votes: %w", err)
	}

	if c.committeeSet.Has(vote.Voter) {
		for supportedConflict := supportedConflicts.Iterator(); supportedConflict.HasNext(); {
			supportedConflict.Next().ApplyVote(vote.WithLiked(true))
		}

		for revokedConflict := revokedConflicts.Iterator(); revokedConflict.HasNext(); {
			revokedConflict.Next().ApplyVote(vote.WithLiked(false))
		}
	}

	return nil
}

func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) AcceptanceState(conflictIDs *advancedset.AdvancedSet[ConflictID]) acceptance.State {
	lowestObservedState := acceptance.Accepted
	if err := conflictIDs.ForEach(func(conflictID ConflictID) error {
		conflict, exists := c.conflictsByID.Get(conflictID)
		if !exists {
			return xerrors.Errorf("tried to retrieve non-existing conflict: %w", conflictdag.ErrFatal)
		}

		if conflict.IsRejected() {
			lowestObservedState = acceptance.Rejected

			return conflictdag.ErrExpected
		}

		if conflict.IsPending() {
			lowestObservedState = acceptance.Pending
		}

		return nil
	}); err != nil && !errors.Is(err, conflictdag.ErrExpected) {
		panic(err)
	}

	return lowestObservedState
}

// UnacceptedConflicts takes a set of ConflictIDs and removes all the accepted Conflicts (leaving only the
// pending or rejected ones behind).
func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) UnacceptedConflicts(conflictIDs *advancedset.AdvancedSet[ConflictID]) *advancedset.AdvancedSet[ConflictID] {
	// TODO: introduce optsMergeToMaster
	// if !c.optsMergeToMaster {
	//	return conflictIDs.Clone()
	// }

	pendingConflictIDs := advancedset.New[ConflictID]()
	conflictIDs.Range(func(currentConflictID ConflictID) {
		if conflict, exists := c.conflictsByID.Get(currentConflictID); exists && !conflict.IsAccepted() {
			pendingConflictIDs.Add(currentConflictID)
		}
	})

	return pendingConflictIDs
}

// EvictConflict removes conflict with given ConflictID from ConflictDAG.
func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) EvictConflict(conflictID ConflictID) error {
	evictedConflictIDs, err := func() ([]ConflictID, error) {
		c.mutex.RLock()
		defer c.mutex.RUnlock()

		// evicting an already evicted conflict is fine
		conflict, exists := c.conflictsByID.Get(conflictID)
		if !exists {
			return nil, nil
		}

		// abort if we faced an error while evicting the conflict
		evictedConflictIDs, err := conflict.Evict()
		if err != nil {
			return nil, xerrors.Errorf("failed to evict conflict with %s: %w", conflictID, err)
		}

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

		return evictedConflictIDs, nil
	}()
	if err != nil {
		return xerrors.Errorf("failed to evict conflict with %s: %w", conflictID, err)
	}

	// trigger the ConflictEvicted event
	for _, evictedConflictID := range evictedConflictIDs {
		c.events.ConflictEvicted.Trigger(evictedConflictID)
	}

	return nil
}

// conflicts returns the Conflicts that are associated with the given ConflictIDs. If ignoreMissing is set to true, it
// will ignore missing Conflicts instead of returning an ErrEntityEvicted error.
func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) conflicts(ids *advancedset.AdvancedSet[ConflictID], ignoreMissing bool) (*advancedset.AdvancedSet[*Conflict[ConflictID, ResourceID, VotePower]], error) {
	conflicts := advancedset.New[*Conflict[ConflictID, ResourceID, VotePower]]()

	return conflicts, ids.ForEach(func(id ConflictID) (err error) {
		existingConflict, exists := c.conflictsByID.Get(id)
		if exists {
			conflicts.Add(existingConflict)
		}

		return lo.Cond(exists || ignoreMissing, nil, xerrors.Errorf("tried to retrieve an evicted conflict with %s: %w", id, conflictdag.ErrEntityEvicted))
	})
}

// conflictSets returns the ConflictSets that are associated with the given ResourceIDs. If createMissing is set to
// true, it will create an empty ConflictSet for each missing ResourceID.
func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) conflictSets(resourceIDs *advancedset.AdvancedSet[ResourceID], createMissing bool) (*advancedset.AdvancedSet[*ConflictSet[ConflictID, ResourceID, VotePower]], error) {
	conflictSets := advancedset.New[*ConflictSet[ConflictID, ResourceID, VotePower]]()

	return conflictSets, resourceIDs.ForEach(func(resourceID ResourceID) (err error) {
		if createMissing {
			conflictSets.Add(lo.Return1(c.conflictSetsByID.GetOrCreate(resourceID, c.conflictSetFactory(resourceID))))

			return nil
		}

		if conflictSet, exists := c.conflictSetsByID.Get(resourceID); exists {
			conflictSets.Add(conflictSet)

			return nil
		}

		return xerrors.Errorf("tried to create a Conflict with evicted Resource: %w", conflictdag.ErrEntityEvicted)
	})
}

// determineVotes determines the Conflicts that are supported and revoked by the given ConflictIDs.
func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) determineVotes(conflictIDs *advancedset.AdvancedSet[ConflictID]) (supportedConflicts, revokedConflicts *advancedset.AdvancedSet[*Conflict[ConflictID, ResourceID, VotePower]], err error) {
	supportedConflicts = advancedset.New[*Conflict[ConflictID, ResourceID, VotePower]]()
	revokedConflicts = advancedset.New[*Conflict[ConflictID, ResourceID, VotePower]]()

	revokedWalker := walker.New[*Conflict[ConflictID, ResourceID, VotePower]]()
	revokeConflict := func(revokedConflict *Conflict[ConflictID, ResourceID, VotePower]) error {
		if revokedConflicts.Add(revokedConflict) {
			if supportedConflicts.Has(revokedConflict) {
				return xerrors.Errorf("applied conflicting votes (%s is supported and revoked)", revokedConflict.ID)
			}

			revokedWalker.PushAll(revokedConflict.Children.Slice()...)
		}

		return nil
	}

	supportedWalker := walker.New[*Conflict[ConflictID, ResourceID, VotePower]]()
	supportConflict := func(supportedConflict *Conflict[ConflictID, ResourceID, VotePower]) error {
		if supportedConflicts.Add(supportedConflict) {
			if err := supportedConflict.ConflictingConflicts.ForEach(revokeConflict); err != nil {
				return xerrors.Errorf("failed to collect conflicting conflicts: %w", err)
			}

			supportedWalker.PushAll(supportedConflict.Parents.Slice()...)
		}

		return nil
	}

	for supportedWalker.PushAll(lo.Return1(c.conflicts(conflictIDs, true)).Slice()...); supportedWalker.HasNext(); {
		if err := supportConflict(supportedWalker.Next()); err != nil {
			return nil, nil, xerrors.Errorf("failed to collect supported conflicts: %w", err)
		}
	}

	for revokedWalker.HasNext() {
		if revokedConflict := revokedWalker.Next(); revokedConflicts.Add(revokedConflict) {
			revokedWalker.PushAll(revokedConflict.Children.Slice()...)
		}
	}

	return supportedConflicts, revokedConflicts, nil
}

func (c *ConflictDAG[ConflictID, ResourceID, VotePower]) conflictSetFactory(resourceID ResourceID) func() *ConflictSet[ConflictID, ResourceID, VotePower] {
	return func() *ConflictSet[ConflictID, ResourceID, VotePower] {
		// TODO: listen to ConflictEvicted events and remove the Conflict from the ConflictSet

		return NewConflictSet[ConflictID, ResourceID, VotePower](resourceID)
	}
}
