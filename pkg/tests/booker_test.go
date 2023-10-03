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
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
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

	ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("tx2"), true, node1)
	ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("tx1"), false, node1)

	ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.Transactions("tx2"), false, node1)
	// make sure that the block is not booked

	ts.IssuePayloadWithOptions("block2", node1, tx1)

	ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("tx1", "tx2"), true, node1)
	ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.Transactions("tx1", "tx2"), true, node1)
	ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
		ts.Block("block1"): {"tx2"},
		ts.Block("block2"): {"tx1"},
	}, node1)

	ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
		ts.TransactionFramework.Transaction("tx2"): {"tx2"},
		ts.TransactionFramework.Transaction("tx1"): {"tx1"},
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

		ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertTransactionsInCachePending(ts.TransactionFramework.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block1"): {"tx1"},
			ts.Block("block2"): {"tx2"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			ts.TransactionFramework.Transaction("tx2"): {"tx2"},
			ts.TransactionFramework.Transaction("tx1"): {"tx1"},
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
		ts.AssertTransactionsInCachePending(ts.TransactionFramework.Transactions("tx1", "tx2"), true, node1, node2)
	}

	// Issue an invalid block and assert that its vote is not cast.
	{
		ts.IssueValidationBlock("block5", node2, blockfactory.WithStrongParents(ts.BlockIDs("block3", "block4")...))

		ts.AssertTransactionsInCachePending(ts.TransactionFramework.Transactions("tx1", "tx2"), true, node1, node2)
	}

	// Issue valid blocks that resolve the conflict.
	{
		ts.IssueValidationBlock("block6", node2, blockfactory.WithStrongParents(ts.BlockIDs("block3", "block4")...), blockfactory.WithShallowLikeParents(ts.BlockID("block2")))
		ts.IssueValidationBlock("block7", node1, blockfactory.WithStrongParents(ts.BlockIDs("block6")...))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block6"): {"tx2"},
		}, node1, node2)
		ts.AssertTransactionsInCacheAccepted(ts.TransactionFramework.Transactions("tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheRejected(ts.TransactionFramework.Transactions("tx1"), true, node1, node2)

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
		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			ts.TransactionFramework.Transaction("tx1"): {"tx1"},
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

		ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.Transactions("tx1", "tx2"), true, ts.Nodes()...)
		ts.AssertTransactionsInCachePending(ts.TransactionFramework.Transactions("tx1", "tx2"), true, ts.Nodes()...)

		ts.AssertBlocksInCacheConflicts(lo.MergeMaps(blocksConflicts, map[*blocks.Block][]string{
			ts.Block("A.3"): {"tx2"},
			ts.Block("B.3"): {"tx2"},
			ts.Block("A.4"): {"tx2"},
			ts.Block("A.5"): {},
			ts.Block("B.4"): {},
		}), ts.Nodes()...)
		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			ts.TransactionFramework.Transaction("tx1"): {"tx1"},
			ts.TransactionFramework.Transaction("tx2"): {"tx2"},
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
		ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("tx1", "tx2"), true, ts.Nodes()...)
		ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.Transactions("tx1", "tx2"), true, ts.Nodes()...)
		ts.AssertTransactionsInCacheAccepted(ts.TransactionFramework.Transactions("tx1", "tx2"), true, ts.Nodes()...)

		ts.AssertBlocksInCacheConflicts(lo.MergeMaps(blocksConflicts, map[*blocks.Block][]string{
			ts.Block("A.6"): {},
			ts.Block("B.5"): {},
			ts.Block("A.7"): {},
			ts.Block("B.6"): {},
		}), ts.Nodes()...)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			ts.TransactionFramework.Transaction("tx1"): {"tx1"},
			ts.TransactionFramework.Transaction("tx2"): {"tx2"},
		}, nodeA, nodeB)
		ts.AssertConflictsInCacheAcceptanceState([]string{"tx1", "tx2"}, acceptance.Accepted, nodeA, nodeB)
	}
}

