package tests

import (
	"testing"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/blockfactory"
	"github.com/iotaledger/iota-core/pkg/core/acceptance"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	iotago "github.com/iotaledger/iota.go/v4"
)

func Test_IssuingTransactionsOutOfOrder(t *testing.T) {
	ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	tx1 := lo.PanicOnErr(ts.TransactionFramework.CreateSimpleTransaction("tx1", 1, "Genesis:0"))

	tx2 := lo.PanicOnErr(ts.TransactionFramework.CreateSimpleTransaction("tx2", 1, "tx1:0"))

	ts.IssuePayloadWithOptions("block1", node1, tx2)

	ts.AssertTransactionsExist(ts.TransactionFramework.SignedTransactions("tx2"), true, node1)
	ts.AssertTransactionsExist(ts.TransactionFramework.SignedTransactions("tx1"), false, node1)

	ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.SignedTransactions("tx2"), false, node1)
	// make sure that the block is not booked

	ts.IssuePayloadWithOptions("block2", node1, tx1)

	ts.AssertTransactionsExist(ts.TransactionFramework.SignedTransactions("tx1", "tx2"), true, node1)
	ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.SignedTransactions("tx1", "tx2"), true, node1)
	ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
		ts.Block("block1"): {"tx2"},
		ts.Block("block2"): {"tx1"},
	}, node1)

	ts.AssertTransactionInCacheConflicts(map[*iotago.SignedTransaction][]string{
		ts.TransactionFramework.SignedTransaction("tx2"): {"tx2"},
		ts.TransactionFramework.SignedTransaction("tx1"): {"tx1"},
	}, node1)
}

func Test_DoubleSpend(t *testing.T) {
	ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddValidatorNode("node2")

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		node1.AccountID,
		node2.AccountID,
	}, ts.Nodes()...)

	// Create and issue double spends
	{
		tx1 := lo.PanicOnErr(ts.TransactionFramework.CreateSimpleTransaction("tx1", 1, "Genesis:0"))
		tx2 := lo.PanicOnErr(ts.TransactionFramework.CreateSimpleTransaction("tx2", 1, "Genesis:0"))

		ts.IssuePayloadWithOptions("block1", node1, tx1, blockfactory.WithStrongParents(ts.BlockID("Genesis")))
		ts.IssuePayloadWithOptions("block2", node1, tx2, blockfactory.WithStrongParents(ts.BlockID("Genesis")))

		ts.AssertTransactionsExist(ts.TransactionFramework.SignedTransactions("tx1", "tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.SignedTransactions("tx1", "tx2"), true, node1, node2)
		ts.AssertTransactionsInCachePending(ts.TransactionFramework.SignedTransactions("tx1", "tx2"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block1"): {"tx1"},
			ts.Block("block2"): {"tx2"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.SignedTransaction][]string{
			ts.TransactionFramework.SignedTransaction("tx2"): {"tx2"},
			ts.TransactionFramework.SignedTransaction("tx1"): {"tx1"},
		}, node1, node2)
	}

	// Issue some more blocks and assert that conflicts are propagated to blocks.
	{
		ts.IssueValidationBlock("block3", node1, blockfactory.WithStrongParents(ts.BlockID("block1")))
		ts.IssueValidationBlock("block4", node1, blockfactory.WithStrongParents(ts.BlockID("block2")))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block3"): {"tx1"},
			ts.Block("block4"): {"tx2"},
		}, node1, node2)
		ts.AssertTransactionsInCachePending(ts.TransactionFramework.SignedTransactions("tx1", "tx2"), true, node1, node2)
	}

	// Issue an invalid block and assert that its vote is not cast.
	{
		ts.IssueValidationBlock("block5", node2, blockfactory.WithStrongParents(ts.BlockIDs("block3", "block4")...))

		ts.AssertTransactionsInCachePending(ts.TransactionFramework.SignedTransactions("tx1", "tx2"), true, node1, node2)
	}

	// Issue valid blocks that resolve the conflict.
	{
		ts.IssueValidationBlock("block6", node2, blockfactory.WithStrongParents(ts.BlockIDs("block3", "block4")...), blockfactory.WithShallowLikeParents(ts.BlockID("block2")))
		ts.IssueValidationBlock("block7", node1, blockfactory.WithStrongParents(ts.BlockIDs("block6")...))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block6"): {"tx2"},
		}, node1, node2)
		ts.AssertTransactionsInCacheAccepted(ts.TransactionFramework.SignedTransactions("tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheRejected(ts.TransactionFramework.SignedTransactions("tx1"), true, node1, node2)

	}
}

