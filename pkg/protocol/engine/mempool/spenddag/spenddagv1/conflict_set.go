package spenddagv1

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
)

// ConflictSet represents a set of Spends that are conflicting with each other over a common Resource.
type ConflictSet[SpendID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]] struct {
	// ID is the ID of the Resource that the Spends in this ConflictSet are conflicting over.
	ID ResourceID

	// members is the set of Spends that are conflicting over the shared resource.
	members ds.Set[*Spend[SpendID, ResourceID, VoteRank]]

	allMembersEvicted reactive.Variable[bool]

	mutex syncutils.RWMutex
}

// NewConflictSet creates a new ConflictSet of Spends that are conflicting with each other over the given Resource.
func NewConflictSet[SpendID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]](id ResourceID) *ConflictSet[SpendID, ResourceID, VoteRank] {
	return &ConflictSet[SpendID, ResourceID, VoteRank]{
		ID:                id,
		allMembersEvicted: reactive.NewVariable[bool](),
		members:           ds.NewSet[*Spend[SpendID, ResourceID, VoteRank]](),
	}
}

// Add adds a Spend to the ConflictSet and returns all other members of the set.
func (c *ConflictSet[SpendID, ResourceID, VoteRank]) Add(addedConflict *Spend[SpendID, ResourceID, VoteRank]) (otherMembers ds.Set[*Spend[SpendID, ResourceID, VoteRank]], err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.allMembersEvicted.Get() {
		return nil, ierrors.New("cannot join a ConflictSet whose all members are evicted")
	}

	if otherMembers = c.members.Clone(); !c.members.Add(addedConflict) {
		return nil, spenddag.ErrAlreadyPartOfConflictSet
	}

	return otherMembers, nil

}

// Remove removes a Spend from the ConflictSet and returns all remaining members of the set.
func (c *ConflictSet[SpendID, ResourceID, VoteRank]) Remove(removedConflict *Spend[SpendID, ResourceID, VoteRank]) (removed bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if removed = c.members.Delete(removedConflict); removed && c.members.IsEmpty() {
		c.allMembersEvicted.Set(true)
	}

	return removed
}

func (c *ConflictSet[SpendID, ResourceID, VoteRank]) ForEach(callback func(parent *Spend[SpendID, ResourceID, VoteRank]) error) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.members.ForEach(callback)
}

// OnAllMembersEvicted executes a callback when all members of the ConflictSet are evicted and the ConflictSet itself can be evicted.
func (c *ConflictSet[SpendID, ResourceID, VoteRank]) OnAllMembersEvicted(callback func(prevValue, newValue bool)) {
	c.allMembersEvicted.OnUpdate(callback)
}
