package tests

import (
	"testing"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Framework is a test framework for the spenddag that allows to easily create and manipulate the DAG and its
// validators using human-readable aliases instead of actual IDs.
type Framework struct {
	// Instance is the spenddag instance that is used in the tests.
	Instance spenddag.SpendDAG[iotago.TransactionID, iotago.OutputID, vote.MockedRank]

	// Accounts is the AccountsTestFramework that is used in the tests.
	Accounts *AccountsTestFramework

	// Assert provides a set of assertions that can be used to verify the state of the spenddag.
	Assert *Assertions

	// SpendID is a function that is used to translate a string alias into a (deterministic) iotago.TransactionID.
	SpendID func(string) iotago.TransactionID

	// ResourceID is a function that is used to translate a string alias into a (deterministic) iotago.OutputID.
	ResourceID func(string) iotago.OutputID

	// test is the *testing.T instance that is used in the tests.
	test *testing.T
}

// NewFramework creates a new instance of the Framework.
func NewFramework(
	t *testing.T,
	spenddag spenddag.SpendDAG[iotago.TransactionID, iotago.OutputID, vote.MockedRank],
	validators *AccountsTestFramework,
	spendID func(string) iotago.TransactionID,
	resourceID func(string) iotago.OutputID,
) *Framework {
	f := &Framework{
		Instance:   spenddag,
		Accounts:   validators,
		SpendID:    spendID,
		ResourceID: resourceID,
		test:       t,
	}
	f.Assert = &Assertions{f}

	return f
}

// CreateOrUpdateConflict creates a new conflict or adds it to the given ConflictSets.
func (f *Framework) CreateOrUpdateConflict(alias string, resourceAliases []string) error {
	f.Instance.CreateSpend(f.SpendID(alias))
	return f.Instance.UpdateConflictingResources(f.SpendID(alias), f.ConflictSetIDs(resourceAliases...))

}

// UpdateConflictParents updates the parents of the conflict with the given alias.
func (f *Framework) UpdateSpendParents(conflictAlias string, addedParentIDs []string, removedParentIDs []string) error {
	return f.Instance.UpdateSpendParents(f.SpendID(conflictAlias), f.SpendIDs(addedParentIDs...), f.SpendIDs(removedParentIDs...))
}

// LikedInstead returns the set of conflicts that are liked instead of the given conflicts.
func (f *Framework) LikedInstead(conflictAliases ...string) ds.Set[iotago.TransactionID] {
	var result ds.Set[iotago.TransactionID]
	_ = f.Instance.ReadConsistent(func(spenddag spenddag.ReadLockedSpendDAG[iotago.TransactionID, iotago.OutputID, vote.MockedRank]) error {
		result = spenddag.LikedInstead(f.SpendIDs(conflictAliases...))

		return nil
	})

	return result
}

// CastVotes casts the given votes for the given conflicts.
func (f *Framework) CastVotes(nodeAlias string, voteRank int, conflictAliases ...string) error {
	seat, exists := f.Accounts.Get(nodeAlias)
	if !exists {
		return ierrors.Errorf("node with alias '%s' does not have a seat in the committee", nodeAlias)
	}

	return f.Instance.CastVotes(vote.NewVote[vote.MockedRank](seat, vote.MockedRank(voteRank)), f.SpendIDs(conflictAliases...))
}

// EvictConflict evicts given conflict from the spenddag.
func (f *Framework) EvictSpend(conflictAlias string) {
	f.Instance.EvictSpend(f.SpendID(conflictAlias))
}

// SpendIDs translates the given aliases into an AdvancedSet of iotago.TransactionIDs.
func (f *Framework) SpendIDs(aliases ...string) ds.Set[iotago.TransactionID] {
	spendIDs := ds.NewSet[iotago.TransactionID]()
	for _, alias := range aliases {
		spendIDs.Add(f.SpendID(alias))
	}

	return spendIDs
}

// ConflictSetIDs translates the given aliases into an AdvancedSet of iotago.OutputIDs.
func (f *Framework) ConflictSetIDs(aliases ...string) ds.Set[iotago.OutputID] {
	conflictSetIDs := ds.NewSet[iotago.OutputID]()
	for _, alias := range aliases {
		conflictSetIDs.Add(f.ResourceID(alias))
	}

	return conflictSetIDs
}
