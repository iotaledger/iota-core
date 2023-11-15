package tests

import (
	"testing"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
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
	wallet := ts.AddGenesisWallet("default", node1)
	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	tx1 := wallet.CreateBasicOutputsEquallyFromInputs("tx1", 1, "Genesis:0")

	tx2 := wallet.CreateBasicOutputsEquallyFromInputs("tx2", 1, "tx1:0")

	ts.IssuePayloadWithOptions("block1", wallet, tx2)

	ts.AssertTransactionsExist(wallet.Transactions("tx2"), true, node1)
	ts.AssertTransactionsExist(wallet.Transactions("tx1"), false, node1)

	ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx2"), false, node1)
	// make sure that the block is not booked

	ts.IssuePayloadWithOptions("block2", wallet, tx1)

	ts.AssertTransactionsExist(wallet.Transactions("tx1", "tx2"), true, node1)
	ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1", "tx2"), true, node1)
	ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
		ts.Block("block1"): {"tx2"},
		ts.Block("block2"): {"tx1"},
	}, node1)

	ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
		wallet.Transaction("tx2"): {"tx2"},
		wallet.Transaction("tx1"): {"tx1"},
	}, node1)
}

func Test_WeightPropagation(t *testing.T) {
	ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddValidatorNode("node2")

	wallet := ts.AddGenesisWallet("default", node1)

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		node1.Validator.AccountID,
		node2.Validator.AccountID,
	}, ts.Nodes()...)

	// Create and issue double spends
	{
		tx1 := wallet.CreateBasicOutputsEquallyFromInputs("tx1", 1, "Genesis:0")
		tx2 := wallet.CreateBasicOutputsEquallyFromInputs("tx2", 1, "Genesis:0")

		ts.IssuePayloadWithOptions("block1", wallet, tx1, mock.WithStrongParents(ts.BlockID("Genesis")))
		ts.IssuePayloadWithOptions("block2", wallet, tx2, mock.WithStrongParents(ts.BlockID("Genesis")))

		ts.AssertTransactionsExist(wallet.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block1"): {"tx1"},
			ts.Block("block2"): {"tx2"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			wallet.Transaction("tx2"): {"tx2"},
			wallet.Transaction("tx1"): {"tx1"},
		}, node1, node2)
	}

	// Issue some more blocks and assert that conflicts are propagated to blocks.
	{

		ts.IssuePayloadWithOptions("block3-basic", ts.Wallet("node1"), &iotago.TaggedData{}, mock.WithStrongParents(ts.BlockID("block1")))
		ts.IssuePayloadWithOptions("block4-basic", ts.Wallet("node2"), &iotago.TaggedData{}, mock.WithStrongParents(ts.BlockID("block2")))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block3-basic"): {"tx1"},
			ts.Block("block4-basic"): {"tx2"},
		}, node1, node2)
		ts.AssertConflictsInCacheAcceptanceState([]string{"tx1", "tx2"}, acceptance.Pending, ts.Nodes()...)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)
	}

	// Issue valid blocks that should resolve the conflict, but basic blocks don't carry any weight..
	{
		ts.IssuePayloadWithOptions("block5-basic", ts.Wallet("node1"), &iotago.TaggedData{}, mock.WithStrongParents(ts.BlockIDs("block4-basic")...), mock.WithShallowLikeParents(ts.BlockID("block2")))
		ts.IssuePayloadWithOptions("block6-basic", ts.Wallet("node2"), &iotago.TaggedData{}, mock.WithStrongParents(ts.BlockIDs("block5-basic")...))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block6-basic"): {"tx2"},
		}, ts.Nodes()...)

		// Make sure that neither approval (conflict weight),
		// nor witness (block weight) was not propagated using basic blocks and caused acceptance.
		ts.AssertConflictsInCacheAcceptanceState([]string{"tx1", "tx2"}, acceptance.Pending, ts.Nodes()...)
		ts.AssertTransactionsInCacheAccepted(wallet.Transactions("tx2"), false, node1, node2)
		ts.AssertTransactionsInCacheRejected(wallet.Transactions("tx1"), false, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("block3-basic", "block4-basic", "block5-basic", "block6-basic"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("block3-basic", "block4-basic", "block5-basic", "block6-basic"), false, ts.Nodes()...)
	}

	// Issue validator blocks that are subjectively invalid, but accept the basic blocks.
	// Make sure that the pre-accepted basic blocks do not apply approval weight - the conflicts should remain unresolved.
	// If basic blocks carry approval or witness weight, then the test will fail.
	{
		ts.IssueValidationBlock("block8", node1, mock.WithStrongParents(ts.BlockIDs("block3-basic", "block6-basic")...))
		ts.IssueValidationBlock("block9", node2, mock.WithStrongParents(ts.BlockID("block8")))
		ts.IssueValidationBlock("block10", node1, mock.WithStrongParents(ts.BlockID("block9")))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block8"):  {"tx1", "tx2"},
			ts.Block("block9"):  {"tx1", "tx2"},
			ts.Block("block10"): {"tx1", "tx2"},
		}, node1, node2)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("block3-basic", "block4-basic", "block5-basic", "block6-basic"), true, node1, node2)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("block3-basic", "block4-basic", "block5-basic", "block6-basic"), true, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)
	}
}

