package tests

import (
	"testing"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/blockissuer"
	"github.com/iotaledger/iota-core/pkg/core/acceptance"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	iotago "github.com/iotaledger/iota.go/v4"
)

func Test_IssuingTransactionsOutOfOrder(t *testing.T) {
	ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1", 1)
	ts.Run(map[string][]options.Option[protocol.Protocol]{})

	node1.HookLogging()

	tx1 := ts.CreateTransaction("Tx1", 1, "Genesis")

	tx2 := ts.CreateTransaction("Tx2", 1, "Tx1:0")

	ts.IssueBlock("block1", node1, blockissuer.WithPayload(tx2))

	ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("Tx2"), true, node1)
	ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("Tx1"), false, node1)

	ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.Transactions("Tx2"), false, node1)
	// make sure that the block is not booked

	ts.IssueBlock("block2", node1, blockissuer.WithPayload(tx1))

	ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("Tx1", "Tx2"), true, node1)
	ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.Transactions("Tx1", "Tx2"), true, node1)
	ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
		ts.Block("block1"): {"Tx2"},
		ts.Block("block2"): {"Tx1"},
	}, node1)

	ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
		ts.TransactionFramework.Transaction("Tx2"): {"Tx2"},
		ts.TransactionFramework.Transaction("Tx1"): {"Tx1"},
	}, node1)
}

func Test_DoubleSpend(t *testing.T) {
	ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1", 1)
	node2 := ts.AddValidatorNode("node2", 1)

	ts.Run(map[string][]options.Option[protocol.Protocol]{})

	node1.HookLogging()

	// Create and issue double spends
	{
		tx1 := ts.CreateTransaction("Tx1", 1, "Genesis")
		tx2 := ts.CreateTransaction("Tx2", 1, "Genesis")

		ts.IssueBlock("block1", node1, blockissuer.WithPayload(tx1))
		ts.IssueBlock("block2", node1, blockissuer.WithPayload(tx2))

		ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("Tx1", "Tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.Transactions("Tx1", "Tx2"), true, node1, node2)
		ts.AssertTransactionsInCachePending(ts.TransactionFramework.Transactions("Tx1", "Tx2"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block1"): {"Tx1"},
			ts.Block("block2"): {"Tx2"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			ts.TransactionFramework.Transaction("Tx2"): {"Tx2"},
			ts.TransactionFramework.Transaction("Tx1"): {"Tx1"},
		}, node1, node2)
	}

	// Issue some more blocks and assert that conflicts are propagated to blocks.
	{
		ts.IssueBlock("block3", node1, blockissuer.WithStrongParents(ts.BlockID("block1")))
		ts.IssueBlock("block4", node1, blockissuer.WithStrongParents(ts.BlockID("block2")))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block3"): {"Tx1"},
			ts.Block("block4"): {"Tx2"},
		}, node1, node2)
		ts.AssertTransactionsInCachePending(ts.TransactionFramework.Transactions("Tx1", "Tx2"), true, node1, node2)
	}

	// Issue an invalid block and assert that its vote is not cast.
	{
		ts.IssueBlock("block5", node2, blockissuer.WithStrongParents(ts.BlockIDs("block3", "block4")...))

		ts.AssertTransactionsInCachePending(ts.TransactionFramework.Transactions("Tx1", "Tx2"), true, node1, node2)
	}

	// Issue a valid block that resolves the conflict.
	{
		ts.IssueBlock("block6", node2, blockissuer.WithStrongParents(ts.BlockIDs("block3", "block4")...), blockissuer.WithShallowLikeParents(ts.BlockID("block2")))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block6"): {"Tx2"},
		}, node1, node2)
		ts.AssertTransactionsInCacheAccepted(ts.TransactionFramework.Transactions("Tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheRejected(ts.TransactionFramework.Transactions("Tx1"), true, node1, node2)

	}
}

func Test_MultipleAttachments(t *testing.T) {
	ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1", 1)
	node2 := ts.AddValidatorNode("node2", 1)

	ts.Run(map[string][]options.Option[protocol.Protocol]{})

	node1.HookLogging()

	//Create a transaction and issue it from both nodes, so that the conflict is accepted, but none attachment is not included yet.
	{
		tx1 := ts.CreateTransaction("Tx1", 2, "Genesis")

		ts.IssueBlock("block1", node1, blockissuer.WithPayload(tx1), blockissuer.WithStrongParents(ts.BlockID("Genesis")))
		ts.IssueBlock("block2", node2, blockissuer.WithPayload(tx1), blockissuer.WithStrongParents(ts.BlockID("Genesis")))

		ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("Tx1"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.Transactions("Tx1"), true, node1, node2)
		ts.AssertTransactionsInCachePending(ts.TransactionFramework.Transactions("Tx1"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block1"): {"Tx1"},
			ts.Block("block2"): {"Tx1"},
		}, node1, node2)
		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			ts.TransactionFramework.Transaction("Tx1"): {"Tx1"},
		}, node1, node2)
		ts.AssertConflictsInCacheAcceptanceState([]string{"Tx1"}, acceptance.Accepted, node1, node2)
	}

	//Create a transaction that is included and whose conflict is accepted, but whose inputs are not accepted.
	{
		tx2 := ts.CreateTransaction("Tx2", 1, "Tx1:1")

		ts.IssueBlock("block3", node1, blockissuer.WithPayload(tx2), blockissuer.WithStrongParents(ts.BlockID("Genesis")))
		ts.IssueBlock("block4", node2, blockissuer.WithStrongParents(ts.BlockID("block3")))

		ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("Tx1", "Tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.Transactions("Tx1", "Tx2"), true, node1, node2)
		ts.AssertTransactionsInCachePending(ts.TransactionFramework.Transactions("Tx1", "Tx2"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block1"): {"Tx1"},
			ts.Block("block2"): {"Tx1"},
			ts.Block("block3"): {"Tx2"},
			ts.Block("block4"): {"Tx2"},
		}, node1, node2)
		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			ts.TransactionFramework.Transaction("Tx1"): {"Tx1"},
			ts.TransactionFramework.Transaction("Tx2"): {"Tx2"},
		}, node1, node2)
		ts.AssertConflictsInCacheAcceptanceState([]string{"Tx1", "Tx2"}, acceptance.Accepted, node1, node2)
	}

	//Issue a block that includes Tx1, and make sure that Tx2 is accepted as well as a consequence.
	{
		ts.IssueBlock("block5", node2, blockissuer.WithStrongParents(ts.BlockID("block1")))

		ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("Tx1", "Tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.Transactions("Tx1", "Tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheAccepted(ts.TransactionFramework.Transactions("Tx1", "Tx2"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block1"): {"Tx1"},
			ts.Block("block2"): {"Tx1"},
			ts.Block("block3"): {"Tx2"},
			ts.Block("block4"): {"Tx2"},
			ts.Block("block5"): {},
		}, node1, node2)
		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			ts.TransactionFramework.Transaction("Tx1"): {"Tx1"},
			ts.TransactionFramework.Transaction("Tx2"): {"Tx2"},
		}, node1, node2)
		ts.AssertConflictsInCacheAcceptanceState([]string{"Tx1", "Tx2"}, acceptance.Accepted, node1, node2)
	}
}
