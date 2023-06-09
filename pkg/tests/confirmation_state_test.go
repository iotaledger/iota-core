package tests

import (
	"testing"
	"time"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection/poa"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestConfirmationFlags(t *testing.T) {
	ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	nodeA := ts.AddValidatorNode("nodeA", 25)
	nodeB := ts.AddValidatorNode("nodeB", 25)
	nodeC := ts.AddValidatorNode("nodeC", 25)
	nodeD := ts.AddValidatorNode("nodeD", 25)

	expectedOnlineCommittee := map[iotago.AccountID]int64{
		nodeA.AccountID: 25,
	}

	expectedCommittee := map[iotago.AccountID]int64{
		nodeA.AccountID: 25,
		nodeB.AccountID: 25,
		nodeC.AccountID: 25,
		nodeD.AccountID: 25,
	}
	ts.Run(map[string][]options.Option[protocol.Protocol]{
		"nodeA": {
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(1),
			),
			protocol.WithSybilProtectionProvider(
				poa.NewProvider(expectedCommittee, poa.WithOnlineCommitteeStartup(nodeA.AccountID), poa.WithActivityWindow(2*time.Minute)),
			),
		},
		"nodeB": {
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(1),
			),
			protocol.WithSybilProtectionProvider(
				poa.NewProvider(expectedCommittee, poa.WithOnlineCommitteeStartup(nodeA.AccountID), poa.WithActivityWindow(2*time.Minute)),
			),
		},
		"nodeC": {
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(1),
			),
			protocol.WithSybilProtectionProvider(
				poa.NewProvider(expectedCommittee, poa.WithOnlineCommitteeStartup(nodeA.AccountID), poa.WithActivityWindow(2*time.Minute)),
			),
		},
		"nodeD": {
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(1),
			),
			protocol.WithSybilProtectionProvider(
				poa.NewProvider(expectedCommittee, poa.WithOnlineCommitteeStartup(nodeA.AccountID), poa.WithActivityWindow(2*time.Minute)),
			),
		},
	})
	ts.HookLogging()

	// Verify that nodes have the expected states.
	ts.AssertNodeState(ts.Nodes(),
		testsuite.WithSnapshotImported(true),
		testsuite.WithProtocolParameters(ts.ProtocolParameters),
		testsuite.WithLatestCommitment(iotago.NewEmptyCommitment()),
		testsuite.WithLatestStateMutationSlot(0),
		testsuite.WithLatestFinalizedSlot(0),
		testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
		testsuite.WithStorageCommitments([]*iotago.Commitment{iotago.NewEmptyCommitment()}),
		testsuite.WithSybilProtectionCommittee(expectedCommittee),
		testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommittee),
		testsuite.WithEvictedSlot(0),
		testsuite.WithActiveRootBlocks(ts.Blocks("Genesis")),
		testsuite.WithStorageRootBlocks(ts.Blocks("Genesis")),
	)

	// Slots 1-3: only node A is online and issues blocks, make slot 1 committed.
	{
		ts.IssueBlockAtSlot("A.1.0", 1, iotago.NewEmptyCommitment(), nodeA, ts.BlockID("Genesis"))
		ts.IssueBlockAtSlot("A.1.1", 1, iotago.NewEmptyCommitment(), nodeA, ts.BlockID("A.1.0"))
		ts.IssueBlockAtSlot("A.2.0", 2, iotago.NewEmptyCommitment(), nodeA, ts.BlockID("A.1.1"))
		ts.IssueBlockAtSlot("A.2.1", 2, iotago.NewEmptyCommitment(), nodeA, ts.BlockID("A.2.0"))
		ts.IssueBlockAtSlot("A.3.0", 3, iotago.NewEmptyCommitment(), nodeA, ts.BlockID("A.2.1"))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.1.0", "A.1.1", "A.2.0", "A.2.1", "A.3.0"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("A.1.0", "A.1.1", "A.2.0", "A.2.1"), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("A.1.0", "A.1.1", "A.2.0", "A.2.1", "A.3.0"), false, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("A.1.0", "A.1.1", "A.2.0", "A.2.1", "A.3.0"), false, ts.Nodes()...)

		// Make slot 1 committed.
		ts.IssueBlockAtSlot("A.3.1", 3, iotago.NewEmptyCommitment(), nodeA, ts.BlockID("A.3.0"))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.3.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("A.3.0"), true, ts.Nodes()...)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestCommitmentSlotIndex(1),
		)
	}

	// Issue in slot 4 so that slot 2 becomes committed.
	{
		slot1Commitment := lo.PanicOnErr(nodeA.Protocol.MainEngineInstance().Storage.Commitments().Load(1)).Commitment()

		ts.IssueBlockAtSlot("A.4.0", 4, iotago.NewEmptyCommitment(), nodeA, ts.BlockID("A.3.1"))
		ts.IssueBlockAtSlot("A.4.1", 4, slot1Commitment, nodeA, ts.BlockID("A.4.0"))
		ts.IssueBlockAtSlot("B.4.0", 4, slot1Commitment, nodeB, ts.BlockID("A.4.1"))
		ts.IssueBlockAtSlot("A.4.2", 4, slot1Commitment, nodeA, ts.BlockID("B.4.0"))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.4.1", "B.4.0"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("A.3.1", "A.4.0"), true, ts.Nodes()...)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithLatestCommitmentSlotIndex(2),
			testsuite.WithSybilProtectionCommittee(expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(lo.MergeMaps(expectedOnlineCommittee, map[iotago.AccountID]int64{
				nodeB.AccountID: 25,
			})),
			testsuite.WithEvictedSlot(2),
		)
	}

	// Confirm A.4.0 by pre-confirming a block a 3rd validator in slot 5.
	{
		slot1Commitment := lo.PanicOnErr(nodeA.Protocol.MainEngineInstance().Storage.Commitments().Load(1)).Commitment()
		slot2Commitment := lo.PanicOnErr(nodeA.Protocol.MainEngineInstance().Storage.Commitments().Load(2)).Commitment()

		ts.IssueBlockAtSlot("C.5.0", 5, slot1Commitment, nodeC, ts.BlockID("A.4.2"))
		ts.IssueBlockAtSlot("A.5.0", 5, slot2Commitment, nodeA, ts.BlockID("C.5.0"))
		ts.IssueBlockAtSlot("B.5.0", 5, slot2Commitment, nodeB, ts.BlockID("C.5.0"))
		ts.IssueBlockAtSlot("C.5.1", 5, slot1Commitment, nodeC, ts.BlockID("C.5.0"))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.3.0", "A.3.1", "A.4.0", "A.4.1", "A.4.2", "B.4.0", "C.5.0"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("A.3.0", "A.3.1", "A.4.0", "A.4.1", "A.4.2", "B.4.0", "C.5.0"), true, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("A.3.0", "A.3.1", "A.4.0", "A.4.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("A.4.0", "A.4.1"), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.5.0", "B.5.0", "C.5.1"), false, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("B.4.0", "A.4.2", "C.5.0"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("A.5.0", "B.5.0", "C.5.1"), false, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("B.4.0", "A.4.2", "C.5.0"), false, ts.Nodes()...)

		// Not confirmed because slot 3 <= 5 (ratifier index) - 2 (confirmation ratification threshold).
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("A.3.0", "A.3.1"), false, ts.Nodes()...)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithLatestCommitmentSlotIndex(2),
			testsuite.WithSybilProtectionCommittee(expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(lo.MergeMaps(expectedOnlineCommittee, map[iotago.AccountID]int64{
				nodeC.AccountID: 25,
			})),
			testsuite.WithEvictedSlot(2),
		)
	}

	// Confirm C.5.0 -> slot 1 should not be finalized as there's no supermajority within slot 4 or slot 5.
	{
		slot2Commitment := lo.PanicOnErr(nodeA.Protocol.MainEngineInstance().Storage.Commitments().Load(2)).Commitment()

		ts.IssueBlockAtSlot("A.6.0", 6, slot2Commitment, nodeA, ts.BlockIDs("A.5.0", "B.5.0", "C.5.1")...)
		ts.IssueBlockAtSlot("B.6.0", 6, slot2Commitment, nodeB, ts.BlockIDs("A.5.0", "B.5.0", "C.5.1")...)
		ts.IssueBlockAtSlot("C.6.0", 6, slot2Commitment, nodeC, ts.BlockIDs("A.5.0", "B.5.0", "C.5.1")...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.5.0", "B.5.0", "C.5.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("A.5.0", "B.5.0", "C.5.1"), true, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("B.4.0", "A.4.1", "C.5.0"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("B.4.0", "A.4.1", "C.5.0"), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.6.0", "B.6.0", "C.6.0"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("A.6.0", "B.6.0", "C.6.0"), false, ts.Nodes()...)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithLatestCommitmentSlotIndex(3),
			testsuite.WithEvictedSlot(3),
		)
	}

	// PreConfirm "A.6.0", "B.6.0", "C.6.0", Confirm "A.5.0", "B.5.0", "C.5.1" -> slot 1 should be finalized.
	{
		slot2Commitment := lo.PanicOnErr(nodeA.Protocol.MainEngineInstance().Storage.Commitments().Load(2)).Commitment()

		ts.IssueBlockAtSlot("A.6.1", 6, slot2Commitment, nodeA, ts.BlockIDs("A.6.0", "B.6.0", "C.6.0")...)
		ts.IssueBlockAtSlot("B.6.1", 6, slot2Commitment, nodeB, ts.BlockIDs("A.6.0", "B.6.0", "C.6.0")...)
		ts.IssueBlockAtSlot("C.6.1", 6, slot2Commitment, nodeC, ts.BlockIDs("A.6.0", "B.6.0", "C.6.0")...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.6.0", "B.6.0", "C.6.0"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("A.6.0", "B.6.0", "C.6.0"), true, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("B.4.0", "A.4.1", "C.5.0", "A.5.0", "B.5.0", "C.5.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("B.4.0", "A.4.1", "C.5.0", "A.5.0", "B.5.0", "C.5.1"), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.6.1", "B.6.1", "C.6.1"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("A.6.1", "B.6.1", "C.6.1"), false, ts.Nodes()...)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(1),
			testsuite.WithLatestCommitmentSlotIndex(3),
			testsuite.WithSybilProtectionCommittee(expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommittee),
			testsuite.WithEvictedSlot(3),
		)
	}
}
