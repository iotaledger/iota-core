package mempooltests

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/debug"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	TestMemoryReleaseMaxMemoryIncreaseFactor = 1.20
)

func TestAll(t *testing.T, frameworkProvider func(*testing.T) *TestFramework) {
	for testName, testCase := range map[string]func(*testing.T, *TestFramework){
		"TestSpendPropagation":                     TestSpendPropagation,
		"TestSetTxOrphanageMultipleAttachments":    TestSetTxOrphanageMultipleAttachments,
		"TestProcessTransactionWithReadOnlyInputs": TestProcessTransactionWithReadOnlyInputs,
		"TestProcessTransaction":                   TestProcessTransaction,
		"TestProcessTransactionsOutOfOrder":        TestProcessTransactionsOutOfOrder,
		"TestSetTransactionOrphanage":              TestSetTransactionOrphanage,
		"TestInvalidTransaction":                   TestInvalidTransaction,
		"TestStoreAttachmentInEvictedSlot":         TestStoreAttachmentInEvictedSlot,
	} {
		t.Run(testName, func(t *testing.T) { testCase(t, frameworkProvider(t)) })
	}
}

func TestProcessTransaction(t *testing.T, tf *TestFramework) {
	tf.CreateTransaction("tx1", []string{"genesis"}, 1)
	tf.CreateTransaction("tx2", []string{"tx1:0"}, 1)

	tf.SignedTransactionFromTransaction("tx2", "tx2")
	tf.SignedTransactionFromTransaction("tx1", "tx1")

	require.NoError(t, tf.AttachTransactions("tx1", "tx2"))

	tf.RequireBooked("tx1", "tx2")

	tx1Metadata, exists := tf.TransactionMetadata("tx1")
	require.True(t, exists)

	_ = tx1Metadata.Outputs().ForEach(func(state mempool.StateMetadata) error {
		require.False(t, state.IsAccepted())
		require.Equal(t, 1, state.PendingSpenderCount())

		return nil
	})

	tx2Metadata, exists := tf.TransactionMetadata("tx2")
	require.True(t, exists)

	_ = tx2Metadata.Outputs().ForEach(func(state mempool.StateMetadata) error {
		require.False(t, state.IsAccepted())
		require.Equal(t, 0, state.PendingSpenderCount())

		return nil
	})
}

func TestProcessTransactionWithReadOnlyInputs(t *testing.T, tf *TestFramework) {
	tf.InjectState("readOnlyInput", mempool.CommitmentInputStateFromCommitment(&iotago.Commitment{
		ProtocolVersion:      0,
		Slot:                 0,
		PreviousCommitmentID: iotago.CommitmentID{},
		RootsID:              iotago.Identifier{},
		CumulativeWeight:     0,
		ReferenceManaCost:    0,
	}))

	tf.CreateTransaction("tx1", []string{"genesis", "readOnlyInput"}, 1)
	tf.CreateTransaction("tx2", []string{"tx1:0", "readOnlyInput"}, 1)

	tf.SignedTransactionFromTransaction("tx2", "tx2")
	tf.SignedTransactionFromTransaction("tx1", "tx1")

	require.NoError(t, tf.AttachTransactions("tx1", "tx2"))

	tf.RequireBooked("tx1", "tx2")

	tx1Metadata, exists := tf.TransactionMetadata("tx1")
	require.True(t, exists)
	_ = tx1Metadata.Outputs().ForEach(func(state mempool.StateMetadata) error {
		if state.State().Type() == mempool.StateTypeUTXOInput {
			require.False(t, state.IsAccepted())
			require.Equal(t, 1, state.PendingSpenderCount())
		}

		return nil
	})

	tx2Metadata, exists := tf.TransactionMetadata("tx2")
	require.True(t, exists)

	_ = tx2Metadata.Outputs().ForEach(func(state mempool.StateMetadata) error {
		if state.State().Type() == mempool.StateTypeUTXOInput {
			require.False(t, state.IsAccepted())
			require.Equal(t, 0, state.PendingSpenderCount())
		}

		if state.State().Type() == mempool.StateTypeCommitment {
			require.False(t, state.IsAccepted())
			require.Equal(t, 2, state.PendingSpenderCount())
		}

		return nil
	})

	spendSetsTx1, exists := tf.SpendDAG.SpendSets(tf.TransactionID("tx1"))
	require.True(t, exists)
	require.Equal(t, 1, spendSetsTx1.Size())
	require.True(t, spendSetsTx1.Has(tf.StateID("genesis")))

	spendSetsTx2, exists := tf.SpendDAG.SpendSets(tf.TransactionID("tx2"))
	require.True(t, exists)
	require.Equal(t, 1, spendSetsTx2.Size())
	require.True(t, spendSetsTx2.Has(tf.StateID("tx1:0")))
}

