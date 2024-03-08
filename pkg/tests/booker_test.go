package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/acceptance"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
)

func Test_IssuingTransactionsOutOfOrder(t *testing.T) {
	ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	wallet := ts.AddDefaultWallet(node1)
	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	tx1 := wallet.CreateBasicOutputsEquallyFromInput("tx1", 1, "Genesis:0")

	tx2 := wallet.CreateBasicOutputsEquallyFromInput("tx2", 1, "tx1:0")

	ts.IssueBasicBlockWithOptions("block1", wallet, tx2)

	ts.AssertTransactionsExist(wallet.Transactions("tx2"), true, node1)
	ts.AssertTransactionsExist(wallet.Transactions("tx1"), false, node1)

	ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx2"), false, node1)
	// make sure that the block is not booked

	ts.IssueBasicBlockWithOptions("block2", wallet, tx1)

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

	wallet := ts.AddDefaultWallet(node1)

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		node1.Validator.AccountID,
		node2.Validator.AccountID,
	}, ts.Nodes()...)

	// Create and issue double spends
	{
		tx1 := wallet.CreateBasicOutputsEquallyFromInput("tx1", 1, "Genesis:0")
		tx2 := wallet.CreateBasicOutputsEquallyFromInput("tx2", 1, "Genesis:0")

		ts.IssueBasicBlockWithOptions("block1", wallet, tx1, mock.WithStrongParents(ts.BlockID("Genesis")))
		ts.IssueBasicBlockWithOptions("block2", wallet, tx2, mock.WithStrongParents(ts.BlockID("Genesis")))

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

		ts.IssueBasicBlockWithOptions("block3-basic", ts.Wallet("node1"), &iotago.TaggedData{}, mock.WithStrongParents(ts.BlockID("block1")))
		ts.IssueBasicBlockWithOptions("block4-basic", ts.Wallet("node2"), &iotago.TaggedData{}, mock.WithStrongParents(ts.BlockID("block2")))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block3-basic"): {"tx1"},
			ts.Block("block4-basic"): {"tx2"},
		}, node1, node2)
		ts.AssertSpendersInCacheAcceptanceState([]string{"tx1", "tx2"}, acceptance.Pending, ts.Nodes()...)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)
	}

	// Issue valid blocks that should resolve the conflict, but basic blocks don't carry any weight..
	{
		ts.IssueBasicBlockWithOptions("block5-basic", ts.Wallet("node1"), &iotago.TaggedData{}, mock.WithStrongParents(ts.BlockIDs("block4-basic")...), mock.WithShallowLikeParents(ts.BlockID("block2")))
		ts.IssueBasicBlockWithOptions("block6-basic", ts.Wallet("node2"), &iotago.TaggedData{}, mock.WithStrongParents(ts.BlockIDs("block5-basic")...))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block6-basic"): {"tx2"},
		}, ts.Nodes()...)

		// Make sure that neither approval (conflict weight),
		// nor witness (block weight) was not propagated using basic blocks and caused acceptance.
		ts.AssertSpendersInCacheAcceptanceState([]string{"tx1", "tx2"}, acceptance.Pending, ts.Nodes()...)
		ts.AssertTransactionsInCacheAccepted(wallet.Transactions("tx2"), false, node1, node2)
		ts.AssertTransactionsInCacheRejected(wallet.Transactions("tx1"), false, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("block3-basic", "block4-basic", "block5-basic", "block6-basic"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("block3-basic", "block4-basic", "block5-basic", "block6-basic"), false, ts.Nodes()...)
	}

	// Issue validation blocks that are subjectively invalid, but accept the basic blocks.
	// Make sure that the pre-accepted basic blocks do not apply approval weight - the conflicts should remain unresolved.
	// If basic blocks carry approval or witness weight, then the test will fail.
	{
		ts.IssueValidationBlockWithHeaderOptions("block8", node1, mock.WithStrongParents(ts.BlockIDs("block3-basic", "block6-basic")...))
		ts.IssueValidationBlockWithHeaderOptions("block9", node2, mock.WithStrongParents(ts.BlockID("block8")))
		ts.IssueValidationBlockWithHeaderOptions("block10", node1, mock.WithStrongParents(ts.BlockID("block9")))

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
	wallet := ts.AddDefaultWallet(node1)

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		node1.Validator.AccountID,
		node2.Validator.AccountID,
	}, ts.Nodes()...)

	// Create and issue double spends
	{
		tx1 := wallet.CreateBasicOutputsEquallyFromInput("tx1", 1, "Genesis:0")
		tx2 := wallet.CreateBasicOutputsEquallyFromInput("tx2", 1, "Genesis:0")

		ts.IssueBasicBlockWithOptions("block1", wallet, tx1, mock.WithStrongParents(ts.BlockID("Genesis")))
		ts.IssueBasicBlockWithOptions("block2", wallet, tx2, mock.WithStrongParents(ts.BlockID("Genesis")))

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
		ts.IssueValidationBlockWithHeaderOptions("block3", node1, mock.WithStrongParents(ts.BlockID("block1")))
		ts.IssueValidationBlockWithHeaderOptions("block4", node1, mock.WithStrongParents(ts.BlockID("block2")))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block3"): {"tx1"},
			ts.Block("block4"): {"tx2"},
		}, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)
	}

	// Issue an invalid block and assert that its vote is not cast.
	{
		ts.IssueValidationBlockWithHeaderOptions("block5", node2, mock.WithStrongParents(ts.BlockIDs("block3", "block4")...))

		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)
	}

	// Issue valid blocks that resolve the conflict.
	{
		ts.IssueValidationBlockWithHeaderOptions("block6", node2, mock.WithStrongParents(ts.BlockIDs("block3", "block4")...), mock.WithShallowLikeParents(ts.BlockID("block2")))
		ts.IssueValidationBlockWithHeaderOptions("block7", node1, mock.WithStrongParents(ts.BlockIDs("block6")...))

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
	wallet := ts.AddDefaultWallet(nodeA)

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	blocksConflicts := make(map[*blocks.Block][]string)

	// Create a transaction and issue it from both nodes, so that the conflict is accepted, but no attachment is included yet.
	{
		tx1 := wallet.CreateBasicOutputsEquallyFromInput("tx1", 2, "Genesis:0")

		ts.IssueBasicBlockWithOptions("A.1", wallet, tx1, mock.WithStrongParents(ts.BlockID("Genesis")))
		ts.IssueValidationBlockWithHeaderOptions("A.1.1", nodeA, mock.WithStrongParents(ts.BlockID("A.1")))
		wallet.SetDefaultNode(nodeB)
		ts.IssueBasicBlockWithOptions("B.1", wallet, tx1, mock.WithStrongParents(ts.BlockID("Genesis")))
		ts.IssueValidationBlockWithHeaderOptions("B.1.1", nodeB, mock.WithStrongParents(ts.BlockID("B.1")))

		nodeA.Wait()
		ts.IssueValidationBlockWithHeaderOptions("A.2", nodeA, mock.WithStrongParents(ts.BlockID("B.1.1")))
		ts.IssueValidationBlockWithHeaderOptions("B.2", nodeB, mock.WithStrongParents(ts.BlockID("A.1.1")))

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
		ts.AssertSpendersInCacheAcceptanceState([]string{"tx1"}, acceptance.Accepted, ts.Nodes()...)
	}

	// Create a transaction that is included and whose conflict is accepted, but whose inputs are not accepted.
	{
		tx2 := wallet.CreateBasicOutputsEquallyFromInput("tx2", 1, "tx1:1")

		wallet.SetDefaultNode(nodeA)
		ts.IssueBasicBlockWithOptions("A.3", wallet, tx2, mock.WithStrongParents(ts.BlockID("Genesis")))
		ts.IssueValidationBlockWithHeaderOptions("A.3.1", nodeA, mock.WithStrongParents(ts.BlockID("A.3")))
		ts.IssueValidationBlockWithHeaderOptions("B.3", nodeB, mock.WithStrongParents(ts.BlockID("A.3.1")))
		ts.IssueValidationBlockWithHeaderOptions("A.4", nodeA, mock.WithStrongParents(ts.BlockID("B.3")))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.3"), true, ts.Nodes()...)

		ts.IssueValidationBlockWithHeaderOptions("B.4", nodeB, mock.WithStrongParents(ts.BlockIDs("B.3", "A.4")...))
		ts.IssueValidationBlockWithHeaderOptions("A.5", nodeA, mock.WithStrongParents(ts.BlockIDs("B.3", "A.4")...))

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
		ts.AssertSpendersInCacheAcceptanceState([]string{"tx1", "tx2"}, acceptance.Accepted, ts.Nodes()...)
	}

	// Issue a block that includes tx1, and make sure that tx2 is accepted as well as a consequence.
	{
		ts.IssueValidationBlockWithHeaderOptions("A.6", nodeA, mock.WithStrongParents(ts.BlockIDs("A.2", "B.2")...))
		ts.IssueValidationBlockWithHeaderOptions("B.5", nodeB, mock.WithStrongParents(ts.BlockIDs("A.2", "B.2")...))

		ts.IssueValidationBlockWithHeaderOptions("A.7", nodeA, mock.WithStrongParents(ts.BlockIDs("A.6", "B.5")...))
		ts.IssueValidationBlockWithHeaderOptions("B.6", nodeB, mock.WithStrongParents(ts.BlockIDs("A.6", "B.5")...))

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
		ts.AssertSpendersInCacheAcceptanceState([]string{"tx1", "tx2"}, acceptance.Accepted, nodeA, nodeB)
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
				15,
				15,
				2,
				5,
				testsuite.DefaultEpochNearingThreshold,
			),
		),
	)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddValidatorNode("node2")
	wallet := ts.AddDefaultWallet(node1)

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		node1.Validator.AccountID,
		node2.Validator.AccountID,
	}, ts.Nodes()...)

	genesisCommitment := lo.PanicOnErr(node1.Protocol.Engines.Main.Get().Storage.Commitments().Load(0)).Commitment()

	// Create and issue double spends
	{
		tx1 := wallet.CreateBasicOutputsEquallyFromInput("tx1", 1, "Genesis:0")
		tx2 := wallet.CreateBasicOutputsEquallyFromInput("tx2", 1, "Genesis:0")

		wallet.SetDefaultNode(node1)
		ts.SetCurrentSlot(1)
		ts.IssueBasicBlockWithOptions("block1.1", wallet, tx1, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockID("Genesis")))
		ts.IssueBasicBlockWithOptions("block1.2", wallet, tx2, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockID("Genesis")))
		ts.SetCurrentSlot(2)
		ts.IssueValidationBlockWithHeaderOptions("block2.tx1", node1, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs("block1.1")...))

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
		ts.IssueValidationBlockWithHeaderOptions("block2.1", node1, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs("block1.1")...))
		ts.IssueValidationBlockWithHeaderOptions("block2.2", node1, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs("block1.2")...))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block2.1"):   {"tx1"},
			ts.Block("block2.2"):   {"tx2"},
			ts.Block("block2.tx1"): {"tx1"},
		}, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)
	}

	// Issue valid blocks that resolve the conflict.
	{
		ts.IssueValidationBlockWithHeaderOptions("block2.3", node2, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs("block2.2")...))
		ts.IssueValidationBlockWithHeaderOptions("block2.4", node1, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs("block2.3")...))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block2.3"):   {"tx2"},
			ts.Block("block2.tx1"): {"tx1"},
		}, node1, node2)
		ts.AssertTransactionsInCacheAccepted(wallet.Transactions("tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheRejected(wallet.Transactions("tx1"), true, node1, node2)
	}

	// Advance both nodes at the edge of slot 1 committability
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{2, 3, 4}, 1, "block2.4", ts.Nodes("node1", "node2"), false, false)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(0),
			testsuite.WithEqualStoredCommitmentAtIndex(0),
			testsuite.WithEvictedSlot(0),
		)

		ts.SetCurrentSlot(5)
		ts.IssueValidationBlockWithHeaderOptions("block5.1", node1, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDsWithPrefix("block1.1")...))
		ts.IssueValidationBlockWithHeaderOptions("block5.2", node1, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDsWithPrefix("block1.2")...))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block5.1"):   {"tx1"}, // on rejected conflict
			ts.Block("block5.2"):   {},      // accepted merged-to-master
			ts.Block("block2.tx1"): {"tx1"},
		}, node1, node2)

		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{5}, 1, "4.0", ts.Nodes("node1"), false, false)

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
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{5}, 1, "5.0", ts.Nodes("node2"), false, false)

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

	commitment1 := lo.PanicOnErr(node2.Protocol.Engines.Main.Get().Storage.Commitments().Load(1)).Commitment()

	// This should be booked on the rejected tx1 conflict
	tx4 := wallet.CreateBasicOutputsEquallyFromInput("tx4", 1, "tx1:0")

	// Issue TX4 on top of rejected TX1 and 1 commitment on node2 (committed to slot 1)
	{
		wallet.SetDefaultNode(node2)
		ts.IssueBasicBlockWithOptions("n2-commit1", wallet, tx4, mock.WithSlotCommitment(commitment1))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("n2-commit1"): {}, // no conflicts inherited as the block is invalid and doesn't get booked.
			ts.Block("block2.tx1"): {"tx1"},
		}, node2)

		ts.AssertTransactionsExist(wallet.Transactions("tx1"), true, node2)
		ts.AssertTransactionsInCacheRejected(wallet.Transactions("tx4"), true, node2)
		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx4"), true, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx4"), true, node2)

		// As the block commits to 1 but spending something orphaned in 1 it should be invalid
		ts.AssertBlocksInCacheBooked(ts.Blocks("n2-commit1"), false, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n2-commit1"), true, node2)
	}

	// Issue a block on node1 that inherits a rejected, but not yet orphaned conflict that has been orphaned on node2
	{
		ts.IssueValidationBlockWithHeaderOptions("n1-rejected-genesis", node1, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs("block2.tx1")...))

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
		ts.IssueBasicBlockWithOptions("n2-genesis", wallet, tx4, mock.WithStrongParents(ts.BlockID("Genesis")), mock.WithSlotCommitment(genesisCommitment))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("n2-genesis"): {"tx4"}, // on rejected conflict
		}, node2)

		ts.AssertBlocksInCacheBooked(ts.Blocks("n2-genesis"), true, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n2-genesis"), false, node2)
	}

	// Issue TX4 on top of rejected TX1 but Genesis commitment on node1 (committed to slot 0)
	{
		wallet.SetDefaultNode(node1)
		ts.IssueBasicBlockWithOptions("n1-genesis", wallet, tx4, mock.WithStrongParents(ts.BlockID("Genesis")), mock.WithSlotCommitment(genesisCommitment))

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

	// Sync up the nodes to the same point and check the consistency between them.
	{
		// Let node1 catch up with commitment 1
		ts.IssueBlocksAtSlots("5.1", []iotago.SlotIndex{5}, 1, "5.0", ts.Nodes("node2"), false, false)

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

		ts.IssueValidationBlockWithHeaderOptions("n1-rejected-commit1", node1, mock.WithSlotCommitment(commitment1), mock.WithStrongParents(ts.BlockIDs("n1-rejected-genesis")...))
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

		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{6, 7, 8, 9, 10}, 5, "5.1", ts.Nodes("node1", "node2"), false, false)

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
				15,
				15,
				2,
				5,
				testsuite.DefaultEpochNearingThreshold,
			),
		),
	)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddValidatorNode("node2")
	wallet := ts.AddDefaultWallet(node1)

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		node1.Validator.AccountID,
		node2.Validator.AccountID,
	}, ts.Nodes()...)

	genesisCommitment := lo.PanicOnErr(node1.Protocol.Engines.Main.Get().Storage.Commitments().Load(0)).Commitment()

	// Create and issue double spends
	{
		tx1 := wallet.CreateBasicOutputsEquallyFromInput("tx1", 1, "Genesis:0")
		tx2 := wallet.CreateBasicOutputsEquallyFromInput("tx2", 1, "Genesis:0")

		wallet.SetDefaultNode(node2)
		ts.SetCurrentSlot(1)
		ts.IssueBasicBlockWithOptions("block1.1", wallet, tx1, mock.WithStrongParents(ts.BlockID("Genesis")))
		ts.IssueBasicBlockWithOptions("block1.2", wallet, tx2, mock.WithStrongParents(ts.BlockID("Genesis")))

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
		ts.SetCurrentSlot(2)
		ts.IssueValidationBlockWithHeaderOptions("block2.1", node2, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockID("block1.1")))
		ts.IssueValidationBlockWithHeaderOptions("block2.2", node2, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockID("block1.2")))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block2.1"): {"tx1"},
			ts.Block("block2.2"): {"tx2"},
		}, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)
	}

	// Advance both nodes at the edge of slot 1 committability
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{2, 3, 4}, 1, "Genesis", ts.Nodes("node1", "node2"), false, false)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(0),
			testsuite.WithEqualStoredCommitmentAtIndex(0),
			testsuite.WithEvictedSlot(0),
		)

		ts.SetCurrentSlot(5)
		ts.IssueValidationBlockWithHeaderOptions("", node1, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDsWithPrefix("4.0")...))

		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{5}, 1, "4.0", ts.Nodes("node1"), false, false)

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
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{5}, 1, "5.0", ts.Nodes("node2"), false, false)

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

	commitment1 := lo.PanicOnErr(node2.Protocol.Engines.Main.Get().Storage.Commitments().Load(1)).Commitment()

	// Issue a block booked on a pending conflict on node2
	{
		ts.IssueValidationBlockWithHeaderOptions("n2-pending-genesis", node2, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs("block2.1")...))
		ts.IssueValidationBlockWithHeaderOptions("n2-pending-commit1", node2, mock.WithSlotCommitment(commitment1), mock.WithStrongParents(ts.BlockIDs("block2.1")...))

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

	// Sync up the nodes to he same point and check consistency between them.+
	{
		// Let node1 catch up with commitment 1
		ts.IssueBlocksAtSlots("5.1", []iotago.SlotIndex{5}, 1, "5.0", ts.Nodes("node2"), false, false)

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

		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{6, 7, 8, 9, 10}, 5, "5.1", ts.Nodes("node1", "node2"), false, false)

		ts.AssertNodeState(ts.Nodes("node1", "node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(8),
			testsuite.WithEqualStoredCommitmentAtIndex(8),
			testsuite.WithEvictedSlot(8),
		)

		ts.AssertTransactionsExist(wallet.Transactions("tx1", "tx2"), false, node1, node2)
	}
}

