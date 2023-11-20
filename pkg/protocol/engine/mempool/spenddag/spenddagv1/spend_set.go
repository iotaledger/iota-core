package spenddagv1

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
)

// SpendSet represents a set of Spends of a Resource.
// If there's more than 1 Spend in a SpendSet, they are conflicting with each other over the shared Resource.
type SpendSet[SpendID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]] struct {
	// ID is the ID of the Resource that the Spends in this SpendSet are conflicting over.
	ID ResourceID

	// members is the set of Spends that are conflicting over the shared resource.
	members ds.Set[*Spend[SpendID, ResourceID, VoteRank]]

	allMembersEvicted reactive.Variable[bool]

	mutex syncutils.RWMutex
}

// NewSpendSet creates a new SpendSet of Spends that are conflicting with each other over the given Resource.
func NewSpendSet[SpendID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]](id ResourceID) *SpendSet[SpendID, ResourceID, VoteRank] {
	return &SpendSet[SpendID, ResourceID, VoteRank]{
		ID:                id,
		allMembersEvicted: reactive.NewVariable[bool](),
		members:           ds.NewSet[*Spend[SpendID, ResourceID, VoteRank]](),
	}
}

// Add adds a Spend to the SpendSet and returns all other members of the set.
func (c *SpendSet[SpendID, ResourceID, VoteRank]) Add(addedConflict *Spend[SpendID, ResourceID, VoteRank]) (otherMembers ds.Set[*Spend[SpendID, ResourceID, VoteRank]], err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.allMembersEvicted.Get() {
		return nil, ierrors.New("cannot join a SpendSet whose all members are evicted")
	}

	if otherMembers = c.members.Clone(); !c.members.Add(addedConflict) {
		return nil, spenddag.ErrAlreadyPartOfSpendSet
	}

	return otherMembers, nil

}

// Remove removes a Spend from the SpendSet and returns all remaining members of the set.
func (c *SpendSet[SpendID, ResourceID, VoteRank]) Remove(removedConflict *Spend[SpendID, ResourceID, VoteRank]) (removed bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if removed = c.members.Delete(removedConflict); removed && c.members.IsEmpty() {
		c.allMembersEvicted.Set(true)
	}

	return removed
}

func (c *SpendSet[SpendID, ResourceID, VoteRank]) ForEach(callback func(parent *Spend[SpendID, ResourceID, VoteRank]) error) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.members.ForEach(callback)
}

// OnAllMembersEvicted executes a callback when all members of the SpendSet are evicted and the SpendSet itself can be evicted.
func (c *SpendSet[SpendID, ResourceID, VoteRank]) OnAllMembersEvicted(callback func(prevValue, newValue bool)) {
	c.allMembersEvicted.OnUpdate(callback)
}
