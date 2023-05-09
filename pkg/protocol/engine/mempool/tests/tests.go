package mempooltests

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/debug"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
)

func TestAll(t *testing.T, frameworkProvider func(*testing.T) *TestFramework) {
	for testName, testCase := range map[string]func(*testing.T, *TestFramework){
		"TestProcessTransaction":                TestProcessTransaction,
		"TestProcessTransactionsOutOfOrder":     TestProcessTransactionsOutOfOrder,
		"TestSetInclusionSlot":                  TestSetInclusionSlot,
		"TestSetTxOrphanage":                    TestSetTxOrphanage,
		"TestSetTxOrphanageMultipleAttachments": TestSetTxOrphanageMultipleAttachments,
		"TestStateDiff":                         TestStateDiff,
	} {
		t.Run(testName, func(t *testing.T) { testCase(t, frameworkProvider(t)) })
	}
}

func TestProcessTransaction(t *testing.T, tf *TestFramework) {
	tf.CreateTransaction("tx1", []string{"genesis"}, 1)
	tf.CreateTransaction("tx2", []string{"tx1:0"}, 1)

	require.NoError(t, tf.AttachTransactions("tx1", "tx2"))

	tf.RequireBooked("tx1", "tx2")

	tx1Metadata, exists := tf.TransactionMetadata("tx1")
	require.True(t, exists)

	_ = tx1Metadata.Outputs().ForEach(func(state mempool.StateWithMetadata) error {
		require.False(t, state.InclusionState().IsAccepted())
		require.True(t, state.SpentState().IsSpent())

		return nil
	})

	tx2Metadata, exists := tf.TransactionMetadata("tx2")
	require.True(t, exists)

	_ = tx2Metadata.Outputs().ForEach(func(state mempool.StateWithMetadata) error {
		require.False(t, state.InclusionState().IsAccepted())
		require.False(t, state.SpentState().IsSpent())

		return nil
	})
}

func TestProcessTransactionsOutOfOrder(t *testing.T, tf *TestFramework) {
	tf.CreateTransaction("tx1", []string{"genesis"}, 1)
	tf.CreateTransaction("tx2", []string{"tx1:0"}, 1)
	tf.CreateTransaction("tx3", []string{"tx2:0"}, 1)

	require.NoError(t, tf.AttachTransactions("tx3"))
	require.NoError(t, tf.AttachTransactions("tx2"))
	require.NoError(t, tf.AttachTransactions("tx1"))

	tf.RequireBooked("tx1", "tx2", "tx3")

	tx1Metadata, exists := tf.TransactionMetadata("tx1")
	require.True(t, exists)

	_ = tx1Metadata.Outputs().ForEach(func(state mempool.StateWithMetadata) error {
		require.False(t, state.InclusionState().IsAccepted())
		require.True(t, state.SpentState().IsSpent())

		return nil
	})

	tx2Metadata, exists := tf.TransactionMetadata("tx2")
	require.True(t, exists)

	_ = tx2Metadata.Outputs().ForEach(func(state mempool.StateWithMetadata) error {
		require.False(t, state.InclusionState().IsAccepted())
		require.True(t, state.SpentState().IsSpent())

		return nil
	})

	tx3Metadata, exists := tf.TransactionMetadata("tx3")
	require.True(t, exists)

	_ = tx3Metadata.Outputs().ForEach(func(state mempool.StateWithMetadata) error {
		require.False(t, state.InclusionState().IsAccepted())
		require.False(t, state.SpentState().IsSpent())

		return nil
	})
}