func Test_SpendRejectedCommittedRace(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithGenesisTimestampOffset(20*10),
		testsuite.WithMinCommittableAge(2),
		testsuite.WithMaxCommittableAge(5),
	)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddValidatorNode("node2")

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		node1.AccountID,
		node2.AccountID,
	}, ts.Nodes()...)

	genesisCommitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(0)).Commitment()

	// Create and issue double spends
	{
		tx1 := lo.PanicOnErr(ts.TransactionFramework.CreateSimpleTransaction("tx1", 1, "Genesis:0"))
		tx2 := lo.PanicOnErr(ts.TransactionFramework.CreateSimpleTransaction("tx2", 1, "Genesis:0"))

		ts.IssueBlockAtSlotWithOptions("block1.1", 1, genesisCommitment, node1, tx1)
		ts.IssueBlockAtSlotWithOptions("block1.2", 1, genesisCommitment, node1, tx2)
		ts.IssueBlockAtSlot("block2.tx1", 2, genesisCommitment, node1, ts.BlockIDs("block1.1")...)

		ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertTransactionsInCachePending(ts.TransactionFramework.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block1.1"):   {"tx1"},
			ts.Block("block1.2"):   {"tx2"},
			ts.Block("block2.tx1"): {"tx1"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			ts.TransactionFramework.Transaction("tx2"): {"tx2"},
			ts.TransactionFramework.Transaction("tx1"): {"tx1"},
		}, node1, node2)
	}

	// Issue some more blocks and assert that conflicts are propagated to blocks.
	{
		ts.IssueBlockAtSlot("block2.1", 2, genesisCommitment, node1, ts.BlockID("block1.1"))
		ts.IssueBlockAtSlot("block2.2", 2, genesisCommitment, node1, ts.BlockID("block1.2"))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block2.1"):   {"tx1"},
			ts.Block("block2.2"):   {"tx2"},
			ts.Block("block2.tx1"): {"tx1"},
		}, node1, node2)
		ts.AssertTransactionsInCachePending(ts.TransactionFramework.Transactions("tx1", "tx2"), true, node1, node2)
	}

	// Issue valid blocks that resolve the conflict.
	{
		ts.IssueBlockAtSlot("block2.3", 2, genesisCommitment, node2, ts.BlockIDs("block2.2")...)
		ts.IssueBlockAtSlot("block2.4", 2, genesisCommitment, node1, ts.BlockIDs("block2.3")...)

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block2.3"):   {"tx2"},
			ts.Block("block2.tx1"): {"tx1"},
		}, node1, node2)
		ts.AssertTransactionsInCacheAccepted(ts.TransactionFramework.Transactions("tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheRejected(ts.TransactionFramework.Transactions("tx1"), true, node1, node2)
	}

	// Advance both nodes at the edge of slot 1 committability
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{2, 3, 4}, 1, "block2.4", ts.Nodes("node1", "node2"), false, nil)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(0),
			testsuite.WithEqualStoredCommitmentAtIndex(0),
			testsuite.WithEvictedSlot(0),
		)

		ts.IssueBlockAtSlot("block5.1", 5, genesisCommitment, node1, ts.BlockIDsWithPrefix("block1.1")...)
		ts.IssueBlockAtSlot("block5.2", 5, genesisCommitment, node1, ts.BlockIDsWithPrefix("block1.2")...)

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block5.1"):   {"tx1"}, // on rejected conflict
			ts.Block("block5.2"):   {},      // accepted merged-to-master
			ts.Block("block2.tx1"): {"tx1"},
		}, node1, node2)

		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{5}, 1, "4.0", ts.Nodes("node1"), false, nil)

		ts.AssertBlocksExist(ts.BlocksWithPrefix("5.0"), true, ts.Nodes()...)
	}

	partitions := map[string][]*mock.Node{
		"node1": {node1},
		"node2": {node2},
	}

	// Split the nodes into partitions and commit slot 1 only on node2
	{

		ts.SplitIntoPartitions(partitions)

		// Only node2 will commit after issuing this one
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{5}, 1, "5.0", ts.Nodes("node2"), false, nil)

		ts.AssertNodeState(ts.Nodes("node1"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(0),
			testsuite.WithEqualStoredCommitmentAtIndex(0),
			testsuite.WithEvictedSlot(0),
		)

		ts.AssertNodeState(ts.Nodes("node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(1),
			testsuite.WithEqualStoredCommitmentAtIndex(1),
			testsuite.WithEvictedSlot(1),
		)
	}

	commitment1 := lo.PanicOnErr(node2.Protocol.MainEngineInstance().Storage.Commitments().Load(1)).Commitment()

	// This should be booked on the rejected tx1 conflict
	tx4 := lo.PanicOnErr(ts.TransactionFramework.CreateSimpleTransaction("tx4", 1, "tx1:0"))

	// Issue TX3 on top of rejected TX1 and 1 commitment on node2 (committed to slot 1)
	{
		ts.IssueBlockAtSlotWithOptions("n2-commit1", 5, commitment1, node2, tx4)

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("n2-commit1"): {}, // no conflits inherited as the block is invalid and doesn't get booked.
			ts.Block("block2.tx1"): {"tx1"},
		}, node2)

		ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("tx1"), true, node2)
		ts.AssertTransactionsInCacheRejected(ts.TransactionFramework.Transactions("tx4"), true, node2)
		ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.Transactions("tx4"), true, node2)

		// As the block commits to 1 but spending something orphaned in 1 it should be invalid
		ts.AssertBlocksInCacheBooked(ts.Blocks("n2-commit1"), false, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n2-commit1"), true, node2)
	}

	// Issue a block on node1 that inherits a pending conflict that has been orphaned on node2
	{
		ts.IssueBlockAtSlot("n1-rejected-genesis", 5, genesisCommitment, node1, ts.BlockIDs("block2.tx1")...)

		ts.AssertBlocksInCacheBooked(ts.Blocks("n1-rejected-genesis"), true, node1)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n1-rejected-genesis"), false, node1)

		ts.AssertTransactionsInCacheRejected(ts.TransactionFramework.Transactions("tx1"), true, node2)

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block2.tx1"):          {"tx1"},
			ts.Block("n1-rejected-genesis"): {"tx1"}, // on rejected conflict
		}, node1)
	}

	// Issue TX4 on top of rejected TX1 but Genesis commitment on node2 (committed to slot 1)
	{
		ts.IssueBlockAtSlotWithOptions("n2-genesis", 5, genesisCommitment, node2, tx4, blockfactory.WithStrongParents(ts.BlockID("Genesis")))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("n2-genesis"): {"tx4"}, // on rejected conflict
		}, node2)

		ts.AssertBlocksInCacheBooked(ts.Blocks("n2-genesis"), true, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n2-genesis"), false, node2)
	}

	// Issue TX4 on top of rejected TX1 but Genesis commitment on node1 (committed to slot 0)
	{
		ts.IssueBlockAtSlotWithOptions("n1-genesis", 5, genesisCommitment, node1, tx4, blockfactory.WithStrongParents(ts.BlockID("Genesis")))

		ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("tx1"), true, node2)
		ts.AssertTransactionsInCacheRejected(ts.TransactionFramework.Transactions("tx4"), true, node2)
		ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.Transactions("tx4"), true, node2)

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("n1-genesis"): {"tx4"}, // on rejected conflict
		}, node1)

		ts.AssertBlocksInCacheBooked(ts.Blocks("n1-genesis"), true, node1)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n1-genesis"), false, node1)
	}

	ts.MergePartitionsToMain(lo.Keys(partitions)...)

	// Sync up the nodes to he same point and check consistency between them.
	{
		// Let node1 catch up with commitment 1
		ts.IssueBlocksAtSlots("5.1", []iotago.SlotIndex{5}, 1, "5.0", ts.Nodes("node2"), false, nil)

		ts.AssertNodeState(ts.Nodes("node1", "node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(1),
			testsuite.WithEqualStoredCommitmentAtIndex(1),
			testsuite.WithEvictedSlot(1),
		)

		// Exchange each-other blocks, ignoring invalidity
		ts.IssueExistingBlock("n2-genesis", node1)
		ts.IssueExistingBlock("n2-commit1", node1)
		ts.IssueExistingBlock("n1-genesis", node2)
		ts.IssueExistingBlock("n1-rejected-genesis", node2)

		ts.IssueBlockAtSlot("n1-rejected-commit1", 5, commitment1, node1, ts.BlockIDs("n1-rejected-genesis")...)
		// Needs reissuing on node2 because it is invalid
		ts.IssueExistingBlock("n1-rejected-commit1", node2)

		// The nodes agree on the results of the invalid blocks
		ts.AssertBlocksInCacheBooked(ts.Blocks("n2-genesis", "n1-genesis", "n1-rejected-genesis"), true, node1, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n2-genesis", "n1-genesis", "n1-rejected-genesis"), false, node1, node2)

		ts.AssertBlocksInCacheBooked(ts.Blocks("n1-rejected-commit1", "n2-commit1"), false, node1, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n1-rejected-commit1", "n2-commit1"), true, node1, node2)
	}

	// Commit further and test eviction of transactions
	{
		ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("tx1", "tx2", "tx4"), true, node1, node2)

		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{6, 7, 8, 9, 10}, 5, "5.1", ts.Nodes("node1", "node2"), false, nil)

		ts.AssertNodeState(ts.Nodes("node1", "node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(8),
			testsuite.WithEqualStoredCommitmentAtIndex(8),
			testsuite.WithEvictedSlot(8),
		)

		ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("tx1", "tx2", "tx4"), false, node1, node2)
	}
}