func Test_ReattachRejectedCommittedRace(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(20, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				testsuite.DefaultSlotsPerEpochExponent,
			),
			iotago.WithLivenessOptions(
				15,
				15,
				2,
				5,
				testsuite.DefaultEpochNearingThreshold,
			),
		),
	)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddValidatorNode("node2")
	wallet := ts.AddDefaultWallet(node1)

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		node1.Validator.AccountID,
		node2.Validator.AccountID,
	}, ts.Nodes()...)

	genesisCommitment := lo.PanicOnErr(node1.Protocol.Engines.Main.Get().Storage.Commitments().Load(0)).Commitment()

	tx1 := wallet.CreateBasicOutputsEquallyFromInput("tx1", 1, "Genesis:0")
	tx2 := wallet.CreateBasicOutputsEquallyFromInput("tx2", 1, "Genesis:0")
	// Create and issue double spends
	{
		wallet.SetDefaultNode(node1)
		ts.SetCurrentSlot(1)
		ts.IssueBasicBlockWithOptions("block1.1", wallet, tx1, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockID("Genesis")))
		ts.IssueBasicBlockWithOptions("block1.2", wallet, tx2, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockID("Genesis")))
		ts.SetCurrentSlot(2)
		ts.IssueValidationBlockWithHeaderOptions("block2.tx1", node1, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs("block1.1")...))

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
		ts.IssueValidationBlockWithHeaderOptions("block2.1", node1, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs("block1.1")...))
		ts.IssueValidationBlockWithHeaderOptions("block2.2", node1, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs("block1.2")...))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block2.1"):   {"tx1"},
			ts.Block("block2.2"):   {"tx2"},
			ts.Block("block2.tx1"): {"tx1"},
		}, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2"), true, node1, node2)
	}

	// Issue valid blocks that resolve the conflict.
	{
		ts.IssueValidationBlockWithHeaderOptions("block2.3", node2, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs("block2.2")...))
		ts.IssueValidationBlockWithHeaderOptions("block2.4", node1, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs("block2.3")...))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block2.3"):   {"tx2"},
			ts.Block("block2.tx1"): {"tx1"},
		}, node1, node2)
		ts.AssertTransactionsInCacheAccepted(wallet.Transactions("tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheRejected(wallet.Transactions("tx1"), true, node1, node2)
	}

	// Advance both nodes at the edge of slot 1 committability
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{2, 3, 4}, 1, "block2.4", ts.Nodes("node1", "node2"), false, false)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(0),
			testsuite.WithEqualStoredCommitmentAtIndex(0),
			testsuite.WithEvictedSlot(0),
		)

		ts.SetCurrentSlot(5)
		ts.IssueValidationBlockWithHeaderOptions("block5.1", node1, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDsWithPrefix("block1.1")...))
		ts.IssueValidationBlockWithHeaderOptions("block5.2", node1, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDsWithPrefix("block1.2")...))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block5.1"):   {"tx1"}, // on rejected conflict
			ts.Block("block5.2"):   {},      // accepted merged-to-master
			ts.Block("block2.tx1"): {"tx1"},
		}, node1, node2)

		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{5}, 1, "4.0", ts.Nodes("node1"), false, false)

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
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{5}, 1, "5.0", ts.Nodes("node2"), false, false)

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

	commitment1 := lo.PanicOnErr(node2.Protocol.Engines.Main.Get().Storage.Commitments().Load(1)).Commitment()

	// This should be booked on the rejected tx1 conflict

	// Issue a reattachment of the rejected TX1 with 1 commitment on node2 (committed to slot 1)
	{
		wallet.SetDefaultNode(node2)
		ts.IssueBasicBlockWithOptions("n2-rejected-tx1-commit1", wallet, tx1, mock.WithSlotCommitment(commitment1))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("n2-rejected-tx1-commit1"): {}, // no conflicts inherited as the block is invalid and doesn't get booked.
			ts.Block("block2.tx1"):              {"tx1"},
		}, node2)

		ts.AssertTransactionsExist(wallet.Transactions("tx1"), true, node2)
		ts.AssertTransactionsInCacheRejected(wallet.Transactions("tx1"), true, node2)
		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx1"), true, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1"), true, node2)

		// As the block commits to 1 but spending something orphaned in 1 it should be invalid
		ts.AssertBlocksInCacheBooked(ts.Blocks("n2-rejected-tx1-commit1"), false, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n2-rejected-tx1-commit1"), true, node2)
	}

	// Issue a reattachment of the rejected, but not yet orphaned TX1 that has been orphaned on node2
	{
		ts.IssueValidationBlockWithHeaderOptions("n1-rejected-tx1-genesis", node1, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs("block2.tx1")...))

		ts.AssertBlocksInCacheBooked(ts.Blocks("n1-rejected-tx1-genesis"), true, node1)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n1-rejected-tx1-genesis"), false, node1)

		ts.AssertTransactionsInCacheRejected(wallet.Transactions("tx1"), true, node2)

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("block2.tx1"):              {"tx1"},
			ts.Block("n1-rejected-tx1-genesis"): {"tx1"}, // on rejected conflict
		}, node1)
	}

	// Issue reattachment of TX1 but Genesis commitment on node2 (committed to slot 1)
	{
		wallet.SetDefaultNode(node2)
		ts.IssueBasicBlockWithOptions("n2-tx1-genesis", wallet, tx1, mock.WithStrongParents(ts.BlockID("Genesis")), mock.WithSlotCommitment(genesisCommitment))

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("n2-tx1-genesis"): {"tx1"}, // on rejected conflict
		}, node2)

		ts.AssertBlocksInCacheBooked(ts.Blocks("n2-tx1-genesis"), true, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n2-tx1-genesis"), false, node2)
	}

	// Issue reattachment of TX1 but Genesis commitment on node1 (committed to slot 0)
	{
		wallet.SetDefaultNode(node1)
		ts.IssueBasicBlockWithOptions("n1-tx1-genesis", wallet, tx1, mock.WithStrongParents(ts.BlockID("Genesis")), mock.WithSlotCommitment(genesisCommitment))

		ts.AssertTransactionsExist(wallet.Transactions("tx1"), true, node2)
		ts.AssertTransactionsInCacheRejected(wallet.Transactions("tx1"), true, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1"), true, node2)

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("n1-tx1-genesis"): {"tx1"}, // on rejected conflict
		}, node1)

		ts.AssertBlocksInCacheBooked(ts.Blocks("n1-tx1-genesis"), true, node1)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n1-tx1-genesis"), false, node1)
	}

	ts.MergePartitionsToMain(lo.Keys(partitions)...)

	// Sync up the nodes to the same point and check the consistency between them.
	{
		// Let node1 catch up with commitment 1
		ts.IssueBlocksAtSlots("5.1", []iotago.SlotIndex{5}, 1, "5.0", ts.Nodes("node2"), false, false)

		ts.AssertNodeState(ts.Nodes("node1", "node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(1),
			testsuite.WithEqualStoredCommitmentAtIndex(1),
			testsuite.WithEvictedSlot(1),
		)

		// Exchange each-other blocks, ignoring invalidity
		wallet.SetDefaultNode(node1)
		ts.IssueExistingBlock("n2-tx1-genesis", wallet)
		ts.IssueExistingBlock("n2-rejected-tx1-commit1", wallet)
		wallet.SetDefaultNode(node2)
		ts.IssueExistingBlock("n1-tx1-genesis", wallet)
		ts.IssueExistingBlock("n1-rejected-tx1-genesis", wallet)

		ts.IssueValidationBlockWithHeaderOptions("n1-rejected-tx1-commit1", node1, mock.WithSlotCommitment(commitment1), mock.WithStrongParents(ts.BlockIDs("n1-tx1-genesis")...))
		// Needs reissuing on node2 because it is invalid
		ts.IssueExistingBlock("n1-rejected-tx1-commit1", wallet)

		// The nodes agree on the results of the invalid blocks
		ts.AssertBlocksInCacheBooked(ts.Blocks("n2-tx1-genesis", "n1-tx1-genesis", "n1-rejected-tx1-genesis"), true, node1, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n2-tx1-genesis", "n1-tx1-genesis", "n1-rejected-tx1-genesis"), false, node1, node2)

		// This block propagates the orphaned conflict from Tangle
		ts.AssertBlocksInCacheBooked(ts.Blocks("n1-rejected-tx1-commit1"), true, node1, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n1-rejected-tx1-commit1"), false, node1, node2)

		// This block spends an orphaned conflict from its Transaction
		ts.AssertBlocksInCacheBooked(ts.Blocks("n2-rejected-tx1-commit1"), false, node1, node2)
		ts.AssertBlocksInCacheInvalid(ts.Blocks("n2-rejected-tx1-commit1"), true, node1, node2)

		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("n1-tx1-genesis"):          {"tx1"}, // on rejected conflict
			ts.Block("n2-tx1-genesis"):          {"tx1"}, // on rejected conflict
			ts.Block("n1-rejected-tx1-genesis"): {"tx1"}, // on rejected conflict
			ts.Block("n2-rejected-tx1-commit1"): {},      // invalid block
			ts.Block("n1-rejected-tx1-commit1"): {},      // merged-to-master
		}, node1, node2)
	}

	// Commit further and test eviction of transactions
	{
		ts.AssertTransactionsExist(wallet.Transactions("tx1", "tx2"), true, node1, node2)

		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{6, 7, 8, 9, 10}, 5, "5.1", ts.Nodes("node1", "node2"), false, false)

		ts.AssertNodeState(ts.Nodes("node1", "node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(8),
			testsuite.WithEqualStoredCommitmentAtIndex(8),
			testsuite.WithEvictedSlot(8),
		)

		ts.AssertTransactionsExist(wallet.Transactions("tx1", "tx2"), false, node1, node2)
	}
}