func TestSetInclusionSlot(t *testing.T, tf *TestFramework) {
	debug.SetEnabled(true)
	defer debug.SetEnabled(false)
	tf.CreateTransaction("tx1", []string{"genesis"}, 1)
	tf.CreateTransaction("tx2", []string{"tx1:0"}, 1)
	tf.CreateTransaction("tx3", []string{"tx2:0"}, 1)

	require.NoError(t, tf.AttachTransaction("tx3", "block3", 3))
	require.NoError(t, tf.AttachTransaction("tx2", "block2", 2))
	require.NoError(t, tf.AttachTransaction("tx1", "block1", 1))

	tf.RequireBooked("tx1", "tx2", "tx3")

	require.True(t, tf.MarkAttachmentIncluded("block2"))

	require.True(t, tf.MarkAttachmentIncluded("block1"))

	tf.RequireAccepted(map[string]bool{"tx1": true, "tx2": true, "tx3": false})

	// TODO: implement test that checks behavior of accepted, but not commited transactions during slot eviction once we define what to do then
	//tf.Instance.Evict(1)
	//time.Sleep(100 * time.Millisecond)
	//tf.RequireEvicted("tx1", "tx2", "tx3")

	tx1Metadata, exists := tf.TransactionMetadata("tx1")
	require.True(t, exists)

	tx2Metadata, exists := tf.TransactionMetadata("tx2")
	require.True(t, exists)

	tx3Metadata, exists := tf.TransactionMetadata("tx3")
	require.True(t, exists)

	tx1Metadata.SetCommitted()
	//time.Sleep(1 * time.Second)
	transactionDeletionState := map[string]bool{"tx1": true, "tx2": false, "tx3": false}
	tf.RequireTransactionsDeleted(transactionDeletionState)

	attachmentDeletionState := map[string]bool{"block1": true, "block2": false, "block3": false}
	tf.RequireAttachmentsDeleted(attachmentDeletionState)

	tf.Instance.Evict(1)

	tf.RequireAccepted(map[string]bool{"tx2": true, "tx3": false})
	tf.RequireBooked("tx3")

	tx2Metadata.SetCommitted()
	//time.Sleep(1 * time.Second)
	tf.RequireTransactionsDeleted(lo.MergeMaps(transactionDeletionState, map[string]bool{"tx2": true}))
	tf.RequireAttachmentsDeleted(lo.MergeMaps(attachmentDeletionState, map[string]bool{"block2": true}))

	tf.Instance.Evict(2)
	tf.RequireBooked("tx3")

	require.True(t, tf.MarkAttachmentIncluded("block3"))
	tf.RequireAccepted(map[string]bool{"tx3": true})

	tx3Metadata.SetCommitted()
	//time.Sleep(1 * time.Second)
	tf.RequireTransactionsDeleted(lo.MergeMaps(transactionDeletionState, map[string]bool{"tx3": true}))

	require.False(t, tx1Metadata.Inclusion().IsOrphaned())
	require.False(t, tx2Metadata.Inclusion().IsOrphaned())
	require.False(t, tx3Metadata.Inclusion().IsOrphaned())

	tf.RequireAttachmentsDeleted(lo.MergeMaps(attachmentDeletionState, map[string]bool{"block3": true}))

}

func TestSetTxOrphanage(t *testing.T, tf *TestFramework) {
	debug.SetEnabled(true)
	defer debug.SetEnabled(false)
	tf.CreateTransaction("tx1", []string{"genesis"}, 1)
	tf.CreateTransaction("tx2", []string{"tx1:0"}, 1)
	tf.CreateTransaction("tx3", []string{"tx2:0"}, 1)

	require.NoError(t, tf.AttachTransaction("tx3", "block3", 3))
	require.NoError(t, tf.AttachTransaction("tx2", "block2", 2))
	require.NoError(t, tf.AttachTransaction("tx1", "block1", 1))
	tf.RequireBooked("tx1", "tx2", "tx3")

	tx1Metadata, exists := tf.TransactionMetadata("tx1")
	require.True(t, exists)

	tx2Metadata, exists := tf.TransactionMetadata("tx2")
	require.True(t, exists)

	tx3Metadata, exists := tf.TransactionMetadata("tx3")
	require.True(t, exists)

	require.True(t, tf.MarkAttachmentIncluded("block2"))
	require.True(t, tf.MarkAttachmentIncluded("block3"))

	require.False(t, tx2Metadata.Inclusion().IsAccepted())
	require.False(t, tx3Metadata.Inclusion().IsAccepted())

	tf.Instance.Evict(1)

	tf.RequireTransactionsDeleted(map[string]bool{"tx1": true, "tx2": true, "tx3": true})

	require.True(t, tx1Metadata.Inclusion().IsOrphaned())
	require.True(t, tx2Metadata.Inclusion().IsOrphaned())
	require.True(t, tx3Metadata.Inclusion().IsOrphaned())

	tf.RequireAttachmentsDeleted(map[string]bool{"block1": true, "block2": true, "block3": true})
}