func Test_SpendPendingCommittedRace(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithGenesisTimestampOffset(20*10),
		testsuite.WithMinCommittableAge(2),
		testsuite.WithMaxCommittableAge(5),
	)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddValidatorNode("node2")

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		node1.AccountID,
		node2.AccountID,
	}, ts.Nodes()...)

	genesisCommitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(0)).Commitment()

	// Create and issue double spends
	{
		tx1 := lo.PanicOnErr(ts.TransactionFramework.CreateSimpleTransaction("tx1", 1, "Genesis:0"))
		tx2 := lo.PanicOnErr(ts.TransactionFramework.CreateSimpleTransaction("tx2", 1, "Genesis:0"))

		ts.IssueBlockAtSlotWithOptions("block1.1", 1, genesisCommitment, node2, tx1)
		ts.IssueBlockAtSlotWithOptions("block1.2", 1, genesisCommitment, node2, tx2)

		ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertTransactionsInCachePending(ts.TransactionFramework.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block1.1"): {"tx1"},
			ts.Block("block1.2"): {"tx2"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			ts.TransactionFramework.Transaction("tx2"): {"tx2"},
			ts.TransactionFramework.Transaction("tx1"): {"tx1"},
		}, node1, node2)
	}

	// Issue some more blocks and assert that conflicts are propagated to blocks.
	{
		ts.IssueBlockAtSlot("block2.1", 2, genesisCommitment, node2, ts.BlockID("block1.1"))
		ts.IssueBlockAtSlot("block2.2", 2, genesisCommitment, node2, ts.BlockID("block1.2"))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block2.1"): {"tx1"},
			ts.Block("block2.2"): {"tx2"},
		}, node1, node2)
		ts.AssertTransactionsInCachePending(ts.TransactionFramework.Transactions("tx1", "tx2"), true, node1, node2)
	}

	// Advance both nodes at the edge of slot 1 committability
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{2, 3, 4}, 1, "Genesis", ts.Nodes("node1", "node2"), false, nil)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(0),
			testsuite.WithEqualStoredCommitmentAtIndex(0),
			testsuite.WithEvictedSlot(0),
		)

		ts.IssueBlockAtSlot("", 5, genesisCommitment, node1, ts.BlockIDsWithPrefix("4.0")...)

		// ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
		// 	ts.Block("block5.1"): {}, // on rejected conflict
		// }, node1, node2)

		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{5}, 1, "4.0", ts.Nodes("node1"), false, nil)

		ts.AssertBlocksExist(ts.BlocksWithPrefix("5.0"), true, ts.Nodes()...)
	}

	partitions := map[string][]*mock.Node{
		"node1": {node1},
		"node2": {node2},
	}

	// Split the nodes into partitions and commit slot 1 only on node2
	{
		ts.SplitIntoPartitions(partitions)

		// Only node2 will commit after issuing this one
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{5}, 1, "5.0", ts.Nodes("node2"), false, nil)

		ts.AssertNodeState(ts.Nodes("node1"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(0),
			testsuite.WithEqualStoredCommitmentAtIndex(0),
			testsuite.WithEvictedSlot(0),
		)

		ts.AssertNodeState(ts.Nodes("node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(1),
			testsuite.WithEqualStoredCommitmentAtIndex(1),
			testsuite.WithEvictedSlot(1),
		)
	}

	commitment1 := lo.PanicOnErr(node2.Protocol.MainEngineInstance().Storage.Commitments().Load(1)).Commitment()

	// Issue a block booked on a pending conflict on node2
	{
		ts.IssueBlockAtSlot("n2-pending-genesis", 5, genesisCommitment, node2, ts.BlockIDs("block2.1")...)
		ts.IssueBlockAtSlot("n2-pending-commit1", 5, commitment1, node2, ts.BlockIDs("block2.1")...)

		ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("tx1"), true, node2)
		ts.AssertTransactionsInCachePending(ts.TransactionFramework.Transactions("tx1"), true, node2)

		ts.AssertBlocksInCacheBooked(ts.Blocks("n2-pending-genesis"), true, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n2-pending-genesis"), false, node2)

		// As the block commits to 1 but spending something orphaned in 1 it should be invalid
		ts.AssertBlocksInCacheBooked(ts.Blocks("n2-pending-commit1"), false, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n2-pending-commit1"), true, node2)

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block2.1"):           {"tx1"},
			ts.Block("n2-pending-genesis"): {"tx1"},
			ts.Block("n2-pending-commit1"): {}, // no conflits inherited as the block is invalid and doesn't get booked.
		}, node2)
	}

	ts.MergePartitionsToMain(lo.Keys(partitions)...)

	// Sync up the nodes to he same point and check consistency between them.
	{
		// Let node1 catch up with commitment 1
		ts.IssueBlocksAtSlots("5.1", []iotago.SlotIndex{5}, 1, "5.0", ts.Nodes("node2"), false, nil)

		ts.AssertNodeState(ts.Nodes("node1", "node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(1),
			testsuite.WithEqualStoredCommitmentAtIndex(1),
			testsuite.WithEvictedSlot(1),
		)

		// Exchange each-other blocks, ignoring invalidity
		ts.IssueExistingBlock("n2-pending-genesis", node1)
		ts.IssueExistingBlock("n2-pending-commit1", node1)

		// The nodes agree on the results of the invalid blocks
		ts.AssertBlocksInCacheBooked(ts.Blocks("n2-pending-genesis"), true, node1, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n2-pending-genesis"), false, node1, node2)

		ts.AssertBlocksInCacheBooked(ts.Blocks("n2-pending-commit1"), false, node1, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n2-pending-commit1"), true, node1, node2)
	}

	// Commit further and test eviction of transactions
	{
		ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertTransactionsInCachePending(ts.TransactionFramework.Transactions("tx1", "tx2"), true, node1, node2)

		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{6, 7, 8, 9, 10}, 5, "5.1", ts.Nodes("node1", "node2"), false, nil)

		ts.AssertNodeState(ts.Nodes("node1", "node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(8),
			testsuite.WithEqualStoredCommitmentAtIndex(8),
			testsuite.WithEvictedSlot(8),
		)

		ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("tx1", "tx2"), false, node1, node2)
	}
}