func Test_RootBlockShallowLike(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(1000, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				3,
			),
			iotago.WithLivenessOptions(
				10,
				10,
				2,
				4,
				5,
			),
		),

		testsuite.WithWaitFor(5*time.Second),
	)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	wallet := ts.AddDefaultWallet(node1)
	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	tx1 := wallet.CreateBasicOutputsEquallyFromInput("tx1", 1, "Genesis:0")

	ts.IssueBasicBlockWithOptions("block1", wallet, tx1, mock.WithIssuingTime(ts.API.TimeProvider().SlotStartTime(1)))
	ts.IssueBasicBlockWithOptions("block2", wallet, &iotago.TaggedData{}, mock.WithIssuingTime(ts.API.TimeProvider().SlotStartTime(1)))

	ts.AssertTransactionsExist(wallet.Transactions("tx1"), true, node1)

	ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
		ts.Block("block1"): {"tx1"},
	}, node1)

	ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
		wallet.Transaction("tx1"): {"tx1"},
	}, node1)

	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{2, 3, 4}, 2, "block", ts.Nodes(), true, false)

	ts.AssertActiveRootBlocks(append(ts.Blocks("Genesis", "block1", "block2"), ts.BlocksWithPrefix("2.")...), ts.Nodes()...)

	ts.IssueBasicBlockWithOptions("block-shallow-like-valid", wallet, &iotago.TaggedData{}, mock.WithStrongParents(ts.BlockID("4.1-node1")), mock.WithShallowLikeParents(ts.BlockID("block1")), mock.WithIssuingTime(ts.API.TimeProvider().SlotStartTime(5)))
	ts.AssertBlocksInCacheBooked(ts.Blocks("block-shallow-like-valid"), true, node1)
	ts.AssertBlocksInCacheInvalid(ts.Blocks("block-shallow-like-valid"), false, node1)

	ts.IssueBasicBlockWithOptions("block-shallow-like-invalid", wallet, &iotago.TaggedData{}, mock.WithStrongParents(ts.BlockID("4.1-node1")), mock.WithShallowLikeParents(ts.BlockID("block2")), mock.WithIssuingTime(ts.API.TimeProvider().SlotStartTime(5)))
	ts.AssertBlocksInCacheBooked(ts.Blocks("block-shallow-like-invalid"), false, node1)
	ts.AssertBlocksInCacheInvalid(ts.Blocks("block-shallow-like-invalid"), true, node1)
}

