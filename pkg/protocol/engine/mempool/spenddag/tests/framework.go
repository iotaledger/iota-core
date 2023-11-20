package tests

import (
	"testing"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Framework is a test framework for the SpendDAG that allows to easily create and manipulate the DAG and its
// validators using human-readable aliases instead of actual IDs.
type Framework struct {
	// Instance is the SpendDAG instance that is used in the tests.
	Instance spenddag.SpendDAG[iotago.TransactionID, iotago.OutputID, vote.MockedRank]

	// Accounts is the AccountsTestFramework that is used in the tests.
	Accounts *AccountsTestFramework

	// Assert provides a set of assertions that can be used to verify the state of the SpendDAG.
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
	spendDAG spenddag.SpendDAG[iotago.TransactionID, iotago.OutputID, vote.MockedRank],
	validators *AccountsTestFramework,
	spendID func(string) iotago.TransactionID,
	resourceID func(string) iotago.OutputID,
) *Framework {
	f := &Framework{
		Instance:   spendDAG,
		Accounts:   validators,
		SpendID:    spendID,
		ResourceID: resourceID,
		test:       t,
	}
	f.Assert = &Assertions{f}

	return f
}

// CreateOrUpdateSpend creates a new spend or adds it to the given SpendSets.
func (f *Framework) CreateOrUpdateSpend(alias string, resourceAliases []string) error {
	f.Instance.CreateSpend(f.SpendID(alias))
	return f.Instance.UpdateConflictingResources(f.SpendID(alias), f.SpendSetIDs(resourceAliases...))

}

// UpdateConflictParents updates the parents of the spend with the given alias.
func (f *Framework) UpdateSpendParents(spendAlias string, addedParentIDs []string, removedParentIDs []string) error {
	return f.Instance.UpdateSpendParents(f.SpendID(spendAlias), f.SpendIDs(addedParentIDs...), f.SpendIDs(removedParentIDs...))
}

// LikedInstead returns the set of spends that are liked instead of the given spends.
func (f *Framework) LikedInstead(spendAliases ...string) ds.Set[iotago.TransactionID] {
	var result ds.Set[iotago.TransactionID]
	_ = f.Instance.ReadConsistent(func(spendDAG spenddag.ReadLockedSpendDAG[iotago.TransactionID, iotago.OutputID, vote.MockedRank]) error {
		result = spendDAG.LikedInstead(f.SpendIDs(spendAliases...))

		return nil
	})

	return result
}

// CastVotes casts the given votes for the given spends.
func (f *Framework) CastVotes(nodeAlias string, voteRank int, spendAliases ...string) error {
	seat, exists := f.Accounts.Get(nodeAlias)
	if !exists {
		return ierrors.Errorf("node with alias '%s' does not have a seat in the committee", nodeAlias)
	}

	return f.Instance.CastVotes(vote.NewVote[vote.MockedRank](seat, vote.MockedRank(voteRank)), f.SpendIDs(spendAliases...))
}

// EvictSpend evicts given spend from the SpendDAG.
func (f *Framework) EvictSpend(spendAlias string) {
	f.Instance.EvictSpend(f.SpendID(spendAlias))
}

// SpendIDs translates the given aliases into an AdvancedSet of iotago.TransactionIDs.
func (f *Framework) SpendIDs(aliases ...string) ds.Set[iotago.TransactionID] {
	spendIDs := ds.NewSet[iotago.TransactionID]()
	for _, alias := range aliases {
		spendIDs.Add(f.SpendID(alias))
	}

	return spendIDs
}

// SpendSetIDs translates the given aliases into an AdvancedSet of iotago.OutputIDs.
func (f *Framework) SpendSetIDs(aliases ...string) ds.Set[iotago.OutputID] {
	spendSetIDs := ds.NewSet[iotago.OutputID]()
	for _, alias := range aliases {
		spendSetIDs.Add(f.ResourceID(alias))
	}

	return spendSetIDs
}
