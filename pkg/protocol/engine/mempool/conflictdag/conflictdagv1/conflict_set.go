package conflictdagv1

import (
	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
)

// ConflictSet represents a set of Conflicts that are conflicting with each other over a common Resource.
type ConflictSet[ConflictID, ResourceID conflictdag.IDType, VoteRank conflictdag.VoteRankType[VoteRank]] struct {
	// ID is the ID of the Resource that the Conflicts in this ConflictSet are conflicting over.
	ID ResourceID

	// members is the set of Conflicts that are conflicting over the shared resource.
	members *advancedset.AdvancedSet[*Conflict[ConflictID, ResourceID, VoteRank]]

	allMembersEvicted *promise.Value[bool]

	mutex syncutils.RWMutex
}

// NewConflictSet creates a new ConflictSet of Conflicts that are conflicting with each other over the given Resource.
func NewConflictSet[ConflictID, ResourceID conflictdag.IDType, VoteRank conflictdag.VoteRankType[VoteRank]](id ResourceID) *ConflictSet[ConflictID, ResourceID, VoteRank] {
	return &ConflictSet[ConflictID, ResourceID, VoteRank]{
		ID:                id,
		allMembersEvicted: promise.NewValue[bool](),
		members:           advancedset.New[*Conflict[ConflictID, ResourceID, VoteRank]](),
	}
}

// Add adds a Conflict to the ConflictSet and returns all other members of the set.
func (c *ConflictSet[ConflictID, ResourceID, VoteRank]) Add(addedConflict *Conflict[ConflictID, ResourceID, VoteRank]) (otherMembers *advancedset.AdvancedSet[*Conflict[ConflictID, ResourceID, VoteRank]], err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.allMembersEvicted.Get() {
		return nil, xerrors.Errorf("cannot join a ConflictSet whose all members are evicted")
	}

	if otherMembers = c.members.Clone(); !c.members.Add(addedConflict) {
		return nil, conflictdag.ErrAlreadyPartOfConflictSet
	}

	return otherMembers, nil

}

// Remove removes a Conflict from the ConflictSet and returns all remaining members of the set.
func (c *ConflictSet[ConflictID, ResourceID, VoteRank]) Remove(removedConflict *Conflict[ConflictID, ResourceID, VoteRank]) (removed bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if removed = c.members.Delete(removedConflict); removed && c.members.IsEmpty() {
		c.allMembersEvicted.Set(true)
	}

	return removed
}

func (c *ConflictSet[ConflictID, ResourceID, VoteRank]) ForEach(callback func(parent *Conflict[ConflictID, ResourceID, VoteRank]) error) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.members.ForEach(callback)
}

// OnAllMembersEvicted executes a callback when all members of the ConflictSet are evicted and the ConflictSet itself can be evicted.
func (c *ConflictSet[ConflictID, ResourceID, VoteRank]) OnAllMembersEvicted(callback func(prevValue, newValue bool)) {
	c.allMembersEvicted.OnUpdate(callback)
}