func Test_DoubleSpend(t *testing.T) {
	ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddValidatorNode("node2")
	wallet := ts.AddGenesisWallet("default", node1)

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		node1.Validator.AccountID,
		node2.Validator.AccountID,
	}, ts.Nodes()...)

	// Create and issue double spends
	{
		tx1 := wallet.CreateBasicOutputsEquallyFromInputs("tx1", 1, "Genesis:0")
		tx2 := wallet.CreateBasicOutputsEquallyFromInputs("tx2", 1, "Genesis:0")

		ts.IssuePayloadWithOptions("block1", wallet, tx1, mock.WithStrongParents(ts.BlockID("Genesis")))
		ts.IssuePayloadWithOptions("block2", wallet, tx2, mock.WithStrongParents(ts.BlockID("Genesis")))

		ts.AssertTransactionsExist(wallet.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block1"): {"tx1"},
			ts.Block("block2"): {"tx2"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			wallet.Transaction("tx2"): {"tx2"},
			wallet.Transaction("tx1"): {"tx1"},
		}, node1, node2)
	}

	// Issue some more blocks and assert that conflicts are propagated to blocks.
	{
		ts.IssueValidationBlock("block3", node1, mock.WithStrongParents(ts.BlockID("block1")))
		ts.IssueValidationBlock("block4", node1, mock.WithStrongParents(ts.BlockID("block2")))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block3"): {"tx1"},
			ts.Block("block4"): {"tx2"},
		}, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)
	}

	// Issue an invalid block and assert that its vote is not cast.
	{
		ts.IssueValidationBlock("block5", node2, mock.WithStrongParents(ts.BlockIDs("block3", "block4")...))

		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)
	}

	// Issue valid blocks that resolve the conflict.
	{
		ts.IssueValidationBlock("block6", node2, mock.WithStrongParents(ts.BlockIDs("block3", "block4")...), mock.WithShallowLikeParents(ts.BlockID("block2")))
		ts.IssueValidationBlock("block7", node1, mock.WithStrongParents(ts.BlockIDs("block6")...))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block6"): {"tx2"},
		}, node1, node2)
		ts.AssertTransactionsInCacheAccepted(wallet.Transactions("tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheRejected(wallet.Transactions("tx1"), true, node1, node2)

	}
}