func Test_BlockWithInvalidTransactionGetsBooked(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(1000, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				13,
			),
		),
	)

	node1 := ts.AddValidatorNode("node1")
	ts.AddNode("node2")
	ts.AddDefaultWallet(node1)

	ts.Run(true)
	defer ts.Shutdown()

	// CREATE NFT FROM BASIC UTXO
	var block1Slot iotago.SlotIndex = ts.API.ProtocolParameters().GenesisSlot() + 1
	ts.SetCurrentSlot(block1Slot)

	tx1 := ts.DefaultWallet().CreateNFTFromInput("TX1", "Genesis:0",
		func(nftBuilder *builder.NFTOutputBuilder) {
			// Set an issuer ID that is not unlocked in the TX which will cause the TX to be invalid.
			nftBuilder.ImmutableIssuer(&iotago.Ed25519Address{})
		},
	)
	block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1)

	vblock1 := ts.IssueValidationBlockWithHeaderOptions("vblock1", ts.DefaultWallet().Node, mock.WithWeakParents(block1.ID()), mock.WithStrongParents(ts.Block("Genesis").ID()))
	vblock2 := ts.IssueValidationBlockWithHeaderOptions("vblock2", ts.DefaultWallet().Node, mock.WithStrongParents(vblock1.ID()))
	vblock3 := ts.IssueValidationBlockWithHeaderOptions("vblock3", ts.DefaultWallet().Node, mock.WithStrongParents(vblock2.ID()))

	ts.AssertBlocksInCacheAccepted(ts.Blocks("block1"), true, ts.Nodes()...)
	ts.AssertBlocksInCacheConfirmed(ts.Blocks("block1"), true, ts.Nodes()...)

	ts.AssertTransactionsExist([]*iotago.Transaction{tx1.Transaction}, true, ts.Nodes()...)
	ts.AssertTransactionFailure(lo.PanicOnErr(tx1.ID()), iotago.ErrIssuerFeatureNotUnlocked, ts.Nodes()...)
	ts.AssertTransactionsInCacheAccepted([]*iotago.Transaction{tx1.Transaction}, false, ts.Nodes()...)

	ts.CommitUntilSlot(block1Slot, vblock3.ID())

	ts.AssertStorageCommitmentBlockAccepted(block1Slot, block1.ID(), true, ts.Nodes()...)
	ts.AssertStorageCommitmentTransactionAccepted(block1Slot, tx1.Transaction.MustID(), false, ts.Nodes()...)
}

