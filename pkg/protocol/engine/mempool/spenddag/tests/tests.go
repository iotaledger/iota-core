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
		"CreateSpender":                 CreateSpender,
		"ExistingSpenderJoinsSpendSets": ExistingSpenderJoinsSpendSets,
		"JoinSpendSetTwice":             JoinSpendSetTwice,
		"UpdateSpenderParents":          UpdateSpenderParents,
		"LikedInstead":                  LikedInstead,
		"CreateSpendWithoutMembers":     CreateSpendWithoutMembers,
		"SpendAcceptance":               SpendAcceptance,
		"CastVotes":                     CastVotes,
		"CastVotes_VoteRank":            CastVotesVoteRank,
		"CastVotesAcceptance":           CastVotesAcceptance,
		"EvictAcceptedSpender":          EvictAcceptedSpender,
		"EvictRejectedSpender":          EvictRejectedSpender,
	} {
		t.Run(testName, func(t *testing.T) { testCase(t, frameworkProvider(t)) })
	}
}

func ExistingSpenderJoinsSpendSets(t *testing.T, tf *Framework) {
	t.Helper()

	require.NoError(t, tf.CreateOrUpdateSpender("spender1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateSpender("spender2", []string{"resource1"}))
	tf.Assert.SpendSetMembers("resource1", "spender1", "spender2")

	require.NoError(t, tf.CreateOrUpdateSpender("spender3", []string{"resource2"}))
	require.NoError(t, tf.CreateOrUpdateSpender("spender1", []string{"resource2"}))

	tf.Assert.SpendSetMembers("resource2", "spender1", "spender3")
	tf.Assert.SpendSetMembers("resource1", "spender1", "spender2")

	require.NoError(t, tf.CreateOrUpdateSpender("spender2", []string{"resource2"}))

	tf.Assert.SpendSetMembers("resource2", "spender1", "spender2", "spender3")
	tf.Assert.SpendSetMembers("resource1", "spender1", "spender2")
}

func UpdateSpenderParents(t *testing.T, tf *Framework) {
	t.Helper()

	require.NoError(t, tf.CreateOrUpdateSpender("spender1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateSpender("spender2", []string{"resource2"}))

	require.NoError(t, tf.CreateOrUpdateSpender("spender3", []string{"resource1", "resource2"}))
	require.NoError(t, tf.UpdateSpenderParents("spender3", []string{"spender1", "spender2"}, []string{}))
	tf.Assert.Children("spender1", "spender3")
	tf.Assert.Parents("spender3", "spender1", "spender2")

	require.NoError(t, tf.CreateOrUpdateSpender("spender2.5", []string{"spender2.5"}))
	require.NoError(t, tf.UpdateSpenderParents("spender2.5", []string{"spender1", "spender2"}, []string{}))
	tf.Assert.Children("spender1", "spender2.5", "spender3")
	tf.Assert.Children("spender2", "spender2.5", "spender3")
	tf.Assert.Parents("spender2.5", "spender1", "spender2")

	require.NoError(t, tf.UpdateSpenderParents("spender3", []string{"spender2.5"}, []string{"spender1", "spender2"}))

	tf.Assert.Children("spender1", "spender2.5")
	tf.Assert.Children("spender2", "spender2.5")
	tf.Assert.Children("spender2.5", "spender3")
	tf.Assert.Parents("spender3", "spender2.5")
	tf.Assert.Parents("spender2.5", "spender1", "spender2")
}

func CreateSpender(t *testing.T, tf *Framework) {
	t.Helper()

	require.NoError(t, tf.CreateOrUpdateSpender("spender1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateSpender("spender2", []string{"resource1"}))
	tf.Assert.SpendSetMembers("resource1", "spender1", "spender2")

	require.NoError(t, tf.CreateOrUpdateSpender("spender3", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpenderParents("spender3", []string{"spender1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpender("spender4", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpenderParents("spender4", []string{"spender1"}, []string{}))

	tf.Assert.SpendSetMembers("resource2", "spender3", "spender4")
	tf.Assert.Children("spender1", "spender3", "spender4")
	tf.Assert.Parents("spender3", "spender1")
	tf.Assert.Parents("spender4", "spender1")
}

func CreateSpendWithoutMembers(t *testing.T, tf *Framework) {
	t.Helper()

	tf.Accounts.CreateID("nodeID1")
	tf.Accounts.CreateID("nodeID2")
	tf.Accounts.CreateID("nodeID3")
	tf.Accounts.CreateID("nodeID4")

	// Non-conflicting conflicts
	{
		require.NoError(t, tf.CreateOrUpdateSpender("spender1", []string{"resource1"}))
		require.NoError(t, tf.CreateOrUpdateSpender("spender2", []string{"resource2"}))

		tf.Assert.SpendSetMembers("resource1", "spender1")
		tf.Assert.SpendSetMembers("resource2", "spender2")

		tf.Assert.LikedInstead([]string{"spender1"})
		tf.Assert.LikedInstead([]string{"spender2"})

		require.NoError(t, tf.CastVotes("nodeID1", 1, "spender1"))
		require.NoError(t, tf.CastVotes("nodeID2", 1, "spender1"))
		require.NoError(t, tf.CastVotes("nodeID3", 1, "spender1"))
		require.NoError(t, tf.CastVotes("nodeID1", 2, "spender1"))
		require.NoError(t, tf.CastVotes("nodeID2", 2, "spender1"))
		require.NoError(t, tf.CastVotes("nodeID3", 2, "spender1"))
		tf.Assert.LikedInstead([]string{"spender1"})
		tf.Assert.Accepted("spender1")
	}

	// Regular conflict
	{
		require.NoError(t, tf.CreateOrUpdateSpender("spender3", []string{"resource3"}))
		require.NoError(t, tf.CreateOrUpdateSpender("spender4", []string{"resource3"}))

		tf.Assert.SpendSetMembers("resource3", "spender3", "spender4")

		require.NoError(t, tf.CastVotes("nodeID3", 1, "spender3"))

		tf.Assert.LikedInstead([]string{"spender3"})
		tf.Assert.LikedInstead([]string{"spender4"}, "spender3")
	}

	tf.Assert.LikedInstead([]string{"spender1", "spender4"}, "spender3")
}

func LikedInstead(t *testing.T, tf *Framework) {
	t.Helper()

	tf.Accounts.CreateID("zero-weight")

	require.NoError(t, tf.CreateOrUpdateSpender("spender1", []string{"resource1"}))
	require.NoError(t, tf.CastVotes("zero-weight", 1, "spender1"))
	require.NoError(t, tf.CreateOrUpdateSpender("spender2", []string{"resource1"}))
	tf.Assert.SpendSetMembers("resource1", "spender1", "spender2")
	tf.Assert.LikedInstead([]string{"spender1", "spender2"}, "spender1")

	require.NoError(t, tf.CreateOrUpdateSpender("spender3", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpenderParents("spender3", []string{"spender1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpender("spender4", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpenderParents("spender4", []string{"spender1"}, []string{}))

	require.NoError(t, tf.CastVotes("zero-weight", 1, "spender4"))
	tf.Assert.LikedInstead([]string{"spender1", "spender2", "spender3", "spender4"}, "spender1", "spender4")
}

func SpendAcceptance(t *testing.T, tf *Framework) {
	t.Helper()

	tf.Accounts.CreateID("nodeID1")
	tf.Accounts.CreateID("nodeID2")
	tf.Accounts.CreateID("nodeID3")
	tf.Accounts.CreateID("nodeID4")

	require.NoError(t, tf.CreateOrUpdateSpender("spender1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateSpender("spender2", []string{"resource1"}))
	tf.Assert.SpendSetMembers("resource1", "spender1", "spender2")
	tf.Assert.SpendSets("spender1", "resource1")
	tf.Assert.SpendSets("spender2", "resource1")

	require.NoError(t, tf.CreateOrUpdateSpender("spender3", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpenderParents("spender3", []string{"spender1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpender("spender4", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpenderParents("spender4", []string{"spender1"}, []string{}))

	tf.Assert.SpendSetMembers("resource2", "spender3", "spender4")
	tf.Assert.Children("spender1", "spender3", "spender4")
	tf.Assert.Parents("spender3", "spender1")
	tf.Assert.Parents("spender4", "spender1")

	require.NoError(t, tf.CastVotes("nodeID1", 1, "spender4"))
	require.NoError(t, tf.CastVotes("nodeID2", 1, "spender4"))
	require.NoError(t, tf.CastVotes("nodeID3", 1, "spender4"))

	require.NoError(t, tf.CastVotes("nodeID1", 2, "spender4"))
	require.NoError(t, tf.CastVotes("nodeID2", 2, "spender4"))
	require.NoError(t, tf.CastVotes("nodeID3", 2, "spender4"))
	tf.Assert.LikedInstead([]string{"spender1"})
	tf.Assert.LikedInstead([]string{"spender2"}, "spender1")
	tf.Assert.LikedInstead([]string{"spender3"}, "spender4")
	tf.Assert.LikedInstead([]string{"spender4"})

	tf.Assert.Accepted("spender1", "spender4")
}

func CastVotes(t *testing.T, tf *Framework) {
	t.Helper()

	tf.Accounts.CreateID("nodeID1")
	tf.Accounts.CreateID("nodeID2")
	tf.Accounts.CreateID("nodeID3")
	tf.Accounts.CreateID("nodeID4")

	require.NoError(t, tf.CreateOrUpdateSpender("spender1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateSpender("spender2", []string{"resource1"}))
	tf.Assert.SpendSetMembers("resource1", "spender1", "spender2")
	tf.Assert.SpendSets("spender1", "resource1")
	tf.Assert.SpendSets("spender2", "resource1")

	require.NoError(t, tf.CreateOrUpdateSpender("spender3", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpenderParents("spender3", []string{"spender1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpender("spender4", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpenderParents("spender4", []string{"spender1"}, []string{}))

	tf.Assert.SpendSetMembers("resource2", "spender3", "spender4")
	tf.Assert.Children("spender1", "spender3", "spender4")
	tf.Assert.Parents("spender3", "spender1")
	tf.Assert.Parents("spender4", "spender1")

	require.NoError(t, tf.CastVotes("nodeID1", 1, "spender2"))
	require.NoError(t, tf.CastVotes("nodeID2", 1, "spender2"))
	require.NoError(t, tf.CastVotes("nodeID3", 1, "spender2"))
	require.NoError(t, tf.CastVotes("nodeID4", 2, "spender2"))
	require.NoError(t, tf.CastVotes("nodeID2", 2, "spender2"))
	require.NoError(t, tf.CastVotes("nodeID3", 2, "spender2"))

	tf.Assert.LikedInstead([]string{"spender1"}, "spender2")

	tf.Assert.Accepted("spender2")
	tf.Assert.Rejected("spender1")
	tf.Assert.Rejected("spender3")
	tf.Assert.Rejected("spender4")

	require.Error(t, tf.CastVotes("nodeID3", 1, "spender1", "spender2"))
}

func CastVotesVoteRank(t *testing.T, tf *Framework) {
	t.Helper()

	tf.Accounts.CreateID("nodeID1")
	tf.Accounts.CreateID("nodeID2")
	tf.Accounts.CreateID("nodeID3")

	require.NoError(t, tf.CreateOrUpdateSpender("spender1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateSpender("spender2", []string{"resource1"}))
	tf.Assert.SpendSetMembers("resource1", "spender1", "spender2")
	tf.Assert.SpendSets("spender1", "resource1")
	tf.Assert.SpendSets("spender2", "resource1")

	// create nested conflicts
	require.NoError(t, tf.CreateOrUpdateSpender("spender3", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpenderParents("spender3", []string{"spender1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpender("spender4", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpenderParents("spender4", []string{"spender1"}, []string{}))

	tf.Assert.SpendSetMembers("resource2", "spender3", "spender4")
	tf.Assert.Children("spender1", "spender3", "spender4")
	tf.Assert.Parents("spender3", "spender1")
	tf.Assert.Parents("spender4", "spender1")

	// casting a vote from a validator updates the validator weight
	require.NoError(t, tf.CastVotes("nodeID1", 2, "spender4"))
	tf.Assert.LikedInstead([]string{"spender1"})
	tf.Assert.LikedInstead([]string{"spender2"}, "spender1")
	tf.Assert.LikedInstead([]string{"spender3"}, "spender4")
	tf.Assert.LikedInstead([]string{"spender4"})

	// casting vote with lower vote power doesn't change the weights of conflicts
	require.NoError(t, tf.CastVotes("nodeID1", 1), "spender3")
	tf.Assert.LikedInstead([]string{"spender1"})
	tf.Assert.LikedInstead([]string{"spender2"}, "spender1")
	tf.Assert.LikedInstead([]string{"spender3"}, "spender4")
	tf.Assert.LikedInstead([]string{"spender4"})
	tf.Assert.ValidatorWeight("spender1", 1)
	tf.Assert.ValidatorWeight("spender2", 0)
	tf.Assert.ValidatorWeight("spender3", 0)
	tf.Assert.ValidatorWeight("spender4", 1)

	// casting vote with higher vote power changes the weights of conflicts
	require.NoError(t, tf.CastVotes("nodeID1", 3, "spender3"))
	tf.Assert.LikedInstead([]string{"spender1"})
	tf.Assert.LikedInstead([]string{"spender2"}, "spender1")
	tf.Assert.LikedInstead([]string{"spender3"})
	tf.Assert.LikedInstead([]string{"spender4"}, "spender3")
	tf.Assert.ValidatorWeight("spender1", 1)
	tf.Assert.ValidatorWeight("spender2", 0)
	tf.Assert.ValidatorWeight("spender3", 1)
	tf.Assert.ValidatorWeight("spender4", 0)
}

func CastVotesAcceptance(t *testing.T, tf *Framework) {
	t.Helper()

	tf.Accounts.CreateID("nodeID1")
	tf.Accounts.CreateID("nodeID2")
	tf.Accounts.CreateID("nodeID3")
	tf.Accounts.CreateID("nodeID4")

	require.NoError(t, tf.CreateOrUpdateSpender("spender1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateSpender("spender2", []string{"resource1"}))
	tf.Assert.SpendSetMembers("resource1", "spender1", "spender2")
	tf.Assert.SpendSets("spender1", "resource1")
	tf.Assert.SpendSets("spender2", "resource1")

	require.NoError(t, tf.CreateOrUpdateSpender("spender3", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpenderParents("spender3", []string{"spender1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpender("spender4", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpenderParents("spender4", []string{"spender1"}, []string{}))

	tf.Assert.SpendSetMembers("resource2", "spender3", "spender4")
	tf.Assert.Children("spender1", "spender3", "spender4")
	tf.Assert.Parents("spender3", "spender1")
	tf.Assert.Parents("spender4", "spender1")

	require.NoError(t, tf.CastVotes("nodeID1", 1, "spender3"))
	require.NoError(t, tf.CastVotes("nodeID2", 1, "spender3"))
	require.NoError(t, tf.CastVotes("nodeID3", 1, "spender3"))
	require.NoError(t, tf.CastVotes("nodeID4", 2, "spender3"))
	require.NoError(t, tf.CastVotes("nodeID2", 2, "spender3"))
	require.NoError(t, tf.CastVotes("nodeID3", 2, "spender3"))
	tf.Assert.LikedInstead([]string{"spender1"})
	tf.Assert.Accepted("spender1")
	tf.Assert.Rejected("spender2")
	tf.Assert.Accepted("spender3")
	tf.Assert.Rejected("spender4")

	require.NoError(t, tf.CreateOrUpdateSpender("spender5", []string{"resource1"}))
	tf.Assert.Rejected("spender5")

	// Evict conflict and try to add non-existing parent to a rejected conflict - update is ignored because the parent is evicted.
	tf.EvictSpender("spender2")
	require.NoError(t, tf.UpdateSpenderParents("spender4", []string{"spender2"}, []string{}))
	parents, exists := tf.Instance.SpenderParents(tf.SpenderID("spender4"))
	require.True(t, exists)
	require.False(t, parents.Has(tf.SpenderID("spender2")))

	// Try to update parents of evicted conflict.
	require.ErrorIs(t, tf.UpdateSpenderParents("spender2", []string{"spender1"}, []string{}), spenddag.ErrEntityEvicted)
}

func JoinSpendSetTwice(t *testing.T, tf *Framework) {
	t.Helper()

	var conflictCreatedEventCount, resourceAddedEventCount int
	tf.Instance.Events().SpenderCreated.Hook(func(_ iotago.TransactionID) {
		conflictCreatedEventCount++
	})

	tf.Instance.Events().SpentResourcesAdded.Hook(func(id iotago.TransactionID, resourceID ds.Set[iotago.OutputID]) {
		fmt.Println("spender joins spendset", id, resourceID)
		resourceAddedEventCount++
	})

	require.NoError(t, tf.CreateOrUpdateSpender("spender1", []string{"resource1"}))
	require.Equal(t, 1, conflictCreatedEventCount)
	require.Equal(t, 1, resourceAddedEventCount)
	tf.Assert.SpendSets("spender1", "resource1")

	require.NoError(t, tf.CreateOrUpdateSpender("spender1", []string{"resource2"}))
	require.Equal(t, 1, conflictCreatedEventCount)
	require.Equal(t, 2, resourceAddedEventCount)
	tf.Assert.SpendSets("spender1", "resource1", "resource2")

	require.NoError(t, tf.CreateOrUpdateSpender("spender1", []string{"resource1", "resource2"}))
	require.Equal(t, 1, conflictCreatedEventCount)
	require.Equal(t, 2, resourceAddedEventCount)
	tf.Assert.SpendSets("spender1", "resource1", "resource2")

	require.NoError(t, tf.CreateOrUpdateSpender("spender1", []string{"resource1", "resource2", "resource3", "resource4"}))
	require.Equal(t, 1, conflictCreatedEventCount)
	require.Equal(t, 3, resourceAddedEventCount)
	tf.Assert.SpendSets("spender1", "resource1", "resource2", "resource3", "resource4")
}

func EvictAcceptedSpender(t *testing.T, tf *Framework) {
	t.Helper()

	tf.Accounts.CreateID("nodeID1")
	tf.Accounts.CreateID("nodeID2")
	tf.Accounts.CreateID("nodeID3")
	tf.Accounts.CreateID("nodeID4")

	require.NoError(t, tf.CreateOrUpdateSpender("spender1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateSpender("spender2", []string{"resource1"}))
	tf.Assert.SpendSetMembers("resource1", "spender1", "spender2")
	tf.Assert.SpendSets("spender1", "resource1")
	tf.Assert.SpendSets("spender2", "resource1")

	require.NoError(t, tf.CreateOrUpdateSpender("spender3", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpenderParents("spender3", []string{"spender1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpender("spender4", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpenderParents("spender4", []string{"spender1"}, []string{}))

	tf.Assert.SpendSetMembers("resource2", "spender3", "spender4")
	tf.Assert.Children("spender1", "spender3", "spender4")
	tf.Assert.Parents("spender3", "spender1")
	tf.Assert.Parents("spender4", "spender1")

	require.NoError(t, tf.CreateOrUpdateSpender("spender5", []string{"resource3"}))
	require.NoError(t, tf.UpdateSpenderParents("spender5", []string{"spender2"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpender("spender6", []string{"resource3"}))
	require.NoError(t, tf.UpdateSpenderParents("spender6", []string{"spender2"}, []string{}))

	tf.Assert.SpendSetMembers("resource3", "spender5", "spender6")
	tf.Assert.Children("spender2", "spender5", "spender6")
	tf.Assert.Parents("spender5", "spender2")
	tf.Assert.Parents("spender6", "spender2")

	require.NoError(t, tf.CastVotes("nodeID1", 1, "spender2"))
	require.NoError(t, tf.CastVotes("nodeID2", 1, "spender2"))
	require.NoError(t, tf.CastVotes("nodeID3", 1, "spender2"))
	require.NoError(t, tf.CastVotes("nodeID1", 2, "spender2"))
	require.NoError(t, tf.CastVotes("nodeID2", 2, "spender2"))
	require.NoError(t, tf.CastVotes("nodeID3", 2, "spender2"))
	tf.Assert.LikedInstead([]string{"spender1"}, "spender2")

	tf.Assert.Accepted("spender2")
	tf.Assert.Rejected("spender1")
	tf.Assert.Rejected("spender3")
	tf.Assert.Rejected("spender4")
	tf.Assert.Pending("spender5", "spender6")

	tf.EvictSpender("spender2")
	require.False(t, lo.Return2(tf.Instance.ConflictingSpenders(tf.SpenderID("spender1"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingSpenders(tf.SpenderID("spender2"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingSpenders(tf.SpenderID("spender3"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingSpenders(tf.SpenderID("spender4"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingSpenders(tf.SpenderID("spender5"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingSpenders(tf.SpenderID("spender6"))))

	require.False(t, lo.Return2(tf.Instance.SpendSetMembers(tf.ResourceID("resource1"))))
	require.False(t, lo.Return2(tf.Instance.SpendSetMembers(tf.ResourceID("resource2"))))
	tf.Assert.SpendSetMembers("resource3", "spender5", "spender6")

	tf.Assert.Parents("spender5")
	tf.Assert.Parents("spender6")
}

func EvictRejectedSpender(t *testing.T, tf *Framework) {
	t.Helper()

	conflictEvictedEventCount := 0
	tf.Instance.Events().SpenderEvicted.Hook(func(_ iotago.TransactionID) {
		conflictEvictedEventCount++
	})

	tf.Accounts.CreateID("nodeID1")
	tf.Accounts.CreateID("nodeID2")
	tf.Accounts.CreateID("nodeID3")
	tf.Accounts.CreateID("nodeID4")

	require.NoError(t, tf.CreateOrUpdateSpender("spender1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateSpender("spender2", []string{"resource1"}))
	tf.Assert.SpendSetMembers("resource1", "spender1", "spender2")
	tf.Assert.SpendSets("spender1", "resource1")
	tf.Assert.SpendSets("spender2", "resource1")

	require.NoError(t, tf.CreateOrUpdateSpender("spender3", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpenderParents("spender3", []string{"spender1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpender("spender4", []string{"resource2"}))
	require.NoError(t, tf.UpdateSpenderParents("spender4", []string{"spender1"}, []string{}))

	tf.Assert.SpendSetMembers("resource2", "spender3", "spender4")
	tf.Assert.Children("spender1", "spender3", "spender4")
	tf.Assert.Parents("spender3", "spender1")
	tf.Assert.Parents("spender4", "spender1")

	require.NoError(t, tf.CreateOrUpdateSpender("spender5", []string{"resource3"}))
	require.NoError(t, tf.UpdateSpenderParents("spender5", []string{"spender2"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateSpender("spender6", []string{"resource3"}))
	require.NoError(t, tf.UpdateSpenderParents("spender6", []string{"spender2"}, []string{}))

	tf.Assert.SpendSetMembers("resource3", "spender5", "spender6")
	tf.Assert.Children("spender2", "spender5", "spender6")
	tf.Assert.Parents("spender5", "spender2")
	tf.Assert.Parents("spender6", "spender2")

	require.NoError(t, tf.CastVotes("nodeID1", 1, "spender2"))
	require.NoError(t, tf.CastVotes("nodeID2", 1, "spender2"))
	require.NoError(t, tf.CastVotes("nodeID3", 1, "spender2"))
	require.NoError(t, tf.CastVotes("nodeID1", 2, "spender2"))
	require.NoError(t, tf.CastVotes("nodeID2", 2, "spender2"))
	require.NoError(t, tf.CastVotes("nodeID3", 2, "spender2"))
	tf.Assert.LikedInstead([]string{"spender1"}, "spender2")

	tf.Assert.Rejected("spender1")
	tf.Assert.Accepted("spender2")
	tf.Assert.Rejected("spender3")
	tf.Assert.Rejected("spender4")
	tf.Assert.Pending("spender5", "spender6")

	tf.EvictSpender("spender1")
	require.False(t, lo.Return2(tf.Instance.ConflictingSpenders(tf.SpenderID("spender1"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingSpenders(tf.SpenderID("spender2"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingSpenders(tf.SpenderID("spender3"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingSpenders(tf.SpenderID("spender4"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingSpenders(tf.SpenderID("spender5"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingSpenders(tf.SpenderID("spender6"))))
	require.Equal(t, 3, conflictEvictedEventCount)

	tf.EvictSpender("spender1")
	require.Equal(t, 3, conflictEvictedEventCount)

	tf.Assert.SpendSetMembers("resource1", "spender2")
	require.False(t, lo.Return2(tf.Instance.SpendSetMembers(tf.ResourceID("resource2"))))
	tf.Assert.SpendSetMembers("resource3", "spender5", "spender6")
	tf.Assert.Parents("spender5", "spender2")
	tf.Assert.Parents("spender6", "spender2")

	tf.EvictSpender("spender6")
	require.False(t, lo.Return2(tf.Instance.ConflictingSpenders(tf.SpenderID("spender1"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingSpenders(tf.SpenderID("spender2"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingSpenders(tf.SpenderID("spender3"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingSpenders(tf.SpenderID("spender4"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingSpenders(tf.SpenderID("spender5"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingSpenders(tf.SpenderID("spender6"))))
	require.Equal(t, 4, conflictEvictedEventCount)

	tf.Assert.SpendSetMembers("resource1", "spender2")
	require.False(t, lo.Return2(tf.Instance.SpendSetMembers(tf.ResourceID("resource2"))))
	tf.Assert.SpendSetMembers("resource3", "spender5")
	tf.Assert.Parents("spender5", "spender2")
	tf.Assert.Children("spender2", "spender5")

	// Try to add non-existing parent to a pending spender - nothing happens.
	require.NoError(t, tf.UpdateSpenderParents("spender5", []string{"spender1"}, []string{}))

	parents, exists := tf.Instance.SpenderParents(tf.SpenderID("spender5"))
	require.True(t, exists)
	require.False(t, parents.Has(tf.SpenderID("spender1")))
}