func TestProcessTransactionsOutOfOrder(t *testing.T, tf *TestFramework) {
	tf.CreateSignedTransaction("tx1", []string{"genesis"}, 1)
	tf.CreateSignedTransaction("tx2", []string{"tx1:0"}, 1)
	tf.CreateSignedTransaction("tx3", []string{"tx2:0"}, 1)

	require.NoError(t, tf.AttachTransaction("tx3-signed", "tx3", "tx3", 1))
	require.NoError(t, tf.AttachTransaction("tx2-signed", "tx2", "tx2", 1))
	require.NoError(t, tf.AttachTransaction("tx1-signed", "tx1", "tx1", 1))

	tf.RequireBooked("tx1", "tx2", "tx3")

	tx1Metadata, exists := tf.TransactionMetadata("tx1")
	require.True(t, exists)

	_ = tx1Metadata.Outputs().ForEach(func(state mempool.StateMetadata) error {
		require.False(t, state.IsAccepted())
		require.Equal(t, 1, state.PendingSpenderCount())

		return nil
	})

	tx2Metadata, exists := tf.TransactionMetadata("tx2")
	require.True(t, exists)

	_ = tx2Metadata.Outputs().ForEach(func(state mempool.StateMetadata) error {
		require.False(t, state.IsAccepted())
		require.Equal(t, 1, state.PendingSpenderCount())

		return nil
	})

	tx3Metadata, exists := tf.TransactionMetadata("tx3")
	require.True(t, exists)

	_ = tx3Metadata.Outputs().ForEach(func(state mempool.StateMetadata) error {
		require.False(t, state.IsAccepted())
		require.Equal(t, 0, state.PendingSpenderCount())

		return nil
	})
}

func TestSetTransactionOrphanage(t *testing.T, tf *TestFramework) {
	debug.SetEnabled(true)
	defer debug.SetEnabled(false)
	tf.CreateSignedTransaction("tx1", []string{"genesis"}, 1)
	tf.CreateSignedTransaction("tx2", []string{"tx1:0"}, 1)
	tf.CreateSignedTransaction("tx3", []string{"tx2:0"}, 1)

	require.NoError(t, tf.AttachTransaction("tx3-signed", "tx3", "block3", 3))
	require.NoError(t, tf.AttachTransaction("tx2-signed", "tx2", "block2", 2))
	require.NoError(t, tf.AttachTransaction("tx1-signed", "tx1", "block1", 1))
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

	// We only evict after MCA
	tf.RequireTransactionsEvicted(map[string]bool{"tx1": false, "tx2": false, "tx3": false})

	require.True(t, lo.Return2(tx1Metadata.OrphanedSlot()))
	require.True(t, tx2Metadata.IsPending())
	require.True(t, tx3Metadata.IsPending())

	tf.RequireAttachmentsEvicted(map[string]bool{"block1": true, "block2": false, "block3": false})
}

func TestSetTxOrphanageMultipleAttachments(t *testing.T, tf *TestFramework) {
	debug.SetEnabled(true)
	defer debug.SetEnabled(false)
	tf.CreateSignedTransaction("tx1", []string{"genesis"}, 1)
	tf.CreateSignedTransaction("tx2", []string{"tx1:0"}, 1)
	tf.CreateSignedTransaction("tx3", []string{"tx2:0"}, 1)

	require.NoError(t, tf.AttachTransaction("tx3-signed", "tx3", "block3", 4))
	require.NoError(t, tf.AttachTransaction("tx2-signed", "tx2", "block2", 3))
	require.NoError(t, tf.AttachTransaction("tx1-signed", "tx1", "block1.1", 1))
	require.NoError(t, tf.AttachTransaction("tx1-signed", "tx1", "block1.2", 2))

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

	tf.CommitSlot(1)

	require.False(t, lo.Return2(tx1Metadata.OrphanedSlot()))
	require.False(t, lo.Return2(tx2Metadata.OrphanedSlot()))
	require.False(t, lo.Return2(tx3Metadata.OrphanedSlot()))

	require.True(t, lo.Return2(tf.SpendDAG.SpendSets(tf.TransactionID("tx1"))))
	require.True(t, lo.Return2(tf.SpendDAG.SpendSets(tf.TransactionID("tx2"))))
	require.True(t, lo.Return2(tf.SpendDAG.SpendSets(tf.TransactionID("tx3"))))

	tf.CommitSlot(2)

	require.True(t, lo.Return2(tx1Metadata.OrphanedSlot()))
	require.True(t, lo.Return2(tx2Metadata.OrphanedSlot()))
	require.True(t, lo.Return2(tx3Metadata.OrphanedSlot()))

	// All conflicts still exist, as they are kept around until MCA
	require.True(t, lo.Return2(tf.SpendDAG.SpendSets(tf.TransactionID("tx1"))))
	require.True(t, lo.Return2(tf.SpendDAG.SpendSets(tf.TransactionID("tx2"))))
	require.True(t, lo.Return2(tf.SpendDAG.SpendSets(tf.TransactionID("tx3"))))

	tf.RequireTransactionsEvicted(map[string]bool{"tx1": false, "tx2": false, "tx3": false})

	tf.RequireAttachmentsEvicted(map[string]bool{"block1.1": true, "block1.2": true, "block2": false, "block3": false})
}

