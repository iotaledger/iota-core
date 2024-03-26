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

	// SpenderID is a function that is used to translate a string alias into a (deterministic) iotago.TransactionID.
	SpenderID func(string) iotago.TransactionID

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
	spenderID func(string) iotago.TransactionID,
	resourceID func(string) iotago.OutputID,
) *Framework {
	t.Helper()

	f := &Framework{
		Instance:   spendDAG,
		Accounts:   validators,
		SpenderID:  spenderID,
		ResourceID: resourceID,
		test:       t,
	}
	f.Assert = &Assertions{f}

	return f
}

// CreateOrUpdateSpender creates a new spender or adds it to the given SpendSets.
func (f *Framework) CreateOrUpdateSpender(alias string, resourceAliases []string) error {
	f.Instance.CreateSpender(f.SpenderID(alias))
	return f.Instance.UpdateSpentResources(f.SpenderID(alias), f.SpendSetIDs(resourceAliases...))
}

// UpdateSpenderParents updates the parents of the spender with the given alias.
func (f *Framework) UpdateSpenderParents(spendAlias string, addedParentIDs []string, removedParentIDs []string) error {
	return f.Instance.UpdateSpenderParents(f.SpenderID(spendAlias), f.SpenderIDs(addedParentIDs...), f.SpenderIDs(removedParentIDs...))
}

// LikedInstead returns the set of spenders that are liked instead of the given spenders.
func (f *Framework) LikedInstead(spendAliases ...string) ds.Set[iotago.TransactionID] {
	var result ds.Set[iotago.TransactionID]
	_ = f.Instance.ReadConsistent(func(spendDAG spenddag.ReadLockedSpendDAG[iotago.TransactionID, iotago.OutputID, vote.MockedRank]) error {
		result = spendDAG.LikedInstead(f.SpenderIDs(spendAliases...))

		return nil
	})

	return result
}

// CastVotes casts the given votes for the given spenders.
func (f *Framework) CastVotes(nodeAlias string, voteRank int, spenderAliases ...string) error {
	seat, exists := f.Accounts.Get(nodeAlias)
	if !exists {
		return ierrors.Errorf("node with alias '%s' does not have a seat in the committee", nodeAlias)
	}

	return f.Instance.CastVotes(vote.NewVote[vote.MockedRank](seat, vote.MockedRank(voteRank)), f.SpenderIDs(spenderAliases...))
}

// EvictSpender evicts given spender from the SpendDAG.
func (f *Framework) EvictSpender(spendAlias string) {
	f.Instance.EvictSpender(f.SpenderID(spendAlias))
}

// SpenderIDs translates the given aliases into an AdvancedSet of iotago.TransactionIDs.
func (f *Framework) SpenderIDs(aliases ...string) ds.Set[iotago.TransactionID] {
	spenderIDs := ds.NewSet[iotago.TransactionID]()
	for _, alias := range aliases {
		spenderIDs.Add(f.SpenderID(alias))
	}

	return spenderIDs
}

// SpendSetIDs translates the given aliases into an AdvancedSet of iotago.OutputIDs.
func (f *Framework) SpendSetIDs(aliases ...string) ds.Set[iotago.OutputID] {
	spendSetIDs := ds.NewSet[iotago.OutputID]()
	for _, alias := range aliases {
		spendSetIDs.Add(f.ResourceID(alias))
	}

	return spendSetIDs
}