// This test checks that a fresh attachment of an orphaned transaction before it's evicted, can mark it as unorphaned and successfully accept it.
// On the other node, the transaction is evicted before a fresh attachment arrives,
// simulating a possible race that could cause a divergence if both cases weren't handled correctly.
// In the end, both nodes accept the transaction,
// commit it and then commit far enough for the committed transaction to be fully evicted.
func Test_OrphanedTransactionRace_Scenario1(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(20, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				testsuite.DefaultSlotsPerEpochExponent,
			),
			iotago.WithLivenessOptions(
				15,
				15,
				2,
				5,
				testsuite.DefaultEpochNearingThreshold,
			),
		),
	)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddNode("node2")
	wallet := ts.AddDefaultWallet(node1)

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		node1.Validator.AccountID,
	}, ts.Nodes()...)

	tx1 := wallet.CreateBasicOutputsEquallyFromInput("tx1", 1, "Genesis:0")

	// Create a transaction to be orphaned.
	{

		ts.SetCurrentSlot(1)
		ts.IssueBasicBlockWithOptions("attachment-1-tx1-orphaned", wallet, tx1, mock.WithStrongParents(ts.BlockID("Genesis")))

		ts.AssertTransactionsExist(wallet.Transactions("tx1"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1"), true, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("attachment-1-tx1-orphaned"): {"tx1"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			wallet.Transaction("tx1"): {"tx1"},
		}, node1, node2)
	}

	// Advance both nodes at the edge of slot 1 evictability.
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{2, 3, 4, 5, 6, 7, 8}, 1, "Genesis", ts.Nodes("node1"), false, false)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(5),
			testsuite.WithEqualStoredCommitmentAtIndex(5),
			testsuite.WithEvictedSlot(5),
		)

		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx1"), true, node1, node2)
	}

	partitions := map[string][]*mock.Node{
		"node1": {node1},
		"node2": {node2},
	}

	// Split the nodes into partitions and commit slot 1 only on node2
	{
		ts.SplitIntoPartitions(partitions)

		// Issue validation blocks on the validator node so that it commits further and performs the delayed eviction of TX1 from the mempool.
		ts.IssueBlocksAtSlots("more-", []iotago.SlotIndex{8}, 3, "8.0", ts.Nodes("node1"), true, true)

		ts.AssertNodeState(ts.Nodes("node1"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(6),
			testsuite.WithEqualStoredCommitmentAtIndex(6),
			testsuite.WithEvictedSlot(6),
		)
		ts.AssertTransactionsExist(wallet.Transactions("tx1"), false, node1)

		//Node 2 didn't perform the delayed eviction of TX1.
		ts.AssertNodeState(ts.Nodes("node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(5),
			testsuite.WithEqualStoredCommitmentAtIndex(5),
			testsuite.WithEvictedSlot(5),
		)

		ts.AssertTransactionsExist(wallet.Transactions("tx1"), true, node2)
		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx1"), true, node2)

	}

	// Issue the block on two partitions, to simulate a situation in which one node already committed and evicted the transaction,
	// while on the other node transaction is still present.
	{
		wallet.SetDefaultNode(node2)
		ts.SetCurrentSlot(8)
		ts.IssueBasicBlockWithOptions("more-attachment-2-tx1", wallet, tx1, mock.WithStrongParents(ts.BlockIDsWithPrefix("8.0")...), mock.WithSlotCommitment(ts.CommitmentOfMainEngine(node1, 5).Commitment()))

		wallet.SetDefaultNode(node1)
		ts.IssueExistingBlock("more-attachment-2-tx1", wallet)

		// Both nodes agree on the perception of the transaction.
		ts.AssertBlocksInCacheInvalid(ts.Blocks("more-attachment-2-tx1"), false, ts.Nodes()...)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1"), true, ts.Nodes()...)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1"), true, ts.Nodes()...)
		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx1"), false, ts.Nodes()...)
	}

	ts.MergePartitionsToMain(lo.Keys(partitions)...)
	fmt.Println("------ MERGE ------")
	// Sync up the nodes to the same point and check the consistency between them. TX1 is accepted this time.
	{
		// Let node1 catch up with commitment 1
		ts.IssueBlocksAtSlots("after-merge", []iotago.SlotIndex{8, 9, 10}, 3, "more-", ts.Nodes("node1"), true, true)

		ts.AssertNodeState(ts.Nodes("node1", "node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(8),
			testsuite.WithEqualStoredCommitmentAtIndex(8),
			testsuite.WithEvictedSlot(8),
		)
		ts.AssertEqualStoredCommitmentAtIndex(8, ts.Nodes()...)

		ts.AssertStorageCommitmentBlockAccepted(8, ts.BlockID("more-attachment-2-tx1"), true, ts.Nodes()...)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1"), true, ts.Nodes()...)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1"), false, ts.Nodes()...)
		ts.AssertTransactionsInCacheAccepted(wallet.Transactions("tx1"), true, ts.Nodes()...)

		ts.AssertStorageCommitmentTransactionAccepted(8, wallet.TransactionID("tx1"), true, ts.Nodes()...)

		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx1"), false, node1)
		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx1"), false, node2)
	}

	// Commit further so that transaction is evicted after maxCommittableAge delay.
	{
		ts.IssueBlocksAtSlots("after-merge", []iotago.SlotIndex{11, 12, 13, 14, 15}, 3, "after-merge10.2", ts.Nodes("node1"), true, true)

		ts.AssertNodeState(ts.Nodes("node1", "node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(13),
			testsuite.WithEqualStoredCommitmentAtIndex(13),
			testsuite.WithEvictedSlot(13),
		)
		ts.AssertEqualStoredCommitmentAtIndex(13, ts.Nodes()...)
		ts.AssertTransactionsExist(wallet.Transactions("tx1"), false, ts.Nodes()...)
	}
}

