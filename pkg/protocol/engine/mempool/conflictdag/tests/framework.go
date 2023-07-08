package tests

import (
	"testing"

	"github.com/iotaledger/hive.go/ds/set"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Framework is a test framework for the ConflictDAG that allows to easily create and manipulate the DAG and its
// validators using human-readable aliases instead of actual IDs.
type Framework struct {
	// Instance is the ConflictDAG instance that is used in the tests.
	Instance conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, vote.MockedRank]

	// Accounts is the AccountsTestFramework that is used in the tests.
	Accounts *AccountsTestFramework

	// Assert provides a set of assertions that can be used to verify the state of the ConflictDAG.
	Assert *Assertions

	// ConflictID is a function that is used to translate a string alias into a (deterministic) iotago.TransactionID.
	ConflictID func(string) iotago.TransactionID

	// ResourceID is a function that is used to translate a string alias into a (deterministic) iotago.OutputID.
	ResourceID func(string) iotago.OutputID

	// test is the *testing.T instance that is used in the tests.
	test *testing.T
}

// NewFramework creates a new instance of the Framework.
func NewFramework(
	t *testing.T,
	conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, vote.MockedRank],
	validators *AccountsTestFramework,
	conflictID func(string) iotago.TransactionID,
	resourceID func(string) iotago.OutputID,
) *Framework {
	f := &Framework{
		Instance:   conflictDAG,
		Accounts:   validators,
		ConflictID: conflictID,
		ResourceID: resourceID,
		test:       t,
	}
	f.Assert = &Assertions{f}

	return f
}

// CreateOrUpdateConflict creates a new conflict or adds it to the given ConflictSets.
func (f *Framework) CreateOrUpdateConflict(alias string, resourceAliases []string) error {
	f.Instance.CreateConflict(f.ConflictID(alias))
	return f.Instance.UpdateConflictingResources(f.ConflictID(alias), f.ConflictSetIDs(resourceAliases...))

}

// UpdateConflictParents updates the parents of the conflict with the given alias.
func (f *Framework) UpdateConflictParents(conflictAlias string, addedParentIDs, removedParentIDs []string) error {
	return f.Instance.UpdateConflictParents(f.ConflictID(conflictAlias), f.ConflictIDs(addedParentIDs...), f.ConflictIDs(removedParentIDs...))
}

// LikedInstead returns the set of conflicts that are liked instead of the given conflicts.
func (f *Framework) LikedInstead(conflictAliases ...string) set.Set[iotago.TransactionID] {
	var result set.Set[iotago.TransactionID]
	_ = f.Instance.ReadConsistent(func(conflictDAG conflictdag.ReadLockedConflictDAG[iotago.TransactionID, iotago.OutputID, vote.MockedRank]) error {
		result = conflictDAG.LikedInstead(f.ConflictIDs(conflictAliases...))

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

	return f.Instance.CastVotes(vote.NewVote[vote.MockedRank](seat, vote.MockedRank(voteRank)), f.ConflictIDs(conflictAliases...))
}

// EvictConflict evicts given conflict from the ConflictDAG.
func (f *Framework) EvictConflict(conflictAlias string) {
	f.Instance.EvictConflict(f.ConflictID(conflictAlias))
}

// ConflictIDs translates the given aliases into an AdvancedSet of iotago.TransactionIDs.
func (f *Framework) ConflictIDs(aliases ...string) set.Set[iotago.TransactionID] {
	conflictIDs := set.New[iotago.TransactionID]()
	for _, alias := range aliases {
		conflictIDs.Add(f.ConflictID(alias))
	}

	return conflictIDs
}

// ConflictSetIDs translates the given aliases into an AdvancedSet of iotago.OutputIDs.
func (f *Framework) ConflictSetIDs(aliases ...string) set.Set[iotago.OutputID] {
	conflictSetIDs := set.New[iotago.OutputID]()
	for _, alias := range aliases {
		conflictSetIDs.Add(f.ResourceID(alias))
	}

	return conflictSetIDs
}