func TestSpendPropagation(t *testing.T, tf *TestFramework) {
	debug.SetEnabled(true)
	defer debug.SetEnabled(false)
	tf.CreateSignedTransaction("tx1", []string{"genesis"}, 1)
	tf.CreateSignedTransaction("tx1*", []string{"genesis"}, 1)

	tf.CreateSignedTransaction("tx2", []string{"tx1:0"}, 1)
	tf.CreateSignedTransaction("tx2*", []string{"tx1*:0"}, 1)
	tf.CreateSignedTransaction("tx3", []string{"tx2:0"}, 1)
	tf.CreateSignedTransaction("tx3*", []string{"tx2*:0"}, 1)
	tf.CreateSignedTransaction("tx4", []string{"tx1:0"}, 1)

	require.NoError(t, tf.AttachTransaction("tx3-signed", "tx3", "block3", 3))
	require.NoError(t, tf.AttachTransaction("tx2-signed", "tx2", "block2", 2))
	require.NoError(t, tf.AttachTransaction("tx1-signed", "tx1", "block1", 1))

	tf.RequireBooked("tx1", "tx2", "tx3")
	tf.RequireSpenderIDs(map[string][]string{"tx1": {"tx1"}, "tx2": {"tx2"}, "tx3": {"tx3"}})

	require.NoError(t, tf.AttachTransaction("tx3*-signed", "tx3*", "block3*", 3))
	require.NoError(t, tf.AttachTransaction("tx2*-signed", "tx2*", "block2*", 2))
	require.NoError(t, tf.AttachTransaction("tx1*-signed", "tx1*", "block1*", 1))

	tf.RequireBooked("tx1*", "tx2*", "tx3*")
	tf.RequireSpenderIDs(map[string][]string{"tx1": {"tx1"}, "tx2": {"tx2"}, "tx3": {"tx3"}, "tx1*": {"tx1*"}, "tx2*": {"tx2*"}, "tx3*": {"tx3*"}})

	require.NoError(t, tf.AttachTransaction("tx4-signed", "tx4", "block4", 2))

	tf.RequireBooked("tx4")
	tf.RequireSpenderIDs(map[string][]string{"tx1": {"tx1"}, "tx2": {"tx2"}, "tx3": {"tx3"}, "tx4": {"tx4"}, "tx1*": {"tx1*"}, "tx2*": {"tx2*"}, "tx3*": {"tx3*"}})
}

func TestInvalidTransaction(t *testing.T, tf *TestFramework) {
	debug.SetEnabled(true)
	defer debug.SetEnabled(false)

	tf.CreateSignedTransaction("tx1", []string{"genesis"}, 1, true)
	require.NoError(t, tf.AttachTransaction("tx1-signed", "tx1", "block2", 1))

	tf.RequireInvalid("tx1")
}

func TestStoreAttachmentInEvictedSlot(t *testing.T, tf *TestFramework) {
	debug.SetEnabled(true)
	defer debug.SetEnabled(false)

	tf.CommitSlot(5)

	tf.CreateSignedTransaction("tx1", []string{"genesis"}, 1, true)
	require.Error(t, tf.AttachTransaction("tx1-signed", "tx1", "block2", 1))

	require.False(t, lo.Return2(tf.TransactionMetadata("tx1")))
}
