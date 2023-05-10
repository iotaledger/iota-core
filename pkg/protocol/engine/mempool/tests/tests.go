package mempooltests

import (
	"fmt"
	"runtime"
	memleakdebug "runtime/debug"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/debug"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestAll(t *testing.T, frameworkProvider func(*testing.T) *TestFramework) {
	for testName, testCase := range map[string]func(*testing.T, *TestFramework){
		"TestProcessTransaction":                TestProcessTransaction,
		"TestProcessTransactionsOutOfOrder":     TestProcessTransactionsOutOfOrder,
		"TestSetInclusionSlot":                  TestSetInclusionSlot,
		"TestSetTransactionOrphanage":           TestSetTransactionOrphanage,
		"TestSetAllAttachmentsOrphaned":         TestSetAllAttachmentsOrphaned,
		"TestSetTxOrphanageMultipleAttachments": TestSetTxOrphanageMultipleAttachments,
		"TestStateDiff":                         TestStateDiff,
		"TestMempool_MemLeak":                   TestMempool_MemLeak,
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
		require.False(t, state.IsAccepted())
		require.True(t, state.IsSpent())

		return nil
	})

	tx2Metadata, exists := tf.TransactionMetadata("tx2")
	require.True(t, exists)

	_ = tx2Metadata.Outputs().ForEach(func(state mempool.StateWithMetadata) error {
		require.False(t, state.IsAccepted())
		require.False(t, state.IsSpent())

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
		require.False(t, state.IsAccepted())
		require.True(t, state.IsSpent())

		return nil
	})

	tx2Metadata, exists := tf.TransactionMetadata("tx2")
	require.True(t, exists)

	_ = tx2Metadata.Outputs().ForEach(func(state mempool.StateWithMetadata) error {
		require.False(t, state.IsAccepted())
		require.True(t, state.IsSpent())

		return nil
	})

	tx3Metadata, exists := tf.TransactionMetadata("tx3")
	require.True(t, exists)

	_ = tx3Metadata.Outputs().ForEach(func(state mempool.StateWithMetadata) error {
		require.False(t, state.IsAccepted())
		require.False(t, state.IsSpent())

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

	tx1Metadata.Commit()
	//time.Sleep(1 * time.Second)
	transactionDeletionState := map[string]bool{"tx1": true, "tx2": false, "tx3": false}
	tf.RequireTransactionsEvicted(transactionDeletionState)

	attachmentDeletionState := map[string]bool{"block1": true, "block2": false, "block3": false}
	tf.RequireAttachmentsEvicted(attachmentDeletionState)

	tf.Instance.Evict(1)

	tf.RequireAccepted(map[string]bool{"tx2": true, "tx3": false})
	tf.RequireBooked("tx3")

	tx2Metadata.Commit()
	//time.Sleep(1 * time.Second)
	tf.RequireTransactionsEvicted(lo.MergeMaps(transactionDeletionState, map[string]bool{"tx2": true}))
	tf.RequireAttachmentsEvicted(lo.MergeMaps(attachmentDeletionState, map[string]bool{"block2": true}))

	tf.Instance.Evict(2)
	tf.RequireBooked("tx3")

	require.True(t, tf.MarkAttachmentIncluded("block3"))
	tf.RequireAccepted(map[string]bool{"tx3": true})

	tx3Metadata.Commit()
	//time.Sleep(1 * time.Second)
	tf.RequireTransactionsEvicted(lo.MergeMaps(transactionDeletionState, map[string]bool{"tx3": true}))

	require.False(t, tx1Metadata.IsOrphaned())
	require.False(t, tx2Metadata.IsOrphaned())
	require.False(t, tx3Metadata.IsOrphaned())

	tf.RequireAttachmentsEvicted(lo.MergeMaps(attachmentDeletionState, map[string]bool{"block3": true}))

}

func TestSetAllAttachmentsOrphaned(t *testing.T, tf *TestFramework) {
	debug.SetEnabled(true)
	defer debug.SetEnabled(false)
	tf.CreateTransaction("tx1", []string{"genesis"}, 1)

	require.NoError(t, tf.AttachTransaction("tx1", "block1.2", 2))
	require.NoError(t, tf.AttachTransaction("tx1", "block1.1", 1))

	tf.RequireBooked("tx1")

	tx1Metadata, exists := tf.TransactionMetadata("tx1")
	require.True(t, exists)

	require.EqualValues(t, 0, tx1Metadata.EarliestIncludedSlot())

	require.True(t, tf.MarkAttachmentIncluded("block1.2"))

	require.True(t, tx1Metadata.IsAccepted())
	require.EqualValues(t, 2, tx1Metadata.EarliestIncludedSlot())
	tf.AssertStateDiff(1, []string{}, []string{}, []string{})
	tf.AssertStateDiff(2, []string{"genesis"}, []string{"tx1:0"}, []string{"tx1"})

	require.True(t, tf.MarkAttachmentIncluded("block1.1"))

	require.True(t, tx1Metadata.IsAccepted())
	require.EqualValues(t, 1, tx1Metadata.EarliestIncludedSlot())
	tf.AssertStateDiff(1, []string{"genesis"}, []string{"tx1:0"}, []string{"tx1"})
	tf.AssertStateDiff(2, []string{}, []string{}, []string{})

	require.True(t, tf.MarkAttachmentOrphaned("block1.1"))

	require.True(t, tx1Metadata.IsAccepted())
	require.False(t, tx1Metadata.IsOrphaned())
	require.EqualValues(t, 2, tx1Metadata.EarliestIncludedSlot())
	tf.AssertStateDiff(1, []string{}, []string{}, []string{})
	tf.AssertStateDiff(2, []string{"genesis"}, []string{"tx1:0"}, []string{"tx1"})

	require.True(t, tf.MarkAttachmentOrphaned("block1.2"))

	require.True(t, tx1Metadata.IsOrphaned())
	require.True(t, tx1Metadata.IsAccepted())
	require.EqualValues(t, 0, tx1Metadata.EarliestIncludedSlot())

	tf.AssertStateDiff(1, []string{}, []string{}, []string{})
	tf.AssertStateDiff(2, []string{}, []string{}, []string{})

}

func TestSetNotAllAttachmentsOrphaned(t *testing.T, tf *TestFramework) {
	debug.SetEnabled(true)
	defer debug.SetEnabled(false)
	tf.CreateTransaction("tx1", []string{"genesis"}, 1)

	require.NoError(t, tf.AttachTransaction("tx1", "block1.6", 6))
	require.NoError(t, tf.AttachTransaction("tx1", "block1.5", 5))
	require.NoError(t, tf.AttachTransaction("tx1", "block1.4", 4))
	require.NoError(t, tf.AttachTransaction("tx1", "block1.3", 3))
	require.NoError(t, tf.AttachTransaction("tx1", "block1.2", 2))
	require.NoError(t, tf.AttachTransaction("tx1", "block1.1", 1))

	tf.RequireBooked("tx1")

	tx1Metadata, exists := tf.TransactionMetadata("tx1")
	require.True(t, exists)

	require.EqualValues(t, 0, tx1Metadata.EarliestIncludedSlot())

	require.True(t, tf.MarkAttachmentIncluded("block1.2"))

	tf.Instance.Evict(1)

	require.True(t, tx1Metadata.IsAccepted())
	require.False(t, tx1Metadata.IsOrphaned())
	require.EqualValues(t, 2, tx1Metadata.EarliestIncludedSlot())
	tf.AssertStateDiff(2, []string{"genesis"}, []string{"tx1:0"}, []string{"tx1"})

	require.True(t, tf.MarkAttachmentOrphaned("block1.2"))

	require.True(t, tx1Metadata.IsAccepted())
	require.False(t, tx1Metadata.IsOrphaned())
	require.EqualValues(t, 0, tx1Metadata.EarliestIncludedSlot())
	tf.AssertStateDiff(2, []string{}, []string{}, []string{})

	require.True(t, tf.MarkAttachmentIncluded("block1.4"))

	require.True(t, tx1Metadata.IsAccepted())
	require.False(t, tx1Metadata.IsOrphaned())
	require.EqualValues(t, 4, tx1Metadata.EarliestIncludedSlot())
	tf.AssertStateDiff(4, []string{"genesis"}, []string{"tx1:0"}, []string{"tx1"})

	tf.Instance.Evict(2)
	tf.Instance.Evict(3)

	require.True(t, tx1Metadata.IsAccepted())
	require.False(t, tx1Metadata.IsOrphaned())
	require.EqualValues(t, 4, tx1Metadata.EarliestIncludedSlot())
	tf.AssertStateDiff(4, []string{"genesis"}, []string{"tx1:0"}, []string{"tx1"})

	tf.Instance.Evict(4)

	require.True(t, tf.MarkAttachmentIncluded("block1.5"))

	require.True(t, tx1Metadata.IsAccepted())
	require.False(t, tx1Metadata.IsOrphaned())
	require.EqualValues(t, 4, tx1Metadata.EarliestIncludedSlot())
	tf.AssertStateDiff(4, []string{}, []string{}, []string{})
	tf.AssertStateDiff(5, []string{}, []string{}, []string{})

}

func TestSetTransactionOrphanage(t *testing.T, tf *TestFramework) {
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

	require.False(t, tx2Metadata.IsAccepted())
	require.False(t, tx3Metadata.IsAccepted())

	tf.Instance.Evict(1)

	tf.RequireTransactionsEvicted(map[string]bool{"tx1": true, "tx2": true, "tx3": true})

	require.True(t, tx1Metadata.IsOrphaned())
	require.True(t, tx2Metadata.IsOrphaned())
	require.True(t, tx3Metadata.IsOrphaned())

	tf.RequireAttachmentsEvicted(map[string]bool{"block1": true, "block2": true, "block3": true})
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

	require.False(t, tx2Metadata.IsAccepted())
	require.False(t, tx3Metadata.IsAccepted())

	tf.Instance.Evict(1)

	require.False(t, tx1Metadata.IsOrphaned())
	require.False(t, tx2Metadata.IsOrphaned())
	require.False(t, tx3Metadata.IsOrphaned())

	tf.Instance.Evict(2)

	require.True(t, tx1Metadata.IsOrphaned())
	require.True(t, tx2Metadata.IsOrphaned())
	require.True(t, tx3Metadata.IsOrphaned())

	tf.RequireTransactionsEvicted(map[string]bool{"tx1": true, "tx2": true, "tx3": true})

	tf.RequireAttachmentsEvicted(map[string]bool{"block1.1": true, "block1.2": true, "block2": true, "block3": true})
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

func TestMempool_MemLeak(t *testing.T, tf *TestFramework) {
	prevStateAlias := "genesis"
	txIndex := 1
	issueTransactions := func(startIndex, transactionCount int, prevStateAlias string) (int, string) {
		index := startIndex
		for ; index < startIndex+transactionCount; index++ {
			txAlias := fmt.Sprintf("tx%d", index)
			blockAlias := fmt.Sprintf("block%d", index)
			tf.CreateTransaction(txAlias, []string{prevStateAlias}, 2)

			require.NoError(t, tf.AttachTransaction(txAlias, blockAlias, iotago.SlotIndex(index)))

			prevStateAlias = fmt.Sprintf("tx%d:0", index)

			tf.Instance.Evict(iotago.SlotIndex(index))
		}
		return index, prevStateAlias
	}

	memStatsStart := memStats()

	txIndex, prevStateAlias = issueTransactions(txIndex, 100000, prevStateAlias)

	time.Sleep(1 * time.Second)

	txIndex, prevStateAlias = issueTransactions(txIndex, 100000, prevStateAlias)

	tf.Cleanup()
	tf = nil
	memStatsEnd := memStats()

	fmt.Println(memStatsEnd.HeapObjects, memStatsStart.HeapObjects)

	assert.Less(t, float64(memStatsEnd.HeapObjects), 1.1*float64(memStatsStart.HeapObjects), "the objects in the heap should not grow by more than 10%")
}

func memStats() *runtime.MemStats {
	runtime.GC()
	memleakdebug.FreeOSMemory()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return &memStats
}
