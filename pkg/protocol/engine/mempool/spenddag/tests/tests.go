package tests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestAll(t *testing.T, frameworkProvider func(*testing.T) *Framework) {
	for testName, testCase := range map[string]func(*testing.T, *Framework){
		"CreateSpend":                 CreateSpend,
		"ExistingSpendJoinsSpendSets": ExistingSpendJoinsSpendSets,
		"JoinSpendSetTwice":           JoinSpendSetTwice,
		"UpdateSpendParents":          UpdateSpendParents,
		"LikedInstead":                LikedInstead,
		"CreateSpendWithoutMembers":   CreateSpendWithoutMembers,
		"SpendAcceptance":             SpendAcceptance,
		"CastVotes":                   CastVotes,
		"CastVotes_VoteRank":          CastVotesVoteRank,
		"CastVotesAcceptance":         CastVotesAcceptance,
		"EvictAcceptedSpend":          EvictAcceptedSpend,
		"EvictRejectedSpend":          EvictRejectedSpend,
	} {
		t.Run(testName, func(t *testing.T) { testCase(t, frameworkProvider(t)) })
	}
}

func ExistingSpendJoinsSpendSets(t *testing.T, tf *Framework) {
	require.NoError(t, tf.CreateOrUpdateSpend("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateSpend("conflict2", []string{"resource1"}))
	tf.Assert.SpendSetMembers("resource1", "conflict1", "conflict2")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict3", []string{"resource2"}))
	require.NoError(t, tf.CreateOrUpdateSpend("conflict1", []string{"resource2"}))

	tf.Assert.SpendSetMembers("resource2", "conflict1", "conflict3")
	tf.Assert.SpendSetMembers("resource1", "conflict1", "conflict2")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict2", []string{"resource2"}))

	tf.Assert.SpendSetMembers("resource2", "conflict1", "conflict2", "conflict3")
	tf.Assert.SpendSetMembers("resource1", "conflict1", "conflict2")

	tf.Assert.LikedInstead([]string{"conflict3"}, "conflict1")
}

func UpdateSpendParents(t *testing.T, tf *Framework) {
	require.NoError(t, tf.CreateOrUpdateSpend("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateSpend("conflict2", []string{"resource2"}))

	require.NoError(t, tf.CreateOrUpdateSpend("conflict3", []string{"resource1", "resource2"}))
	require.NoError(t, tf.UpdateSpendParents("conflict3", []string{"conflict1", "conflict2"}, []string{}))
	tf.Assert.Children("conflict1", "conflict3")
	tf.Assert.Parents("conflict3", "conflict1", "conflict2")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict2.5", []string{"conflict2.5"}))
	require.NoError(t, tf.UpdateSpendParents("conflict2.5", []string{"conflict1", "conflict2"}, []string{}))
	tf.Assert.Children("conflict1", "conflict2.5", "conflict3")
	tf.Assert.Children("conflict2", "conflict2.5", "conflict3")
	tf.Assert.Parents("conflict2.5", "conflict1", "conflict2")

	require.NoError(t, tf.UpdateSpendParents("conflict3", []string{"conflict2.5"}, []string{"conflict1", "conflict2"}))

	tf.Assert.Children("conflict1", "conflict2.5")
	tf.Assert.Children("conflict2", "conflict2.5")
	tf.Assert.Children("conflict2.5", "conflict3")
	tf.Assert.Parents("conflict3", "conflict2.5")
	tf.Assert.Parents("conflict2.5", "conflict1", "conflict2")
}

func CreateSpend(t *testing.T, tf *Framework) {
	require.NoError(t, tf.CreateOrUpdateSpend("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateSpend("conflict2", []string{"resource1"}))
	tf.Assert.SpendSetMembers("resource1", "conflict1", "conflict2")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict3", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpendParents("conflict3", []string{"conflict1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpend("conflict4", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpendParents("conflict4", []string{"conflict1"}, []string{}))

	tf.Assert.SpendSetMembers("resource2", "conflict3", "conflict4")
	tf.Assert.Children("conflict1", "conflict3", "conflict4")
	tf.Assert.Parents("conflict3", "conflict1")
	tf.Assert.Parents("conflict4", "conflict1")
}

func CreateSpendWithoutMembers(t *testing.T, tf *Framework) {
	tf.Accounts.CreateID("nodeID1")
	tf.Accounts.CreateID("nodeID2")
	tf.Accounts.CreateID("nodeID3")
	tf.Accounts.CreateID("nodeID4")

	// Non-conflicting conflicts
	{
		require.NoError(t, tf.CreateOrUpdateSpend("conflict1", []string{"resource1"}))
		require.NoError(t, tf.CreateOrUpdateSpend("conflict2", []string{"resource2"}))

		tf.Assert.SpendSetMembers("resource1", "conflict1")
		tf.Assert.SpendSetMembers("resource2", "conflict2")

		tf.Assert.LikedInstead([]string{"conflict1"})
		tf.Assert.LikedInstead([]string{"conflict2"})

		require.NoError(t, tf.CastVotes("nodeID1", 1, "conflict1"))
		require.NoError(t, tf.CastVotes("nodeID2", 1, "conflict1"))
		require.NoError(t, tf.CastVotes("nodeID3", 1, "conflict1"))

		tf.Assert.LikedInstead([]string{"conflict1"})
		tf.Assert.Accepted("conflict1")
	}

	// Regular conflict
	{
		require.NoError(t, tf.CreateOrUpdateSpend("conflict3", []string{"resource3"}))
		require.NoError(t, tf.CreateOrUpdateSpend("conflict4", []string{"resource3"}))

		tf.Assert.SpendSetMembers("resource3", "conflict3", "conflict4")

		require.NoError(t, tf.CastVotes("nodeID3", 1, "conflict3"))

		tf.Assert.LikedInstead([]string{"conflict3"})
		tf.Assert.LikedInstead([]string{"conflict4"}, "conflict3")
	}

	tf.Assert.LikedInstead([]string{"conflict1", "conflict4"}, "conflict3")
}

func LikedInstead(t *testing.T, tf *Framework) {
	tf.Accounts.CreateID("zero-weight")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CastVotes("zero-weight", 1, "conflict1"))
	require.NoError(t, tf.CreateOrUpdateSpend("conflict2", []string{"resource1"}))
	tf.Assert.SpendSetMembers("resource1", "conflict1", "conflict2")
	tf.Assert.LikedInstead([]string{"conflict1", "conflict2"}, "conflict1")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict3", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpendParents("conflict3", []string{"conflict1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpend("conflict4", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpendParents("conflict4", []string{"conflict1"}, []string{}))

	require.NoError(t, tf.CastVotes("zero-weight", 1, "conflict4"))
	tf.Assert.LikedInstead([]string{"conflict1", "conflict2", "conflict3", "conflict4"}, "conflict1", "conflict4")
}

func SpendAcceptance(t *testing.T, tf *Framework) {
	tf.Accounts.CreateID("nodeID1")
	tf.Accounts.CreateID("nodeID2")
	tf.Accounts.CreateID("nodeID3")
	tf.Accounts.CreateID("nodeID4")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateSpend("conflict2", []string{"resource1"}))
	tf.Assert.SpendSetMembers("resource1", "conflict1", "conflict2")
	tf.Assert.SpendSets("conflict1", "resource1")
	tf.Assert.SpendSets("conflict2", "resource1")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict3", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpendParents("conflict3", []string{"conflict1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpend("conflict4", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpendParents("conflict4", []string{"conflict1"}, []string{}))

	tf.Assert.SpendSetMembers("resource2", "conflict3", "conflict4")
	tf.Assert.Children("conflict1", "conflict3", "conflict4")
	tf.Assert.Parents("conflict3", "conflict1")
	tf.Assert.Parents("conflict4", "conflict1")

	require.NoError(t, tf.CastVotes("nodeID1", 1, "conflict4"))
	require.NoError(t, tf.CastVotes("nodeID2", 1, "conflict4"))
	require.NoError(t, tf.CastVotes("nodeID3", 1, "conflict4"))

	tf.Assert.LikedInstead([]string{"conflict1"})
	tf.Assert.LikedInstead([]string{"conflict2"}, "conflict1")
	tf.Assert.LikedInstead([]string{"conflict3"}, "conflict4")
	tf.Assert.LikedInstead([]string{"conflict4"})

	tf.Assert.Accepted("conflict1", "conflict4")
}

func CastVotes(t *testing.T, tf *Framework) {
	tf.Accounts.CreateID("nodeID1")
	tf.Accounts.CreateID("nodeID2")
	tf.Accounts.CreateID("nodeID3")
	tf.Accounts.CreateID("nodeID4")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateSpend("conflict2", []string{"resource1"}))
	tf.Assert.SpendSetMembers("resource1", "conflict1", "conflict2")
	tf.Assert.SpendSets("conflict1", "resource1")
	tf.Assert.SpendSets("conflict2", "resource1")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict3", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpendParents("conflict3", []string{"conflict1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpend("conflict4", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpendParents("conflict4", []string{"conflict1"}, []string{}))

	tf.Assert.SpendSetMembers("resource2", "conflict3", "conflict4")
	tf.Assert.Children("conflict1", "conflict3", "conflict4")
	tf.Assert.Parents("conflict3", "conflict1")
	tf.Assert.Parents("conflict4", "conflict1")

	require.NoError(t, tf.CastVotes("nodeID1", 1, "conflict2"))
	require.NoError(t, tf.CastVotes("nodeID2", 1, "conflict2"))
	require.NoError(t, tf.CastVotes("nodeID3", 1, "conflict2"))
	tf.Assert.LikedInstead([]string{"conflict1"}, "conflict2")

	tf.Assert.Accepted("conflict2")
	tf.Assert.Rejected("conflict1")
	tf.Assert.Rejected("conflict3")
	tf.Assert.Rejected("conflict4")

	require.Error(t, tf.CastVotes("nodeID3", 1, "conflict1", "conflict2"))
}

func CastVotesVoteRank(t *testing.T, tf *Framework) {
	tf.Accounts.CreateID("nodeID1")
	tf.Accounts.CreateID("nodeID2")
	tf.Accounts.CreateID("nodeID3")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateSpend("conflict2", []string{"resource1"}))
	tf.Assert.SpendSetMembers("resource1", "conflict1", "conflict2")
	tf.Assert.SpendSets("conflict1", "resource1")
	tf.Assert.SpendSets("conflict2", "resource1")

	// create nested conflicts
	require.NoError(t, tf.CreateOrUpdateSpend("conflict3", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpendParents("conflict3", []string{"conflict1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpend("conflict4", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpendParents("conflict4", []string{"conflict1"}, []string{}))

	tf.Assert.SpendSetMembers("resource2", "conflict3", "conflict4")
	tf.Assert.Children("conflict1", "conflict3", "conflict4")
	tf.Assert.Parents("conflict3", "conflict1")
	tf.Assert.Parents("conflict4", "conflict1")

	// casting a vote from a validator updates the validator weight
	require.NoError(t, tf.CastVotes("nodeID1", 2, "conflict4"))
	tf.Assert.LikedInstead([]string{"conflict1"})
	tf.Assert.LikedInstead([]string{"conflict2"}, "conflict1")
	tf.Assert.LikedInstead([]string{"conflict3"}, "conflict4")
	tf.Assert.LikedInstead([]string{"conflict4"})

	// casting vote with lower vote power doesn't change the weights of conflicts
	require.NoError(t, tf.CastVotes("nodeID1", 1), "conflict3")
	tf.Assert.LikedInstead([]string{"conflict1"})
	tf.Assert.LikedInstead([]string{"conflict2"}, "conflict1")
	tf.Assert.LikedInstead([]string{"conflict3"}, "conflict4")
	tf.Assert.LikedInstead([]string{"conflict4"})
	tf.Assert.ValidatorWeight("conflict1", 1)
	tf.Assert.ValidatorWeight("conflict2", 0)
	tf.Assert.ValidatorWeight("conflict3", 0)
	tf.Assert.ValidatorWeight("conflict4", 1)

	// casting vote with higher vote power changes the weights of conflicts
	require.NoError(t, tf.CastVotes("nodeID1", 3, "conflict3"))
	tf.Assert.LikedInstead([]string{"conflict1"})
	tf.Assert.LikedInstead([]string{"conflict2"}, "conflict1")
	tf.Assert.LikedInstead([]string{"conflict3"})
	tf.Assert.LikedInstead([]string{"conflict4"}, "conflict3")
	tf.Assert.ValidatorWeight("conflict1", 1)
	tf.Assert.ValidatorWeight("conflict2", 0)
	tf.Assert.ValidatorWeight("conflict3", 1)
	tf.Assert.ValidatorWeight("conflict4", 0)
}

func CastVotesAcceptance(t *testing.T, tf *Framework) {
	tf.Accounts.CreateID("nodeID1")
	tf.Accounts.CreateID("nodeID2")
	tf.Accounts.CreateID("nodeID3")
	tf.Accounts.CreateID("nodeID4")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateSpend("conflict2", []string{"resource1"}))
	tf.Assert.SpendSetMembers("resource1", "conflict1", "conflict2")
	tf.Assert.SpendSets("conflict1", "resource1")
	tf.Assert.SpendSets("conflict2", "resource1")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict3", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpendParents("conflict3", []string{"conflict1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpend("conflict4", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpendParents("conflict4", []string{"conflict1"}, []string{}))

	tf.Assert.SpendSetMembers("resource2", "conflict3", "conflict4")
	tf.Assert.Children("conflict1", "conflict3", "conflict4")
	tf.Assert.Parents("conflict3", "conflict1")
	tf.Assert.Parents("conflict4", "conflict1")

	require.NoError(t, tf.CastVotes("nodeID1", 1, "conflict3"))
	require.NoError(t, tf.CastVotes("nodeID2", 1, "conflict3"))
	require.NoError(t, tf.CastVotes("nodeID3", 1, "conflict3"))
	tf.Assert.LikedInstead([]string{"conflict1"})
	tf.Assert.Accepted("conflict1")
	tf.Assert.Rejected("conflict2")
	tf.Assert.Accepted("conflict3")
	tf.Assert.Rejected("conflict4")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict5", []string{"resource1"}))
	tf.Assert.Rejected("conflict5")

	// Evict conflict and try to add non-existing parent to a rejected conflict - update is ignored because the parent is evicted.
	tf.EvictSpend("conflict2")
	require.NoError(t, tf.UpdateSpendParents("conflict4", []string{"conflict2"}, []string{}))
	parents, exists := tf.Instance.SpendParents(tf.SpendID("conflict4"))
	require.True(t, exists)
	require.False(t, parents.Has(tf.SpendID("conflict2")))

	// Try to update parents of evicted conflict.
	require.ErrorIs(t, tf.UpdateSpendParents("conflict2", []string{"conflict1"}, []string{}), spenddag.ErrEntityEvicted)
}

func JoinSpendSetTwice(t *testing.T, tf *Framework) {
	var conflictCreatedEventCount, resourceAddedEventCount int
	tf.Instance.Events().SpendCreated.Hook(func(id iotago.TransactionID) {
		conflictCreatedEventCount++
	})

	tf.Instance.Events().ConflictingResourcesAdded.Hook(func(id iotago.TransactionID, resourceID ds.Set[iotago.OutputID]) {
		fmt.Println("conflict joins spendset", id, resourceID)
		resourceAddedEventCount++
	})

	require.NoError(t, tf.CreateOrUpdateSpend("conflict1", []string{"resource1"}))
	require.Equal(t, 1, conflictCreatedEventCount)
	require.Equal(t, 1, resourceAddedEventCount)
	tf.Assert.SpendSets("conflict1", "resource1")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict1", []string{"resource2"}))
	require.Equal(t, 1, conflictCreatedEventCount)
	require.Equal(t, 2, resourceAddedEventCount)
	tf.Assert.SpendSets("conflict1", "resource1", "resource2")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict1", []string{"resource1", "resource2"}))
	require.Equal(t, 1, conflictCreatedEventCount)
	require.Equal(t, 2, resourceAddedEventCount)
	tf.Assert.SpendSets("conflict1", "resource1", "resource2")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict1", []string{"resource1", "resource2", "resource3", "resource4"}))
	require.Equal(t, 1, conflictCreatedEventCount)
	require.Equal(t, 3, resourceAddedEventCount)
	tf.Assert.SpendSets("conflict1", "resource1", "resource2", "resource3", "resource4")
}

func EvictAcceptedSpend(t *testing.T, tf *Framework) {
	tf.Accounts.CreateID("nodeID1")
	tf.Accounts.CreateID("nodeID2")
	tf.Accounts.CreateID("nodeID3")
	tf.Accounts.CreateID("nodeID4")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateSpend("conflict2", []string{"resource1"}))
	tf.Assert.SpendSetMembers("resource1", "conflict1", "conflict2")
	tf.Assert.SpendSets("conflict1", "resource1")
	tf.Assert.SpendSets("conflict2", "resource1")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict3", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpendParents("conflict3", []string{"conflict1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpend("conflict4", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpendParents("conflict4", []string{"conflict1"}, []string{}))

	tf.Assert.SpendSetMembers("resource2", "conflict3", "conflict4")
	tf.Assert.Children("conflict1", "conflict3", "conflict4")
	tf.Assert.Parents("conflict3", "conflict1")
	tf.Assert.Parents("conflict4", "conflict1")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict5", []string{"resource3"}))
	require.NoError(t, tf.UpdateSpendParents("conflict5", []string{"conflict2"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpend("conflict6", []string{"resource3"}))
	require.NoError(t, tf.UpdateSpendParents("conflict6", []string{"conflict2"}, []string{}))

	tf.Assert.SpendSetMembers("resource3", "conflict5", "conflict6")
	tf.Assert.Children("conflict2", "conflict5", "conflict6")
	tf.Assert.Parents("conflict5", "conflict2")
	tf.Assert.Parents("conflict6", "conflict2")

	require.NoError(t, tf.CastVotes("nodeID1", 1, "conflict2"))
	require.NoError(t, tf.CastVotes("nodeID2", 1, "conflict2"))
	require.NoError(t, tf.CastVotes("nodeID3", 1, "conflict2"))
	tf.Assert.LikedInstead([]string{"conflict1"}, "conflict2")

	tf.Assert.Accepted("conflict2")
	tf.Assert.Rejected("conflict1")
	tf.Assert.Rejected("conflict3")
	tf.Assert.Rejected("conflict4")
	tf.Assert.Pending("conflict5", "conflict6")

	tf.EvictSpend("conflict2")
	require.False(t, lo.Return2(tf.Instance.ConflictingSpends(tf.SpendID("conflict1"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingSpends(tf.SpendID("conflict2"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingSpends(tf.SpendID("conflict3"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingSpends(tf.SpendID("conflict4"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingSpends(tf.SpendID("conflict5"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingSpends(tf.SpendID("conflict6"))))

	require.False(t, lo.Return2(tf.Instance.SpendSetMembers(tf.ResourceID("resource1"))))
	require.False(t, lo.Return2(tf.Instance.SpendSetMembers(tf.ResourceID("resource2"))))
	tf.Assert.SpendSetMembers("resource3", "conflict5", "conflict6")

	tf.Assert.Parents("conflict5")
	tf.Assert.Parents("conflict6")
}

func EvictRejectedSpend(t *testing.T, tf *Framework) {
	conflictEvictedEventCount := 0
	tf.Instance.Events().SpendEvicted.Hook(func(id iotago.TransactionID) {
		conflictEvictedEventCount++
	})

	tf.Accounts.CreateID("nodeID1")
	tf.Accounts.CreateID("nodeID2")
	tf.Accounts.CreateID("nodeID3")
	tf.Accounts.CreateID("nodeID4")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateSpend("conflict2", []string{"resource1"}))
	tf.Assert.SpendSetMembers("resource1", "conflict1", "conflict2")
	tf.Assert.SpendSets("conflict1", "resource1")
	tf.Assert.SpendSets("conflict2", "resource1")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict3", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpendParents("conflict3", []string{"conflict1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpend("conflict4", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpendParents("conflict4", []string{"conflict1"}, []string{}))

	tf.Assert.SpendSetMembers("resource2", "conflict3", "conflict4")
	tf.Assert.Children("conflict1", "conflict3", "conflict4")
	tf.Assert.Parents("conflict3", "conflict1")
	tf.Assert.Parents("conflict4", "conflict1")

	require.NoError(t, tf.CreateOrUpdateSpend("conflict5", []string{"resource3"}))
	require.NoError(t, tf.UpdateSpendParents("conflict5", []string{"conflict2"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpend("conflict6", []string{"resource3"}))
	require.NoError(t, tf.UpdateSpendParents("conflict6", []string{"conflict2"}, []string{}))

	tf.Assert.SpendSetMembers("resource3", "conflict5", "conflict6")
	tf.Assert.Children("conflict2", "conflict5", "conflict6")
	tf.Assert.Parents("conflict5", "conflict2")
	tf.Assert.Parents("conflict6", "conflict2")

	require.NoError(t, tf.CastVotes("nodeID1", 1, "conflict2"))
	require.NoError(t, tf.CastVotes("nodeID2", 1, "conflict2"))
	require.NoError(t, tf.CastVotes("nodeID3", 1, "conflict2"))
	tf.Assert.LikedInstead([]string{"conflict1"}, "conflict2")

	tf.Assert.Rejected("conflict1")
	tf.Assert.Accepted("conflict2")
	tf.Assert.Rejected("conflict3")
	tf.Assert.Rejected("conflict4")
	tf.Assert.Pending("conflict5", "conflict6")

	tf.EvictSpend("conflict1")
	require.False(t, lo.Return2(tf.Instance.ConflictingSpends(tf.SpendID("conflict1"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingSpends(tf.SpendID("conflict2"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingSpends(tf.SpendID("conflict3"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingSpends(tf.SpendID("conflict4"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingSpends(tf.SpendID("conflict5"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingSpends(tf.SpendID("conflict6"))))
	require.Equal(t, 3, conflictEvictedEventCount)

	tf.EvictSpend("conflict1")
	require.Equal(t, 3, conflictEvictedEventCount)

	tf.Assert.SpendSetMembers("resource1", "conflict2")
	require.False(t, lo.Return2(tf.Instance.SpendSetMembers(tf.ResourceID("resource2"))))
	tf.Assert.SpendSetMembers("resource3", "conflict5", "conflict6")
	tf.Assert.Parents("conflict5", "conflict2")
	tf.Assert.Parents("conflict6", "conflict2")

	tf.EvictSpend("conflict6")
	require.False(t, lo.Return2(tf.Instance.ConflictingSpends(tf.SpendID("conflict1"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingSpends(tf.SpendID("conflict2"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingSpends(tf.SpendID("conflict3"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingSpends(tf.SpendID("conflict4"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingSpends(tf.SpendID("conflict5"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingSpends(tf.SpendID("conflict6"))))
	require.Equal(t, 4, conflictEvictedEventCount)

	tf.Assert.SpendSetMembers("resource1", "conflict2")
	require.False(t, lo.Return2(tf.Instance.SpendSetMembers(tf.ResourceID("resource2"))))
	tf.Assert.SpendSetMembers("resource3", "conflict5")
	tf.Assert.Parents("conflict5", "conflict2")
	tf.Assert.Children("conflict2", "conflict5")

	// Try to add non-existing parent to a pending conflict - nothing happens.
	require.NoError(t, tf.UpdateSpendParents("conflict5", []string{"conflict1"}, []string{}))

	parents, exists := tf.Instance.SpendParents(tf.SpendID("conflict5"))
	require.True(t, exists)
	require.False(t, parents.Has(tf.SpendID("conflict1")))
}