// This test checks, similar to the previous one,
// that a fresh attachment of an orphaned transaction before it's evicted, can mark it as unorphaned and then orphaned again.
// On the other node, the transaction is evicted before a fresh attachment arrives,
// simulating a possible race that could cause a divergence if both cases weren't handled correctly.
// In the end, both nodes orphaned the transaction again and evicted it from the cache.
func Test_OrphanedTransactionRace_Scenario2(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(20, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				testsuite.DefaultSlotsPerEpochExponent,
			),
			iotago.WithLivenessOptions(
				15,
				15,
				2,
				5,
				testsuite.DefaultEpochNearingThreshold,
			),
		),
	)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddNode("node2")
	wallet := ts.AddDefaultWallet(node1)

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	node2.Protocol.SetLogLevel(log.LevelTrace)

	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		node1.Validator.AccountID,
	}, ts.Nodes()...)

	tx1 := wallet.CreateBasicOutputsEquallyFromInput("tx1", 1, "Genesis:0")

	// Create a transaction to be orphaned.
	{

		ts.SetCurrentSlot(1)
		ts.IssueBasicBlockWithOptions("attachment-1-tx1-orphaned", wallet, tx1, mock.WithStrongParents(ts.BlockID("Genesis")))

		ts.AssertTransactionsExist(wallet.Transactions("tx1"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1"), true, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("attachment-1-tx1-orphaned"): {"tx1"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			wallet.Transaction("tx1"): {"tx1"},
		}, node1, node2)
	}

	// Advance both nodes at the edge of slot 1 committability
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{2, 3, 4, 5, 6, 7, 8}, 1, "Genesis", ts.Nodes("node1"), false, false)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(5),
			testsuite.WithEqualStoredCommitmentAtIndex(5),
			testsuite.WithEvictedSlot(5),
		)

		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx1"), true, node1, node2)
	}

	partitions := map[string][]*mock.Node{
		"node1": {node1},
		"node2": {node2},
	}

	// Split the nodes into partitions and commit slot 1 only on node2
	{
		ts.SplitIntoPartitions(partitions)

		// Issue validation blocks on the validator node so that it commits further and performs the delayed eviction of TX1 from the mempool.
		ts.IssueBlocksAtSlots("more-", []iotago.SlotIndex{8}, 3, "8.0", ts.Nodes("node1"), true, true)

		ts.AssertNodeState(ts.Nodes("node1"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(6),
			testsuite.WithEqualStoredCommitmentAtIndex(6),
			testsuite.WithEvictedSlot(6),
		)
		ts.AssertTransactionsExist(wallet.Transactions("tx1"), false, node1)

		//Node 2 didn't perform the delayed eviction of TX1.
		ts.AssertNodeState(ts.Nodes("node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(5),
			testsuite.WithEqualStoredCommitmentAtIndex(5),
			testsuite.WithEvictedSlot(5),
		)

		ts.AssertTransactionsExist(wallet.Transactions("tx1"), true, node2)
		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx1"), true, node2)

	}

	// Issue the block on two partitions, to simulate a situation in which one node already committed and evicted the transaction,
	// while on the other node transaction is still present.
	{
		wallet.SetDefaultNode(node2)
		ts.SetCurrentSlot(8)
		ts.IssueBasicBlockWithOptions("attachment-2-tx1-orphaned", wallet, tx1, mock.WithStrongParents(ts.BlockIDsWithPrefix("8.0")...), mock.WithSlotCommitment(ts.CommitmentOfMainEngine(node1, 5).Commitment()))

		wallet.SetDefaultNode(node1)
		ts.IssueExistingBlock("attachment-2-tx1-orphaned", wallet)

		ts.AssertBlocksInCacheInvalid(ts.Blocks("attachment-2-tx1-orphaned"), false, ts.Nodes()...)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1"), true, ts.Nodes()...)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1"), true, ts.Nodes()...)
		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx1"), false, ts.Nodes()...)
	}

	ts.MergePartitionsToMain(lo.Keys(partitions)...)
	fmt.Println("------ MERGE ------")
	// Sync up the nodes to the same point and check the consistency between them. TX1 is again orphaned and evicted on both nodes.
	{
		// Let node1 catch up with commitment 1
		ts.IssueBlocksAtSlots("after-merge", []iotago.SlotIndex{8, 9, 10}, 3, "more-", ts.Nodes("node1"), true, true)

		ts.AssertNodeState(ts.Nodes("node1", "node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(8),
			testsuite.WithEqualStoredCommitmentAtIndex(8),
			testsuite.WithEvictedSlot(8),
		)
		ts.AssertEqualStoredCommitmentAtIndex(8, ts.Nodes()...)

		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1"), true, ts.Nodes()...)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1"), true, ts.Nodes()...)
		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx1"), true, ts.Nodes()...)
	}

	// Commit further so that transaction is evicted after maxCommittableAge delay.
	{
		ts.IssueBlocksAtSlots("after-merge", []iotago.SlotIndex{11, 12, 13, 14, 15}, 3, "after-merge10.2", ts.Nodes("node1"), true, true)

		ts.AssertNodeState(ts.Nodes("node1", "node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(13),
			testsuite.WithEqualStoredCommitmentAtIndex(13),
			testsuite.WithEvictedSlot(13),
		)
		ts.AssertEqualStoredCommitmentAtIndex(13, ts.Nodes()...)
		ts.AssertTransactionsExist(wallet.Transactions("tx1"), false, ts.Nodes()...)
	}
}

