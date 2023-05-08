package mempooltests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/runtime/debug"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
)

func TestAll(t *testing.T, frameworkProvider func(*testing.T) *TestFramework) {
	for testName, testCase := range map[string]func(*testing.T, *TestFramework){
		"TestProcessTransaction":            TestProcessTransaction,
		"TestProcessTransactionsOutOfOrder": TestProcessTransactionsOutOfOrder,
		"TestSetInclusionSlot":              TestSetInclusionSlot,
	} {
		t.Run(testName, func(t *testing.T) { testCase(t, frameworkProvider(t)) })
	}
}

func TestProcessTransaction(t *testing.T, tf *TestFramework) {
	tf.CreateTransaction("tx1", []string{"genesis"}, 1)
	tf.CreateTransaction("tx2", []string{"tx1:0"}, 1)

	// PENDING STATE
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

	tf.CreateTransaction("tx1", []string{"genesis"}, 1)
	tf.CreateTransaction("tx2", []string{"tx1:0"}, 1)
	tf.CreateTransaction("tx3", []string{"tx2:0"}, 1)

	require.NoError(t, tf.AttachTransaction("tx3", "block3", 3))
	require.NoError(t, tf.AttachTransaction("tx2", "block2", 2))
	require.NoError(t, tf.AttachTransaction("tx1", "block1", 1))

	tf.RequireBooked("tx1", "tx2", "tx3")

	require.NoError(t, tf.MarkAttachmentIncluded("block2"))

	require.NoError(t, tf.MarkAttachmentIncluded("block1"))
	tf.RequireAccepted("tx1", "tx2")

	// TODO: implement test that checks behavior of accepted, but not commited transactions during slot eviction once we define what to do then
	//tf.Instance.Evict(1)
	//time.Sleep(100 * time.Millisecond)
	//tf.RequireEvicted("tx1", "tx2", "tx3")

	tx1Metadata, exists := tf.TransactionMetadata("tx1")
	require.True(t, exists)

	tx2Metadata, exists := tf.TransactionMetadata("tx2")
	require.True(t, exists)

	//tx3Metadata, exists := tf.TransactionMetadata("tx3")
	//require.True(t, exists)

	tx1Metadata.SetCommitted()
	time.Sleep(1 * time.Second)
	tf.RequireDeleted("tx1")

	tf.Instance.Evict(1)
	tf.RequireAccepted("tx2")
	tf.RequireBooked("tx3")

	tx2Metadata.SetCommitted()
	time.Sleep(1 * time.Second)
	tf.RequireDeleted("tx1", "tx2")

	tf.Instance.Evict(2)
	tf.RequireBooked("tx3")

	// TODO: would be nice to make sure that all attachments are removed as well in the underlying structure

}
