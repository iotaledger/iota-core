package tests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds/set"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestAll(t *testing.T, frameworkProvider func(*testing.T) *Framework) {
	for testName, testCase := range map[string]func(*testing.T, *Framework){
		"CreateConflict":                    CreateConflict,
		"ExistingConflictJoinsConflictSets": ExistingConflictJoinsConflictSets,
		"JoinConflictSetTwice":              JoinConflictSetTwice,
		"UpdateConflictParents":             UpdateConflictParents,
		"LikedInstead":                      LikedInstead,
		"CreateConflictWithoutMembers":      CreateConflictWithoutMembers,
		"ConflictAcceptance":                ConflictAcceptance,
		"CastVotes":                         CastVotes,
		"CastVotes_VoteRank":                CastVotesVoteRank,
		"CastVotesAcceptance":               CastVotesAcceptance,
		"EvictAcceptedConflict":             EvictAcceptedConflict,
		"EvictRejectedConflict":             EvictRejectedConflict,
	} {
		t.Run(testName, func(t *testing.T) { testCase(t, frameworkProvider(t)) })
	}
}

func ExistingConflictJoinsConflictSets(t *testing.T, tf *Framework) {
	require.NoError(t, tf.CreateOrUpdateConflict("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateConflict("conflict2", []string{"resource1"}))
	tf.Assert.ConflictSetMembers("resource1", "conflict1", "conflict2")

	require.NoError(t, tf.CreateOrUpdateConflict("conflict3", []string{"resource2"}))
	require.NoError(t, tf.CreateOrUpdateConflict("conflict1", []string{"resource2"}))

	tf.Assert.ConflictSetMembers("resource2", "conflict1", "conflict3")
	tf.Assert.ConflictSetMembers("resource1", "conflict1", "conflict2")

	require.NoError(t, tf.CreateOrUpdateConflict("conflict2", []string{"resource2"}))

	tf.Assert.ConflictSetMembers("resource2", "conflict1", "conflict2", "conflict3")
	tf.Assert.ConflictSetMembers("resource1", "conflict1", "conflict2")

	tf.Assert.LikedInstead([]string{"conflict3"}, "conflict1")
}

func UpdateConflictParents(t *testing.T, tf *Framework) {
	require.NoError(t, tf.CreateOrUpdateConflict("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateConflict("conflict2", []string{"resource2"}))

	require.NoError(t, tf.CreateOrUpdateConflict("conflict3", []string{"resource1", "resource2"}))
	require.NoError(t, tf.UpdateConflictParents("conflict3", []string{"conflict1", "conflict2"}, []string{}))
	tf.Assert.Children("conflict1", "conflict3")
	tf.Assert.Parents("conflict3", "conflict1", "conflict2")

	require.NoError(t, tf.CreateOrUpdateConflict("conflict2.5", []string{"conflict2.5"}))
	require.NoError(t, tf.UpdateConflictParents("conflict2.5", []string{"conflict1", "conflict2"}, []string{}))
	tf.Assert.Children("conflict1", "conflict2.5", "conflict3")
	tf.Assert.Children("conflict2", "conflict2.5", "conflict3")
	tf.Assert.Parents("conflict2.5", "conflict1", "conflict2")

	require.NoError(t, tf.UpdateConflictParents("conflict3", []string{"conflict2.5"}, []string{"conflict1", "conflict2"}))

	tf.Assert.Children("conflict1", "conflict2.5")
	tf.Assert.Children("conflict2", "conflict2.5")
	tf.Assert.Children("conflict2.5", "conflict3")
	tf.Assert.Parents("conflict3", "conflict2.5")
	tf.Assert.Parents("conflict2.5", "conflict1", "conflict2")
}

func CreateConflict(t *testing.T, tf *Framework) {
	require.NoError(t, tf.CreateOrUpdateConflict("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateConflict("conflict2", []string{"resource1"}))
	tf.Assert.ConflictSetMembers("resource1", "conflict1", "conflict2")

	require.NoError(t, tf.CreateOrUpdateConflict("conflict3", []string{"resource2"}))
	require.NoError(t, tf.UpdateConflictParents("conflict3", []string{"conflict1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateConflict("conflict4", []string{"resource2"}))
	require.NoError(t, tf.UpdateConflictParents("conflict4", []string{"conflict1"}, []string{}))

	tf.Assert.ConflictSetMembers("resource2", "conflict3", "conflict4")
	tf.Assert.Children("conflict1", "conflict3", "conflict4")
	tf.Assert.Parents("conflict3", "conflict1")
	tf.Assert.Parents("conflict4", "conflict1")
}

func CreateConflictWithoutMembers(t *testing.T, tf *Framework) {
	tf.Accounts.CreateID("nodeID1")
	tf.Accounts.CreateID("nodeID2")
	tf.Accounts.CreateID("nodeID3")
	tf.Accounts.CreateID("nodeID4")

	// Non-conflicting conflicts
	{
		require.NoError(t, tf.CreateOrUpdateConflict("conflict1", []string{"resource1"}))
		require.NoError(t, tf.CreateOrUpdateConflict("conflict2", []string{"resource2"}))

		tf.Assert.ConflictSetMembers("resource1", "conflict1")
		tf.Assert.ConflictSetMembers("resource2", "conflict2")

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
		require.NoError(t, tf.CreateOrUpdateConflict("conflict3", []string{"resource3"}))
		require.NoError(t, tf.CreateOrUpdateConflict("conflict4", []string{"resource3"}))

		tf.Assert.ConflictSetMembers("resource3", "conflict3", "conflict4")

		require.NoError(t, tf.CastVotes("nodeID3", 1, "conflict3"))

		tf.Assert.LikedInstead([]string{"conflict3"})
		tf.Assert.LikedInstead([]string{"conflict4"}, "conflict3")
	}

	tf.Assert.LikedInstead([]string{"conflict1", "conflict4"}, "conflict3")
}

func LikedInstead(t *testing.T, tf *Framework) {
	tf.Accounts.CreateID("zero-weight")

	require.NoError(t, tf.CreateOrUpdateConflict("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CastVotes("zero-weight", 1, "conflict1"))
	require.NoError(t, tf.CreateOrUpdateConflict("conflict2", []string{"resource1"}))
	tf.Assert.ConflictSetMembers("resource1", "conflict1", "conflict2")
	tf.Assert.LikedInstead([]string{"conflict1", "conflict2"}, "conflict1")

	require.NoError(t, tf.CreateOrUpdateConflict("conflict3", []string{"resource2"}))
	require.NoError(t, tf.UpdateConflictParents("conflict3", []string{"conflict1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateConflict("conflict4", []string{"resource2"}))
	require.NoError(t, tf.UpdateConflictParents("conflict4", []string{"conflict1"}, []string{}))

	require.NoError(t, tf.CastVotes("zero-weight", 1, "conflict4"))
	tf.Assert.LikedInstead([]string{"conflict1", "conflict2", "conflict3", "conflict4"}, "conflict1", "conflict4")
}

func ConflictAcceptance(t *testing.T, tf *Framework) {
	tf.Accounts.CreateID("nodeID1")
	tf.Accounts.CreateID("nodeID2")
	tf.Accounts.CreateID("nodeID3")
	tf.Accounts.CreateID("nodeID4")

	require.NoError(t, tf.CreateOrUpdateConflict("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateConflict("conflict2", []string{"resource1"}))
	tf.Assert.ConflictSetMembers("resource1", "conflict1", "conflict2")
	tf.Assert.ConflictSets("conflict1", "resource1")
	tf.Assert.ConflictSets("conflict2", "resource1")

	require.NoError(t, tf.CreateOrUpdateConflict("conflict3", []string{"resource2"}))
	require.NoError(t, tf.UpdateConflictParents("conflict3", []string{"conflict1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateConflict("conflict4", []string{"resource2"}))
	require.NoError(t, tf.UpdateConflictParents("conflict4", []string{"conflict1"}, []string{}))

	tf.Assert.ConflictSetMembers("resource2", "conflict3", "conflict4")
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

	require.NoError(t, tf.CreateOrUpdateConflict("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateConflict("conflict2", []string{"resource1"}))
	tf.Assert.ConflictSetMembers("resource1", "conflict1", "conflict2")
	tf.Assert.ConflictSets("conflict1", "resource1")
	tf.Assert.ConflictSets("conflict2", "resource1")

	require.NoError(t, tf.CreateOrUpdateConflict("conflict3", []string{"resource2"}))
	require.NoError(t, tf.UpdateConflictParents("conflict3", []string{"conflict1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateConflict("conflict4", []string{"resource2"}))
	require.NoError(t, tf.UpdateConflictParents("conflict4", []string{"conflict1"}, []string{}))

	tf.Assert.ConflictSetMembers("resource2", "conflict3", "conflict4")
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

	require.NoError(t, tf.CreateOrUpdateConflict("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateConflict("conflict2", []string{"resource1"}))
	tf.Assert.ConflictSetMembers("resource1", "conflict1", "conflict2")
	tf.Assert.ConflictSets("conflict1", "resource1")
	tf.Assert.ConflictSets("conflict2", "resource1")

	// create nested conflicts
	require.NoError(t, tf.CreateOrUpdateConflict("conflict3", []string{"resource2"}))
	require.NoError(t, tf.UpdateConflictParents("conflict3", []string{"conflict1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateConflict("conflict4", []string{"resource2"}))
	require.NoError(t, tf.UpdateConflictParents("conflict4", []string{"conflict1"}, []string{}))

	tf.Assert.ConflictSetMembers("resource2", "conflict3", "conflict4")
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

	require.NoError(t, tf.CreateOrUpdateConflict("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateConflict("conflict2", []string{"resource1"}))
	tf.Assert.ConflictSetMembers("resource1", "conflict1", "conflict2")
	tf.Assert.ConflictSets("conflict1", "resource1")
	tf.Assert.ConflictSets("conflict2", "resource1")

	require.NoError(t, tf.CreateOrUpdateConflict("conflict3", []string{"resource2"}))
	require.NoError(t, tf.UpdateConflictParents("conflict3", []string{"conflict1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateConflict("conflict4", []string{"resource2"}))
	require.NoError(t, tf.UpdateConflictParents("conflict4", []string{"conflict1"}, []string{}))

	tf.Assert.ConflictSetMembers("resource2", "conflict3", "conflict4")
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

	require.NoError(t, tf.CreateOrUpdateConflict("conflict5", []string{"resource1"}))
	tf.Assert.Rejected("conflict5")

	// Evict conflict and try to add non-existing parent to a rejected conflict.
	tf.EvictConflict("conflict2")
	require.ErrorIs(t, tf.UpdateConflictParents("conflict4", []string{"conflict2"}, []string{}), conflictdag.ErrEntityEvicted)

	// Try to update parents of evicted conflict
	require.ErrorIs(t, tf.UpdateConflictParents("conflict2", []string{"conflict1"}, []string{}), conflictdag.ErrEntityEvicted)
}

func JoinConflictSetTwice(t *testing.T, tf *Framework) {
	var conflictCreatedEventCount, resourceAddedEventCount int
	tf.Instance.Events().ConflictCreated.Hook(func(id iotago.TransactionID) {
		conflictCreatedEventCount++
	})

	tf.Instance.Events().ConflictingResourcesAdded.Hook(func(id iotago.TransactionID, resourceID set.Set[iotago.OutputID]) {
		fmt.Println("conflict joins conflictset", id, resourceID)
		resourceAddedEventCount++
	})

	require.NoError(t, tf.CreateOrUpdateConflict("conflict1", []string{"resource1"}))
	require.Equal(t, 1, conflictCreatedEventCount)
	require.Equal(t, 1, resourceAddedEventCount)
	tf.Assert.ConflictSets("conflict1", "resource1")

	require.NoError(t, tf.CreateOrUpdateConflict("conflict1", []string{"resource2"}))
	require.Equal(t, 1, conflictCreatedEventCount)
	require.Equal(t, 2, resourceAddedEventCount)
	tf.Assert.ConflictSets("conflict1", "resource1", "resource2")

	require.NoError(t, tf.CreateOrUpdateConflict("conflict1", []string{"resource1", "resource2"}))
	require.Equal(t, 1, conflictCreatedEventCount)
	require.Equal(t, 2, resourceAddedEventCount)
	tf.Assert.ConflictSets("conflict1", "resource1", "resource2")

	require.NoError(t, tf.CreateOrUpdateConflict("conflict1", []string{"resource1", "resource2", "resource3", "resource4"}))
	require.Equal(t, 1, conflictCreatedEventCount)
	require.Equal(t, 3, resourceAddedEventCount)
	tf.Assert.ConflictSets("conflict1", "resource1", "resource2", "resource3", "resource4")
}

func EvictAcceptedConflict(t *testing.T, tf *Framework) {
	tf.Accounts.CreateID("nodeID1")
	tf.Accounts.CreateID("nodeID2")
	tf.Accounts.CreateID("nodeID3")
	tf.Accounts.CreateID("nodeID4")

	require.NoError(t, tf.CreateOrUpdateConflict("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateConflict("conflict2", []string{"resource1"}))
	tf.Assert.ConflictSetMembers("resource1", "conflict1", "conflict2")
	tf.Assert.ConflictSets("conflict1", "resource1")
	tf.Assert.ConflictSets("conflict2", "resource1")

	require.NoError(t, tf.CreateOrUpdateConflict("conflict3", []string{"resource2"}))
	require.NoError(t, tf.UpdateConflictParents("conflict3", []string{"conflict1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateConflict("conflict4", []string{"resource2"}))
	require.NoError(t, tf.UpdateConflictParents("conflict4", []string{"conflict1"}, []string{}))

	tf.Assert.ConflictSetMembers("resource2", "conflict3", "conflict4")
	tf.Assert.Children("conflict1", "conflict3", "conflict4")
	tf.Assert.Parents("conflict3", "conflict1")
	tf.Assert.Parents("conflict4", "conflict1")

	require.NoError(t, tf.CreateOrUpdateConflict("conflict5", []string{"resource3"}))
	require.NoError(t, tf.UpdateConflictParents("conflict5", []string{"conflict2"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateConflict("conflict6", []string{"resource3"}))
	require.NoError(t, tf.UpdateConflictParents("conflict6", []string{"conflict2"}, []string{}))

	tf.Assert.ConflictSetMembers("resource3", "conflict5", "conflict6")
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

	tf.EvictConflict("conflict2")
	require.False(t, lo.Return2(tf.Instance.ConflictingConflicts(tf.ConflictID("conflict1"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingConflicts(tf.ConflictID("conflict2"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingConflicts(tf.ConflictID("conflict3"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingConflicts(tf.ConflictID("conflict4"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingConflicts(tf.ConflictID("conflict5"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingConflicts(tf.ConflictID("conflict6"))))

	require.False(t, lo.Return2(tf.Instance.ConflictSetMembers(tf.ResourceID("resource1"))))
	require.False(t, lo.Return2(tf.Instance.ConflictSetMembers(tf.ResourceID("resource2"))))
	tf.Assert.ConflictSetMembers("resource3", "conflict5", "conflict6")

	tf.Assert.Parents("conflict5")
	tf.Assert.Parents("conflict6")
}

func EvictRejectedConflict(t *testing.T, tf *Framework) {
	conflictEvictedEventCount := 0
	tf.Instance.Events().ConflictEvicted.Hook(func(id iotago.TransactionID) {
		conflictEvictedEventCount++
	})

	tf.Accounts.CreateID("nodeID1")
	tf.Accounts.CreateID("nodeID2")
	tf.Accounts.CreateID("nodeID3")
	tf.Accounts.CreateID("nodeID4")

	require.NoError(t, tf.CreateOrUpdateConflict("conflict1", []string{"resource1"}))
	require.NoError(t, tf.CreateOrUpdateConflict("conflict2", []string{"resource1"}))
	tf.Assert.ConflictSetMembers("resource1", "conflict1", "conflict2")
	tf.Assert.ConflictSets("conflict1", "resource1")
	tf.Assert.ConflictSets("conflict2", "resource1")

	require.NoError(t, tf.CreateOrUpdateConflict("conflict3", []string{"resource2"}))
	require.NoError(t, tf.UpdateConflictParents("conflict3", []string{"conflict1"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateConflict("conflict4", []string{"resource2"}))
	require.NoError(t, tf.UpdateConflictParents("conflict4", []string{"conflict1"}, []string{}))

	tf.Assert.ConflictSetMembers("resource2", "conflict3", "conflict4")
	tf.Assert.Children("conflict1", "conflict3", "conflict4")
	tf.Assert.Parents("conflict3", "conflict1")
	tf.Assert.Parents("conflict4", "conflict1")

	require.NoError(t, tf.CreateOrUpdateConflict("conflict5", []string{"resource3"}))
	require.NoError(t, tf.UpdateConflictParents("conflict5", []string{"conflict2"}, []string{}))

	require.NoError(t, tf.CreateOrUpdateConflict("conflict6", []string{"resource3"}))
	require.NoError(t, tf.UpdateConflictParents("conflict6", []string{"conflict2"}, []string{}))

	tf.Assert.ConflictSetMembers("resource3", "conflict5", "conflict6")
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

	tf.EvictConflict("conflict1")
	require.False(t, lo.Return2(tf.Instance.ConflictingConflicts(tf.ConflictID("conflict1"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingConflicts(tf.ConflictID("conflict2"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingConflicts(tf.ConflictID("conflict3"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingConflicts(tf.ConflictID("conflict4"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingConflicts(tf.ConflictID("conflict5"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingConflicts(tf.ConflictID("conflict6"))))
	require.Equal(t, 3, conflictEvictedEventCount)

	tf.EvictConflict("conflict1")
	require.Equal(t, 3, conflictEvictedEventCount)

	tf.Assert.ConflictSetMembers("resource1", "conflict2")
	require.False(t, lo.Return2(tf.Instance.ConflictSetMembers(tf.ResourceID("resource2"))))
	tf.Assert.ConflictSetMembers("resource3", "conflict5", "conflict6")
	tf.Assert.Parents("conflict5", "conflict2")
	tf.Assert.Parents("conflict6", "conflict2")

	tf.EvictConflict("conflict6")
	require.False(t, lo.Return2(tf.Instance.ConflictingConflicts(tf.ConflictID("conflict1"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingConflicts(tf.ConflictID("conflict2"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingConflicts(tf.ConflictID("conflict3"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingConflicts(tf.ConflictID("conflict4"))))
	require.True(t, lo.Return2(tf.Instance.ConflictingConflicts(tf.ConflictID("conflict5"))))
	require.False(t, lo.Return2(tf.Instance.ConflictingConflicts(tf.ConflictID("conflict6"))))
	require.Equal(t, 4, conflictEvictedEventCount)

	tf.Assert.ConflictSetMembers("resource1", "conflict2")
	require.False(t, lo.Return2(tf.Instance.ConflictSetMembers(tf.ResourceID("resource2"))))
	tf.Assert.ConflictSetMembers("resource3", "conflict5")
	tf.Assert.Parents("conflict5", "conflict2")
	tf.Assert.Children("conflict2", "conflict5")

	// Try to add non-existing parent to a pending conflict.
	require.ErrorIs(t, tf.UpdateConflictParents("conflict5", []string{"conflict1"}, []string{}), conflictdag.ErrFatal)
}