// This test checks that a fresh attachment of an orphaned transaction before it's evicted, can mark it as unorphaned and successfully accept it.
// This also tests that the chain of transactions, spending from the orphaned transaction is unorphaned
// and then successfully accepted.
// In the end, both nodes accept the transaction,
func Test_OrphanedTransactionReattached_Scenario1(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(20, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				testsuite.DefaultSlotsPerEpochExponent,
			),
			iotago.WithLivenessOptions(
				15,
				15,
				2,
				5,
				testsuite.DefaultEpochNearingThreshold,
			),
		),
	)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddNode("node2")
	wallet := ts.AddDefaultWallet(node1)

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		node1.Validator.AccountID,
	}, ts.Nodes()...)

	tx1 := wallet.CreateBasicOutputsEquallyFromInput("tx1", 1, "Genesis:0")
	tx2 := wallet.CreateBasicOutputsEquallyFromInput("tx2", 1, "tx1:0")
	tx3 := wallet.CreateBasicOutputsEquallyFromInput("tx3", 1, "tx2:0")
	// Create a transaction to be orphaned.
	{

		ts.SetCurrentSlot(1)
		ts.IssueBasicBlockWithOptions("attachment-1-tx1-orphaned", wallet, tx1, mock.WithStrongParents(ts.BlockID("Genesis")))

		ts.AssertTransactionsExist(wallet.Transactions("tx1"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1"), true, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("attachment-1-tx1-orphaned"): {"tx1"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			wallet.Transaction("tx1"): {"tx1"},
		}, node1, node2)
	}

	{
		ts.SetCurrentSlot(2)
		ts.IssueBasicBlockWithOptions("attachment-tx2", wallet, tx2, mock.WithStrongParents(ts.BlockID("Genesis")))

		ts.AssertTransactionsExist(wallet.Transactions("tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx2"), true, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx2"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("attachment-tx2"): {"tx2"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			wallet.Transaction("tx2"): {"tx2"},
		}, node1, node2)
	}

	{
		ts.IssueBasicBlockWithOptions("attachment-tx3", wallet, tx3, mock.WithStrongParents(ts.BlockID("attachment-tx2")))

		ts.AssertTransactionsExist(wallet.Transactions("tx3"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx3"), true, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx3"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("attachment-tx3"): {"tx2", "tx3"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			wallet.Transaction("tx3"): {"tx3"},
		}, node1, node2)
	}

	// Commit slot 1 so that the tx1 becomes orphaned along with its UTXO future cone.
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{2, 3}, 3, "attachment-tx3", ts.Nodes("node1"), false, false)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(1),
			testsuite.WithEqualStoredCommitmentAtIndex(1),
			testsuite.WithEvictedSlot(1),
		)

		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx1", "tx2", "tx3"), true, node1, node2)
	}

	// Issue the reattachment of tx1, which makes
	{
		ts.SetCurrentSlot(2)
		ts.IssueBasicBlockWithOptions("3.2-attachment-2-tx1", wallet, tx1, mock.WithStrongParents(ts.BlockIDsWithPrefix("2.2")...), mock.WithSlotCommitment(ts.CommitmentOfMainEngine(node1, 0).Commitment()))

		ts.AssertBlocksInCacheInvalid(ts.Blocks("3.2-attachment-2-tx1"), false, ts.Nodes()...)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1", "tx2", "tx3"), true, ts.Nodes()...)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2", "tx3"), true, ts.Nodes()...)
		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx1", "tx2", "tx3"), false, ts.Nodes()...)
	}

	// Accept the new attachment of TX1, which triggers acceptance of TX2 and TX3 as a consequence.
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{3}, 3, "3.2", ts.Nodes("node1"), true, true)

		ts.AssertNodeState(ts.Nodes("node1", "node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(1),
			testsuite.WithEqualStoredCommitmentAtIndex(1),
			testsuite.WithEvictedSlot(1),
		)
		ts.AssertEqualStoredCommitmentAtIndex(1, ts.Nodes()...)

		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1", "tx2", "tx3"), true, ts.Nodes()...)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2", "tx3"), false, ts.Nodes()...)
		ts.AssertTransactionsInCacheAccepted(wallet.Transactions("tx1", "tx2", "tx3"), true, ts.Nodes()...)
		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx1", "tx2", "tx3"), false, ts.Nodes()...)
	}
}

// This test checks that a fresh attachment of an orphaned transaction can mark it as unorphaned and then orphaned again.
// This also tests that the chain of transactions, spending from the orphaned transaction is unorphaned
// and then orphaned again.
func Test_OrphanedTransactionReattached_Scenario2(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(20, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				testsuite.DefaultSlotsPerEpochExponent,
			),
			iotago.WithLivenessOptions(
				15,
				15,
				2,
				5,
				testsuite.DefaultEpochNearingThreshold,
			),
		),
	)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddNode("node2")
	wallet := ts.AddDefaultWallet(node1)

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	node2.Protocol.SetLogLevel(log.LevelTrace)

	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		node1.Validator.AccountID,
	}, ts.Nodes()...)

	tx1 := wallet.CreateBasicOutputsEquallyFromInput("tx1", 1, "Genesis:0")
	tx2 := wallet.CreateBasicOutputsEquallyFromInput("tx2", 1, "tx1:0")
	tx3 := wallet.CreateBasicOutputsEquallyFromInput("tx3", 1, "tx2:0")
	// Create a transaction to be orphaned.
	{

		ts.SetCurrentSlot(1)
		ts.IssueBasicBlockWithOptions("attachment-1-tx1-orphaned", wallet, tx1, mock.WithStrongParents(ts.BlockID("Genesis")))

		ts.AssertTransactionsExist(wallet.Transactions("tx1"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1"), true, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("attachment-1-tx1-orphaned"): {"tx1"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			wallet.Transaction("tx1"): {"tx1"},
		}, node1, node2)
	}

	// Issue tx2
	{
		ts.SetCurrentSlot(2)
		ts.IssueBasicBlockWithOptions("attachment-tx2", wallet, tx2, mock.WithStrongParents(ts.BlockID("Genesis")))

		ts.AssertTransactionsExist(wallet.Transactions("tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx2"), true, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx2"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("attachment-tx2"): {"tx2"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			wallet.Transaction("tx2"): {"tx2"},
		}, node1, node2)
	}

	// Issue tx3
	{
		ts.IssueBasicBlockWithOptions("attachment-tx3", wallet, tx3, mock.WithStrongParents(ts.BlockID("attachment-tx2")))

		ts.AssertTransactionsExist(wallet.Transactions("tx3"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx3"), true, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx3"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("attachment-tx3"): {"tx2", "tx3"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			wallet.Transaction("tx3"): {"tx3"},
		}, node1, node2)
	}

	// Commit slot 1 so that tx1 is orphaned along with its UTXO future cone.
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{2, 3}, 3, "Genesis", ts.Nodes("node1"), false, false)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(1),
			testsuite.WithEqualStoredCommitmentAtIndex(1),
			testsuite.WithEvictedSlot(1),
		)

		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx1", "tx2", "tx3"), true, node1, node2)
	}

	// Issue the block on two partitions, to simulate a situation in which one node already committed and evicted the transaction,
	// while on the other node transaction is still present.
	{
		ts.SetCurrentSlot(2)
		ts.IssueBasicBlockWithOptions("3.2-attachment-2-tx1-orphaned", wallet, tx1, mock.WithStrongParents(ts.BlockIDsWithPrefix("2.2")...), mock.WithSlotCommitment(ts.CommitmentOfMainEngine(node1, 0).Commitment()))

		wallet.SetDefaultNode(node1)
		ts.IssueExistingBlock("3.2-attachment-2-tx1-orphaned", wallet)

		ts.AssertBlocksInCacheInvalid(ts.Blocks("attachment-2-tx1-orphaned"), false, ts.Nodes()...)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1", "tx2", "tx3"), true, ts.Nodes()...)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2", "tx3"), true, ts.Nodes()...)
		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx1", "tx2", "tx3"), false, ts.Nodes()...)
	}

	// Commit further so that tx2 and 3 are orphaned, because they belong to slot 2,
	// while tx1 is not orphaned, because the new attachment is in slot 3.
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{4}, 3, "3.2", ts.Nodes("node1"), true, true)

		ts.AssertNodeState(ts.Nodes("node1", "node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(2),
			testsuite.WithEqualStoredCommitmentAtIndex(2),
			testsuite.WithEvictedSlot(2),
		)
		ts.AssertEqualStoredCommitmentAtIndex(2, ts.Nodes()...)

		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1", "tx2", "tx3"), true, ts.Nodes()...)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2", "tx3"), true, ts.Nodes()...)
		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx1"), false, ts.Nodes()...)
		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx2", "tx3"), true, ts.Nodes()...)
	}

	// Commit further so that tx1 becomes orphaned as well.
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{5}, 3, "4.2", ts.Nodes("node1"), true, true)

		ts.AssertNodeState(ts.Nodes("node1", "node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(3),
			testsuite.WithEqualStoredCommitmentAtIndex(3),
			testsuite.WithEvictedSlot(3),
		)
		ts.AssertEqualStoredCommitmentAtIndex(3, ts.Nodes()...)

		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1", "tx2", "tx3"), true, ts.Nodes()...)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2", "tx3"), true, ts.Nodes()...)
		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx1", "tx2", "tx3"), true, ts.Nodes()...)
	}
}

