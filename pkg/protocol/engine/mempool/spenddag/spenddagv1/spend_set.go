package spenddagv1

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
)

// SpendSet represents a set of Spenders of a Resource.
// If there's more than 1 spender in a SpendSet, they are conflicting with each other over the shared Resource.
type SpendSet[SpenderID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]] struct {
	// ID is the ID of the Resource being spent.
	ID ResourceID

	// spenders is the set of spenders (e.g. transactions) that spend the resource (e.g. a utxo).
	spenders ds.Set[*Spender[SpenderID, ResourceID, VoteRank]]

	allMembersEvicted reactive.Variable[bool]

	mutex syncutils.RWMutex
}

// NewSpendSet creates a new SpendSet containing spenders (e.g. a transaction) of a common resource (e.g. a utxo).
func NewSpendSet[SpenderID, ResourceID spenddag.IDType, VoteRank spenddag.VoteRankType[VoteRank]](id ResourceID) *SpendSet[SpenderID, ResourceID, VoteRank] {
	return &SpendSet[SpenderID, ResourceID, VoteRank]{
		ID:                id,
		allMembersEvicted: reactive.NewVariable[bool](),
		spenders:          ds.NewSet[*Spender[SpenderID, ResourceID, VoteRank]](),
	}
}

// Add adds a Spender to the SpendSet and returns all other members of the set.
func (c *SpendSet[SpenderID, ResourceID, VoteRank]) Add(addedSpender *Spender[SpenderID, ResourceID, VoteRank]) (otherMembers ds.Set[*Spender[SpenderID, ResourceID, VoteRank]], err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.allMembersEvicted.Get() {
		return nil, ierrors.New("cannot join a SpendSet whose all members are evicted")
	}

	if otherMembers = c.spenders.Clone(); !c.spenders.Add(addedSpender) {
		return nil, spenddag.ErrAlreadyPartOfSpendSet
	}

	return otherMembers, nil
}

// Remove removes a Spender from the SpendSet and returns all remaining members of the set.
func (c *SpendSet[SpenderID, ResourceID, VoteRank]) Remove(removedSpender *Spender[SpenderID, ResourceID, VoteRank]) (removed bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if removed = c.spenders.Delete(removedSpender); removed && c.spenders.IsEmpty() {
		c.allMembersEvicted.Set(true)
	}

	return removed
}

func (c *SpendSet[SpenderID, ResourceID, VoteRank]) ForEach(callback func(parent *Spender[SpenderID, ResourceID, VoteRank]) error) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.spenders.ForEach(callback)
}

// OnAllMembersEvicted executes a callback when all members of the SpendSet are evicted and the SpendSet itself can be evicted.
func (c *SpendSet[SpenderID, ResourceID, VoteRank]) OnAllMembersEvicted(callback func(prevValue, newValue bool)) {
	c.allMembersEvicted.OnUpdate(callback)
}