func Test_MultipleAttachments(t *testing.T) {
	ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	nodeA := ts.AddValidatorNode("nodeA")
	nodeB := ts.AddValidatorNode("nodeB")

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	blocksConflicts := make(map[*blocks.Block][]string)

	// Create a transaction and issue it from both nodes, so that the conflict is accepted, but no attachment is included yet.
	{
		tx1 := lo.PanicOnErr(ts.TransactionFramework.CreateSimpleTransaction("tx1", 2, "Genesis:0"))

		ts.IssuePayloadWithOptions("A.1", nodeA, tx1, blockfactory.WithStrongParents(ts.BlockID("Genesis")))
		ts.IssueValidationBlock("A.1.1", nodeA, blockfactory.WithStrongParents(ts.BlockID("A.1")))
		ts.IssuePayloadWithOptions("B.1", nodeB, tx1, blockfactory.WithStrongParents(ts.BlockID("Genesis")))
		ts.IssueValidationBlock("B.1.1", nodeB, blockfactory.WithStrongParents(ts.BlockID("B.1")))

		ts.IssueValidationBlock("A.2", nodeA, blockfactory.WithStrongParents(ts.BlockID("B.1.1")))
		ts.IssueValidationBlock("B.2", nodeB, blockfactory.WithStrongParents(ts.BlockID("A.1.1")))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.1", "B.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("A.1", "B.1"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheConflicts(lo.MergeMaps(blocksConflicts, map[*blocks.Block][]string{
			ts.Block("A.1"): {"tx1"},
			ts.Block("B.1"): {"tx1"},
			ts.Block("A.2"): {"tx1"},
			ts.Block("B.2"): {"tx1"},
		}), ts.Nodes()...)
		ts.AssertTransactionInCacheConflicts(map[*iotago.SignedTransaction][]string{
			ts.TransactionFramework.SignedTransaction("tx1"): {"tx1"},
		}, ts.Nodes()...)
		ts.AssertConflictsInCacheAcceptanceState([]string{"tx1"}, acceptance.Accepted, ts.Nodes()...)
	}

	// Create a transaction that is included and whose conflict is accepted, but whose inputs are not accepted.
	{
		tx2 := lo.PanicOnErr(ts.TransactionFramework.CreateSimpleTransaction("tx2", 1, "tx1:1"))

		ts.IssuePayloadWithOptions("A.3", nodeA, tx2, blockfactory.WithStrongParents(ts.BlockID("Genesis")))
		ts.IssueValidationBlock("B.3", nodeB, blockfactory.WithStrongParents(ts.BlockID("A.3")))
		ts.IssueValidationBlock("A.4", nodeA, blockfactory.WithStrongParents(ts.BlockID("B.3")))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.3"), true, ts.Nodes()...)

		ts.IssueValidationBlock("B.4", nodeB, blockfactory.WithStrongParents(ts.BlockIDs("B.3", "A.4")...))
		ts.IssueValidationBlock("A.5", nodeA, blockfactory.WithStrongParents(ts.BlockIDs("B.3", "A.4")...))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("B.3", "A.4"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("B.4", "A.5"), false, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("A.3"), true, ts.Nodes()...)

		ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.SignedTransactions("tx1", "tx2"), true, ts.Nodes()...)
		ts.AssertTransactionsInCachePending(ts.TransactionFramework.SignedTransactions("tx1", "tx2"), true, ts.Nodes()...)

		ts.AssertBlocksInCacheConflicts(lo.MergeMaps(blocksConflicts, map[*blocks.Block][]string{
			ts.Block("A.3"): {"tx2"},
			ts.Block("B.3"): {"tx2"},
			ts.Block("A.4"): {"tx2"},
			ts.Block("A.5"): {},
			ts.Block("B.4"): {},
		}), ts.Nodes()...)
		ts.AssertTransactionInCacheConflicts(map[*iotago.SignedTransaction][]string{
			ts.TransactionFramework.SignedTransaction("tx1"): {"tx1"},
			ts.TransactionFramework.SignedTransaction("tx2"): {"tx2"},
		}, nodeA, nodeB)
		ts.AssertConflictsInCacheAcceptanceState([]string{"tx1", "tx2"}, acceptance.Accepted, ts.Nodes()...)
	}

	// Issue a block that includes tx1, and make sure that tx2 is accepted as well as a consequence.
	{
		ts.IssueValidationBlock("A.6", nodeA, blockfactory.WithStrongParents(ts.BlockIDs("A.2", "B.2")...))
		ts.IssueValidationBlock("B.5", nodeB, blockfactory.WithStrongParents(ts.BlockIDs("A.2", "B.2")...))

		ts.IssueValidationBlock("A.7", nodeA, blockfactory.WithStrongParents(ts.BlockIDs("A.6", "B.5")...))
		ts.IssueValidationBlock("B.6", nodeB, blockfactory.WithStrongParents(ts.BlockIDs("A.6", "B.5")...))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.2", "B.2", "A.6", "B.5"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("A.1", "B.1"), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.7", "B.6"), false, ts.Nodes()...)
		ts.AssertTransactionsExist(ts.TransactionFramework.SignedTransactions("tx1", "tx2"), true, ts.Nodes()...)
		ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.SignedTransactions("tx1", "tx2"), true, ts.Nodes()...)
		ts.AssertTransactionsInCacheAccepted(ts.TransactionFramework.SignedTransactions("tx1", "tx2"), true, ts.Nodes()...)

		ts.AssertBlocksInCacheConflicts(lo.MergeMaps(blocksConflicts, map[*blocks.Block][]string{
			ts.Block("A.6"): {},
			ts.Block("B.5"): {},
			ts.Block("A.7"): {},
			ts.Block("B.6"): {},
		}), ts.Nodes()...)

		ts.AssertTransactionInCacheConflicts(map[*iotago.SignedTransaction][]string{
			ts.TransactionFramework.SignedTransaction("tx1"): {"tx1"},
			ts.TransactionFramework.SignedTransaction("tx2"): {"tx2"},
		}, nodeA, nodeB)
		ts.AssertConflictsInCacheAcceptanceState([]string{"tx1", "tx2"}, acceptance.Accepted, nodeA, nodeB)
	}
}