// This test checks that a fresh attachment of an orphaned transaction can mark it as unorphaned and accepted.
// A chain of transaction spending from the first transaction is orphaned, then unorphaned,
// then orphaned again, while the first transaction is accepted.
func Test_OrphanedTransactionReattached_Scenario3(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(20, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				testsuite.DefaultSlotsPerEpochExponent,
			),
			iotago.WithLivenessOptions(
				15,
				15,
				2,
				5,
				testsuite.DefaultEpochNearingThreshold,
			),
		),
	)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddNode("node2")
	wallet := ts.AddDefaultWallet(node1)

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	node2.Protocol.SetLogLevel(log.LevelTrace)

	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		node1.Validator.AccountID,
	}, ts.Nodes()...)

	tx1 := wallet.CreateBasicOutputsEquallyFromInput("tx1", 1, "Genesis:0")
	tx2 := wallet.CreateBasicOutputsEquallyFromInput("tx2", 1, "tx1:0")
	tx3 := wallet.CreateBasicOutputsEquallyFromInput("tx3", 1, "tx2:0")
	// Create a transaction to be orphaned.
	{

		ts.SetCurrentSlot(1)
		ts.IssueBasicBlockWithOptions("attachment-1-tx1-orphaned", wallet, tx1, mock.WithStrongParents(ts.BlockID("Genesis")))

		ts.AssertTransactionsExist(wallet.Transactions("tx1"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1"), true, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("attachment-1-tx1-orphaned"): {"tx1"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			wallet.Transaction("tx1"): {"tx1"},
		}, node1, node2)
	}

	// Issue tx2
	{
		ts.SetCurrentSlot(2)
		ts.IssueBasicBlockWithOptions("attachment-tx2", wallet, tx2, mock.WithStrongParents(ts.BlockID("Genesis")))

		ts.AssertTransactionsExist(wallet.Transactions("tx2"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx2"), true, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx2"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("attachment-tx2"): {"tx2"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			wallet.Transaction("tx2"): {"tx2"},
		}, node1, node2)
	}

	// Issue tx3
	{
		ts.IssueBasicBlockWithOptions("attachment-tx3", wallet, tx3, mock.WithStrongParents(ts.BlockID("attachment-tx2")))

		ts.AssertTransactionsExist(wallet.Transactions("tx3"), true, node1, node2)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx3"), true, node1, node2)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx3"), true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
			ts.Block("attachment-tx3"): {"tx2", "tx3"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
			wallet.Transaction("tx3"): {"tx3"},
		}, node1, node2)
	}

	// Commit slot 1 so that tx1 is orphaned along with its UTXO future cone.
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{2, 3}, 3, "attachment-tx3", ts.Nodes("node1"), false, false)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(1),
			testsuite.WithEqualStoredCommitmentAtIndex(1),
			testsuite.WithEvictedSlot(1),
		)

		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx1", "tx2", "tx3"), true, node1, node2)
	}

	// Issue a new attachment of tx1 in slot 4. This marks all transactions as not orphaned.
	{
		ts.SetCurrentSlot(4)
		ts.IssueBasicBlockWithOptions("attachment-2-tx1", wallet, tx1, mock.WithStrongParents(ts.BlockIDsWithPrefix("3.2")...), mock.WithSlotCommitment(ts.CommitmentOfMainEngine(node1, 1).Commitment()))

		wallet.SetDefaultNode(node1)
		ts.IssueExistingBlock("attachment-2-tx1", wallet)

		ts.AssertBlocksInCacheInvalid(ts.Blocks("attachment-2-tx1"), false, ts.Nodes()...)
		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1", "tx2", "tx3"), true, ts.Nodes()...)
		ts.AssertTransactionsInCachePending(wallet.Transactions("tx1", "tx2", "tx3"), true, ts.Nodes()...)
		// Tx2 and Tx3 cannot be marked as not orphaned, because their earliest attachment belongs to an earlier slot,
		// than the earliest attachment of the transaction they spend from.
		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx1"), false, ts.Nodes()...)
		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx2", "tx3"), true, ts.Nodes()...)
	}

	// Commit further so that tx1 is accepted, which should not trigger acceptance of Tx2 and Tx3 because they're attachment is too old and orphaned.
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{4}, 3, "attachment-2-tx1", ts.Nodes("node1"), true, true)

		ts.AssertNodeState(ts.Nodes("node1", "node2"),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitmentSlotIndex(2),
			testsuite.WithEqualStoredCommitmentAtIndex(2),
			testsuite.WithEvictedSlot(2),
		)
		ts.AssertEqualStoredCommitmentAtIndex(2, ts.Nodes()...)

		ts.AssertTransactionsInCacheBooked(wallet.Transactions("tx1", "tx2", "tx3"), true, ts.Nodes()...)
		ts.AssertTransactionsInCacheAccepted(wallet.Transactions("tx1"), true, ts.Nodes()...)
		ts.AssertTransactionsInCacheOrphaned(wallet.Transactions("tx2", "tx3"), false, ts.Nodes()...)
	}
}

// TODO: balanced StateDiff in the mempool

// TODO: UTXO ledger should react when we try to delete an output that does not exist