func Test_MultipleAttachments(t *testing.T) {
	ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	nodeA := ts.AddValidatorNode("nodeA")
	nodeB := ts.AddValidatorNode("nodeB")
	wallet := ts.AddGenesisWallet("default", nodeA)

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	blocksConflicts := make(map[*blocks.Block][]string)

	// Create a transaction and issue it from both nodes, so that the conflict is accepted, but no attachment is included yet.
	{
		tx1 := wallet.CreateBasicOutputsEquallyFromInputs("tx1", 2, "Genesis:0")

		ts.IssuePayloadWithOptions("A.1", wallet, tx1, mock.WithStrongParents(ts.BlockID("Genesis")))
		ts.IssueValidationBlock("A.1.1", nodeA, mock.WithStrongParents(ts.BlockID("A.1")))
		wallet.SetDefaultNode(nodeB)
		ts.IssuePayloadWithOptions("B.1", wallet, tx1, mock.WithStrongParents(ts.BlockID("Genesis")))
		ts.IssueValidationBlock("B.1.1", nodeB, mock.WithStrongParents(ts.BlockID("B.1")))

		ts.IssueValidationBlock("A.2", nodeA, mock.WithStrongParents(ts.BlockID("B.1.1")))
		ts.IssueValidationBlock("B.2", nodeB, mock.WithStrongParents(ts.BlockID("A.1.1")))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.1", "B.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("A.1", "B.1"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheConflicts(lo.MergeMaps(blocksConflicts, map[*blocks.Block][]string{
			ts.Block("A.1"): {"tx1"},
			ts.Block("B.1"): {"tx1"},
			ts.Block("A.2"): {"tx1"},
			ts.Block("B.2"): {"tx1"},
		}), ts.Nodes()...)
		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			wallet.Transaction("tx1"): {"tx1"},
		}, ts.Nodes()...)
		ts.AssertSpendsInCacheAcceptanceState([]string{"tx1"}, acceptance.Accepted, ts.Nodes()...)
	}

	// Create a transaction that is included and whose conflict is accepted, but whose inputs are not accepted.
	{
		tx2 := wallet.CreateBasicOutputsEquallyFromInputs("tx2", 1, "tx1:1")

		wallet.SetDefaultNode(nodeA)
		ts.IssuePayloadWithOptions("A.3", wallet, tx2, mock.WithStrongParents(ts.BlockID("Genesis")))
		ts.IssueValidationBlock("A.3.1", nodeA, mock.WithStrongParents(ts.BlockID("A.3")))
		ts.IssueValidationBlock("B.3", nodeB, mock.WithStrongParents(ts.BlockID("A.3.1")))
		ts.IssueValidationBlock("A.4", nodeA, mock.WithStrongParents(ts.BlockID("B.3")))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.3"), true, ts.Nodes()...)

		ts.IssueValidationBlock("B.4", nodeB, mock.WithStrongParents(ts.BlockIDs("B.3", "A.4")...))
		ts.IssueValidationBlock("A.5", nodeA, mock.WithStrongParents(ts.BlockIDs("B.3", "A.4")...))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("B.3", "A.4"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("B.4", "A.5"), false, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("A.3"), true, ts.Nodes()...)

		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1", "tx2"), true, ts.Nodes()...)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, ts.Nodes()...)

		ts.AssertBlocksInCacheConflicts(lo.MergeMaps(blocksConflicts, map[*blocks.Block][]string{
			ts.Block("A.3"): {"tx2"},
			ts.Block("B.3"): {"tx2"},
			ts.Block("A.4"): {"tx2"},
			ts.Block("A.5"): {},
			ts.Block("B.4"): {},
		}), ts.Nodes()...)
		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			wallet.Transaction("tx1"): {"tx1"},
			wallet.Transaction("tx2"): {"tx2"},
		}, nodeA, nodeB)
		ts.AssertSpendsInCacheAcceptanceState([]string{"tx1", "tx2"}, acceptance.Accepted, ts.Nodes()...)
	}

	// Issue a block that includes tx1, and make sure that tx2 is accepted as well as a consequence.
	{
		ts.IssueValidationBlock("A.6", nodeA, mock.WithStrongParents(ts.BlockIDs("A.2", "B.2")...))
		ts.IssueValidationBlock("B.5", nodeB, mock.WithStrongParents(ts.BlockIDs("A.2", "B.2")...))

		ts.IssueValidationBlock("A.7", nodeA, mock.WithStrongParents(ts.BlockIDs("A.6", "B.5")...))
		ts.IssueValidationBlock("B.6", nodeB, mock.WithStrongParents(ts.BlockIDs("A.6", "B.5")...))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.2", "B.2", "A.6", "B.5"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("A.1", "B.1"), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.7", "B.6"), false, ts.Nodes()...)
		ts.AssertTransactionsExist(wallet.Transactions("tx1", "tx2"), true, ts.Nodes()...)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1", "tx2"), true, ts.Nodes()...)
		ts.AssertTransactionsInCacheAccepted(wallet.Transactions("tx1", "tx2"), true, ts.Nodes()...)

		ts.AssertBlocksInCacheConflicts(lo.MergeMaps(blocksConflicts, map[*blocks.Block][]string{
			ts.Block("A.6"): {},
			ts.Block("B.5"): {},
			ts.Block("A.7"): {},
			ts.Block("B.6"): {},
		}), ts.Nodes()...)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			wallet.Transaction("tx1"): {"tx1"},
			wallet.Transaction("tx2"): {"tx2"},
		}, nodeA, nodeB)
		ts.AssertSpendsInCacheAcceptanceState([]string{"tx1", "tx2"}, acceptance.Accepted, nodeA, nodeB)
	}
}