func TestSetTxOrphanageMultipleAttachments(t *testing.T, tf *TestFramework) {
	debug.SetEnabled(true)
	defer debug.SetEnabled(false)
	tf.CreateTransaction("tx1", []string{"genesis"}, 1)
	tf.CreateTransaction("tx2", []string{"tx1:0"}, 1)
	tf.CreateTransaction("tx3", []string{"tx2:0"}, 1)

	require.NoError(t, tf.AttachTransaction("tx3", "block3", 4))
	require.NoError(t, tf.AttachTransaction("tx2", "block2", 3))
	require.NoError(t, tf.AttachTransaction("tx1", "block1.1", 1))
	require.NoError(t, tf.AttachTransaction("tx1", "block1.2", 2))

	tf.RequireBooked("tx1", "tx2", "tx3")

	tx1Metadata, exists := tf.TransactionMetadata("tx1")
	require.True(t, exists)

	tx2Metadata, exists := tf.TransactionMetadata("tx2")
	require.True(t, exists)

	tx3Metadata, exists := tf.TransactionMetadata("tx3")
	require.True(t, exists)

	require.True(t, tf.MarkAttachmentIncluded("block2"))

	require.True(t, tf.MarkAttachmentIncluded("block3"))

	require.False(t, tx2Metadata.Inclusion().IsAccepted())
	require.False(t, tx3Metadata.Inclusion().IsAccepted())

	tf.Instance.Evict(1)

	require.False(t, tx1Metadata.Inclusion().IsOrphaned())
	require.False(t, tx2Metadata.Inclusion().IsOrphaned())
	require.False(t, tx3Metadata.Inclusion().IsOrphaned())

	tf.Instance.Evict(2)

	require.True(t, tx1Metadata.Inclusion().IsOrphaned())
	require.True(t, tx2Metadata.Inclusion().IsOrphaned())
	require.True(t, tx3Metadata.Inclusion().IsOrphaned())

	tf.RequireTransactionsDeleted(map[string]bool{"tx1": true, "tx2": true, "tx3": true})

	tf.RequireAttachmentsDeleted(map[string]bool{"block1.1": true, "block1.2": true, "block2": true, "block3": true})
}

func TestStateDiff(t *testing.T, tf *TestFramework) {
	debug.SetEnabled(true)
	defer debug.SetEnabled(false)
	tf.CreateTransaction("tx1", []string{"genesis"}, 1)
	tf.CreateTransaction("tx2", []string{"tx1:0"}, 1)
	tf.CreateTransaction("tx3", []string{"tx2:0"}, 1)

	require.NoError(t, tf.AttachTransaction("tx3", "block3", 1))
	require.NoError(t, tf.AttachTransaction("tx2", "block2", 1))
	require.NoError(t, tf.AttachTransaction("tx1", "block1", 1))

	tf.RequireBooked("tx1", "tx2", "tx3")

	acceptanceState := map[string]bool{}

	require.True(t, tf.MarkAttachmentIncluded("block1"))

	tf.RequireAccepted(lo.MergeMaps(acceptanceState, map[string]bool{"tx1": true, "tx2": false, "tx3": false}))
	tf.AssertStateDiff(1, []string{"genesis"}, []string{"tx1:0"}, []string{"tx1"})

	require.True(t, tf.MarkAttachmentIncluded("block2"))

	tf.RequireAccepted(lo.MergeMaps(acceptanceState, map[string]bool{"tx2": true}))
	tf.AssertStateDiff(1, []string{"genesis"}, []string{"tx2:0"}, []string{"tx1", "tx2"})

	require.True(t, tf.MarkAttachmentIncluded("block3"))

	tf.RequireAccepted(lo.MergeMaps(acceptanceState, map[string]bool{"tx3": true}))
	tf.AssertStateDiff(1, []string{"genesis"}, []string{"tx3:0"}, []string{"tx1", "tx2", "tx3"})
}