func Test_SpendRejectedCommittedRace(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(20, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				testsuite.DefaultSlotsPerEpochExponent,
			),
			iotago.WithLivenessOptions(
				testsuite.DefaultLivenessThresholdLowerBoundInSeconds,
				testsuite.DefaultLivenessThresholdUpperBoundInSeconds,
				2,
				5,
				testsuite.DefaultEpochNearingThreshold,
			),
		),
	)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddValidatorNode("node2")
	wallet := ts.AddGenesisWallet("default", node1)

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		node1.Validator.AccountID,
		node2.Validator.AccountID,
	}, ts.Nodes()...)

	genesisCommitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(0)).Commitment()

	// Create and issue double spends
	{
		tx1 := wallet.CreateBasicOutputsEquallyFromInputs("tx1", 1, "Genesis:0")
		tx2 := wallet.CreateBasicOutputsEquallyFromInputs("tx2", 1, "Genesis:0")

		wallet.SetDefaultNode(node1)
		ts.IssueBasicBlockAtSlotWithOptions("block1.1", 1, wallet, tx1, mock.WithSlotCommitment(genesisCommitment))
		ts.IssueBasicBlockAtSlotWithOptions("block1.2", 1, wallet, tx2, mock.WithSlotCommitment(genesisCommitment))
		ts.IssueValidationBlockAtSlot("block2.tx1", 2, genesisCommitment, node1, ts.BlockIDs("block1.1")...)

		ts.AssertTransactionsExist(wallet.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block1.1"):   {"tx1"},
			ts.Block("block1.2"):   {"tx2"},
			ts.Block("block2.tx1"): {"tx1"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			wallet.Transaction("tx2"): {"tx2"},
			wallet.Transaction("tx1"): {"tx1"},
		}, node1, node2)
	}

	// Issue some more blocks and assert that conflicts are propagated to blocks.
	{
		ts.IssueValidationBlockAtSlot("block2.1", 2, genesisCommitment, node1, ts.BlockID("block1.1"))
		ts.IssueValidationBlockAtSlot("block2.2", 2, genesisCommitment, node1, ts.BlockID("block1.2"))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block2.1"):   {"tx1"},
			ts.Block("block2.2"):   {"tx2"},
			ts.Block("block2.tx1"): {"tx1"},
		}, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)
	}

	// Issue valid blocks that resolve the conflict.
	{
		ts.IssueValidationBlockAtSlot("block2.3", 2, genesisCommitment, node2, ts.BlockIDs("block2.2")...)
		ts.IssueValidationBlockAtSlot("block2.4", 2, genesisCommitment, node1, ts.BlockIDs("block2.3")...)

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block2.3"):   {"tx2"},
			ts.Block("block2.tx1"): {"tx1"},
		}, node1, node2)
		ts.AssertTransactionsInCacheAccepted(wallet.Transactions("tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheRejected(wallet.Transactions("tx1"), true, node1, node2)
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

		ts.IssueValidationBlockAtSlot("block5.1", 5, genesisCommitment, node1, ts.BlockIDsWithPrefix("block1.1")...)
		ts.IssueValidationBlockAtSlot("block5.2", 5, genesisCommitment, node1, ts.BlockIDsWithPrefix("block1.2")...)

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
	tx4 := wallet.CreateBasicOutputsEquallyFromInputs("tx4", 1, "tx1:0")

	// Issue TX3 on top of rejected TX1 and 1 commitment on node2 (committed to slot 1)
	{
		wallet.SetDefaultNode(node2)
		ts.IssueBasicBlockAtSlotWithOptions("n2-commit1", 5, wallet, tx4, mock.WithSlotCommitment(commitment1))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("n2-commit1"): {}, // no conflits inherited as the block is invalid and doesn't get booked.
			ts.Block("block2.tx1"): {"tx1"},
		}, node2)

		ts.AssertTransactionsExist(wallet.Transactions("tx1"), true, node2)
		ts.AssertTransactionsInCacheRejected(wallet.Transactions("tx4"), true, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx4"), true, node2)

		// As the block commits to 1 but spending something orphaned in 1 it should be invalid
		ts.AssertBlocksInCacheBooked(ts.Blocks("n2-commit1"), false, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n2-commit1"), true, node2)
	}

	// Issue a block on node1 that inherits a pending conflict that has been orphaned on node2
	{
		ts.IssueValidationBlockAtSlot("n1-rejected-genesis", 5, genesisCommitment, node1, ts.BlockIDs("block2.tx1")...)

		ts.AssertBlocksInCacheBooked(ts.Blocks("n1-rejected-genesis"), true, node1)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n1-rejected-genesis"), false, node1)

		ts.AssertTransactionsInCacheRejected(wallet.Transactions("tx1"), true, node2)

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block2.tx1"):          {"tx1"},
			ts.Block("n1-rejected-genesis"): {"tx1"}, // on rejected conflict
		}, node1)
	}

	// Issue TX4 on top of rejected TX1 but Genesis commitment on node2 (committed to slot 1)
	{
		wallet.SetDefaultNode(node2)
		ts.IssueBasicBlockAtSlotWithOptions("n2-genesis", 5, wallet, tx4, mock.WithStrongParents(ts.BlockID("Genesis")), mock.WithSlotCommitment(genesisCommitment))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("n2-genesis"): {"tx4"}, // on rejected conflict
		}, node2)

		ts.AssertBlocksInCacheBooked(ts.Blocks("n2-genesis"), true, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n2-genesis"), false, node2)
	}

	// Issue TX4 on top of rejected TX1 but Genesis commitment on node1 (committed to slot 0)
	{
		wallet.SetDefaultNode(node1)
		ts.IssueBasicBlockAtSlotWithOptions("n1-genesis", 5, wallet, tx4, mock.WithStrongParents(ts.BlockID("Genesis")), mock.WithSlotCommitment(genesisCommitment))

		ts.AssertTransactionsExist(wallet.Transactions("tx1"), true, node2)
		ts.AssertTransactionsInCacheRejected(wallet.Transactions("tx4"), true, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx4"), true, node2)

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
		wallet.SetDefaultNode(node1)
		ts.IssueExistingBlock("n2-genesis", wallet)
		ts.IssueExistingBlock("n2-commit1", wallet)
		wallet.SetDefaultNode(node2)
		ts.IssueExistingBlock("n1-genesis", wallet)
		ts.IssueExistingBlock("n1-rejected-genesis", wallet)

		ts.IssueValidationBlockAtSlot("n1-rejected-commit1", 5, commitment1, node1, ts.BlockIDs("n1-rejected-genesis")...)
		// Needs reissuing on node2 because it is invalid
		ts.IssueExistingBlock("n1-rejected-commit1", wallet)

		// The nodes agree on the results of the invalid blocks
		ts.AssertBlocksInCacheBooked(ts.Blocks("n2-genesis", "n1-genesis", "n1-rejected-genesis"), true, node1, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n2-genesis", "n1-genesis", "n1-rejected-genesis"), false, node1, node2)

		// This block propagates the orphaned conflict from Tangle
		ts.AssertBlocksInCacheBooked(ts.Blocks("n1-rejected-commit1"), true, node1, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n1-rejected-commit1"), false, node1, node2)

		// This block spends an orphaned conflict from its Transaction
		ts.AssertBlocksInCacheBooked(ts.Blocks("n2-commit1"), false, node1, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n2-commit1"), true, node1, node2)

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("n1-genesis"):          {"tx4"}, // on rejected conflict
			ts.Block("n2-genesis"):          {"tx4"}, // on rejected conflict
			ts.Block("n1-rejected-genesis"): {"tx1"}, // on rejected conflict
			ts.Block("n2-commit1"):          {},      // invalid block
			ts.Block("n1-rejected-commit1"): {},      // merged-to-master
		}, node1, node2)
	}

	// Commit further and test eviction of transactions
	{
		ts.AssertTransactionsExist(wallet.Transactions("tx1", "tx2", "tx4"), true, node1, node2)

		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{6, 7, 8, 9, 10}, 5, "5.1", ts.Nodes("node1", "node2"), false, nil)

		ts.AssertNodeState(ts.Nodes("node1", "node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(8),
			testsuite.WithEqualStoredCommitmentAtIndex(8),
			testsuite.WithEvictedSlot(8),
		)

		ts.AssertTransactionsExist(wallet.Transactions("tx1", "tx2", "tx4"), false, node1, node2)
	}
}

func Test_SpendPendingCommittedRace(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(20, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				testsuite.DefaultSlotsPerEpochExponent,
			),
			iotago.WithLivenessOptions(
				testsuite.DefaultLivenessThresholdLowerBoundInSeconds,
				testsuite.DefaultLivenessThresholdUpperBoundInSeconds,
				2,
				5,
				testsuite.DefaultEpochNearingThreshold,
			),
		),
	)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddValidatorNode("node2")
	wallet := ts.AddGenesisWallet("default", node1)

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		node1.Validator.AccountID,
		node2.Validator.AccountID,
	}, ts.Nodes()...)

	genesisCommitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(0)).Commitment()

	// Create and issue double spends
	{
		tx1 := wallet.CreateBasicOutputsEquallyFromInputs("tx1", 1, "Genesis:0")
		tx2 := wallet.CreateBasicOutputsEquallyFromInputs("tx2", 1, "Genesis:0")

		wallet.SetDefaultNode(node2)
		ts.IssueBasicBlockAtSlotWithOptions("block1.1", 1, wallet, tx1)
		ts.IssueBasicBlockAtSlotWithOptions("block1.2", 1, wallet, tx2)

		ts.AssertTransactionsExist(wallet.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block1.1"): {"tx1"},
			ts.Block("block1.2"): {"tx2"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			wallet.Transaction("tx2"): {"tx2"},
			wallet.Transaction("tx1"): {"tx1"},
		}, node1, node2)
	}

	// Issue some more blocks and assert that conflicts are propagated to blocks.
	{
		ts.IssueValidationBlockAtSlot("block2.1", 2, genesisCommitment, node2, ts.BlockID("block1.1"))
		ts.IssueValidationBlockAtSlot("block2.2", 2, genesisCommitment, node2, ts.BlockID("block1.2"))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block2.1"): {"tx1"},
			ts.Block("block2.2"): {"tx2"},
		}, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)
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

		ts.IssueValidationBlockAtSlot("", 5, genesisCommitment, node1, ts.BlockIDsWithPrefix("4.0")...)

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
		ts.IssueValidationBlockAtSlot("n2-pending-genesis", 5, genesisCommitment, node2, ts.BlockIDs("block2.1")...)
		ts.IssueValidationBlockAtSlot("n2-pending-commit1", 5, commitment1, node2, ts.BlockIDs("block2.1")...)

		ts.AssertTransactionsExist(wallet.Transactions("tx1"), true, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1"), true, node2)

		ts.AssertBlocksInCacheBooked(ts.Blocks("n2-pending-genesis", "n2-pending-commit1"), true, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n2-pending-genesis", "n2-pending-commit1"), false, node2)

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block2.1"):           {"tx1"},
			ts.Block("n2-pending-genesis"): {"tx1"},
			ts.Block("n2-pending-commit1"): {}, // no conflits inherited as the block merges orphaned conflicts.
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
		wallet.SetDefaultNode(node1)
		ts.IssueExistingBlock("n2-pending-genesis", wallet)
		ts.IssueExistingBlock("n2-pending-commit1", wallet)

		// The nodes agree on the results of the invalid blocks
		ts.AssertBlocksInCacheBooked(ts.Blocks("n2-pending-genesis", "n2-pending-commit1"), true, node1, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n2-pending-genesis", "n2-pending-commit1"), false, node1, node2)

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block2.1"):           {"tx1"},
			ts.Block("n2-pending-genesis"): {"tx1"},
			ts.Block("n2-pending-commit1"): {}, // no conflits inherited as the block merges orphaned conflicts.
		}, node1, node2)

		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)
	}

	// Commit further and test eviction of transactions
	{
		ts.AssertTransactionsExist(wallet.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)

		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{6, 7, 8, 9, 10}, 5, "5.1", ts.Nodes("node1", "node2"), false, nil)

		ts.AssertNodeState(ts.Nodes("node1", "node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(8),
			testsuite.WithEqualStoredCommitmentAtIndex(8),
			testsuite.WithEvictedSlot(8),
		)

		ts.AssertTransactionsExist(wallet.Transactions("tx1", "tx2"), false, node1, node2)
	}
}
