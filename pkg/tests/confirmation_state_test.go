package tests

import (
	"fmt"
	"testing"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager/topstakers"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func TestConfirmationFlags(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		// TODO: remove this opt and use a proper value when refactoring the test with scheduler
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(100, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				testsuite.DefaultSlotsPerEpochExponent,
			),
			iotago.WithLivenessOptions(
				10,
				10,
				testsuite.DefaultMinCommittableAge,
				testsuite.DefaultMaxCommittableAge,
				testsuite.DefaultEpochNearingThreshold,
			),
			iotago.WithTargetCommitteeSize(4),
		),
	)
	defer ts.Shutdown()

	nodeA := ts.AddValidatorNode("nodeA")
	nodeB := ts.AddValidatorNode("nodeB")
	nodeC := ts.AddValidatorNode("nodeC")
	nodeD := ts.AddValidatorNode("nodeD")

	expectedCommittee := []iotago.AccountID{
		nodeA.Validator.AccountData.ID,
		nodeB.Validator.AccountData.ID,
		nodeC.Validator.AccountData.ID,
		nodeD.Validator.AccountData.ID,
	}

	nodeOpts := []options.Option[protocol.Protocol]{
		protocol.WithNotarizationProvider(
			slotnotarization.NewProvider(),
		),
		protocol.WithSybilProtectionProvider(
			sybilprotectionv1.NewProvider(
				sybilprotectionv1.WithSeatManagerProvider(
					topstakers.NewProvider(
						topstakers.WithOnlineCommitteeStartup(nodeA.Validator.AccountData.ID),
					),
				),
			),
		),
	}
	ts.Run(true, map[string][]options.Option[protocol.Protocol]{
		"nodeA": nodeOpts,
		"nodeB": nodeOpts,
		"nodeC": nodeOpts,
		"nodeD": nodeOpts,
	})

	// Verify that nodes have the expected states.
	genesisCommitment := iotago.NewEmptyCommitment(ts.API)
	genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
	ts.AssertNodeState(ts.Nodes(),
		testsuite.WithSnapshotImported(true),
		testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
		testsuite.WithLatestCommitment(genesisCommitment),
		testsuite.WithLatestFinalizedSlot(0),
		testsuite.WithMainChainID(genesisCommitment.MustID()),
		testsuite.WithStorageCommitments([]*iotago.Commitment{genesisCommitment}),
		testsuite.WithSybilProtectionCommittee(0, expectedCommittee),
		testsuite.WithSybilProtectionOnlineCommittee(lo.Return1(lo.Return1(nodeA.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(nodeA.Validator.AccountData.ID))),
		testsuite.WithEvictedSlot(0),
		testsuite.WithActiveRootBlocks(ts.Blocks("Genesis")),
		testsuite.WithStorageRootBlocks(ts.Blocks("Genesis")),
	)

	// Slots 1-3: only node A is online and issues blocks, make slot 1 committed.
	{
		ts.SetCurrentSlot(1)
		ts.IssueValidationBlockWithHeaderOptions("A.1.0", nodeA, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs("Genesis")...))
		ts.IssueValidationBlockWithHeaderOptions("A.1.1", nodeA, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs("A.1.0")...))
		ts.SetCurrentSlot(2)
		ts.IssueValidationBlockWithHeaderOptions("A.2.0", nodeA, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs("A.1.1")...))
		ts.IssueValidationBlockWithHeaderOptions("A.2.1", nodeA, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs("A.2.0")...))
		ts.SetCurrentSlot(3)
		ts.IssueValidationBlockWithHeaderOptions("A.3.0", nodeA, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs("A.2.1")...))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.1.0", "A.1.1", "A.2.0", "A.2.1", "A.3.0"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("A.1.0", "A.1.1", "A.2.0", "A.2.1"), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("A.1.0", "A.1.1", "A.2.0", "A.2.1", "A.3.0"), false, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("A.1.0", "A.1.1", "A.2.0", "A.2.1", "A.3.0"), false, ts.Nodes()...)

		// Make slot 1 committed.
		slot1CommittableSlot := 1 + ts.API.ProtocolParameters().MinCommittableAge()
		ts.SetCurrentSlot(slot1CommittableSlot)
		alias1A0 := fmt.Sprintf("A.%d.0", slot1CommittableSlot)
		alias1A1 := fmt.Sprintf("A.%d.1", slot1CommittableSlot)
		ts.IssueValidationBlockWithHeaderOptions(alias1A0, nodeA, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs("A.3.0")...))
		ts.IssueValidationBlockWithHeaderOptions(alias1A1, nodeA, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs(alias1A0)...))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks(alias1A0), true, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("A.3.0"), true, ts.Nodes()...)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestCommitmentSlotIndex(1),
			testsuite.WithEqualStoredCommitmentAtIndex(1),
		)

		// Issue in the next slot so that slot 2 becomes committed.
		slot1Commitment := lo.PanicOnErr(nodeA.Protocol.Engines.Main.Get().Storage.Commitments().Load(1)).Commitment()
		slot2CommittableSlot := slot1CommittableSlot + 1
		ts.SetCurrentSlot(slot2CommittableSlot)
		alias2A0 := fmt.Sprintf("A.%d.0", slot2CommittableSlot)
		alias2A1 := fmt.Sprintf("A.%d.1", slot2CommittableSlot)
		alias2A2 := fmt.Sprintf("A.%d.2", slot2CommittableSlot)
		alias2B0 := fmt.Sprintf("B.%d.0", slot2CommittableSlot)
		ts.IssueValidationBlockWithHeaderOptions(alias2A0, nodeA, mock.WithSlotCommitment(genesisCommitment), mock.WithStrongParents(ts.BlockIDs(alias1A1)...))
		ts.IssueValidationBlockWithHeaderOptions(alias2A1, nodeA, mock.WithSlotCommitment(slot1Commitment), mock.WithStrongParents(ts.BlockIDs(alias2A0)...))
		ts.IssueValidationBlockWithHeaderOptions(alias2B0, nodeB, mock.WithSlotCommitment(slot1Commitment), mock.WithStrongParents(ts.BlockIDs(alias2A1)...))
		ts.IssueValidationBlockWithHeaderOptions(alias2A2, nodeA, mock.WithSlotCommitment(slot1Commitment), mock.WithStrongParents(ts.BlockIDs(alias2B0)...))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks(alias2A1, alias2B0), true, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks(alias1A1, alias2A0), true, ts.Nodes()...)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithLatestCommitmentSlotIndex(2),
			testsuite.WithEqualStoredCommitmentAtIndex(2),
			testsuite.WithSybilProtectionCommittee(ts.API.TimeProvider().EpochFromSlot(slot2CommittableSlot), expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(
				lo.Return1(lo.Return1(nodeA.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(nodeA.Validator.AccountData.ID)),
				lo.Return1(lo.Return1(nodeA.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(nodeB.Validator.AccountData.ID)),
			),
			testsuite.WithEvictedSlot(2),
		)

		// Confirm aliasA0 by pre-confirming a block a 3rd validator in the next slot.
		slot2Commitment := lo.PanicOnErr(nodeA.Protocol.Engines.Main.Get().Storage.Commitments().Load(2)).Commitment()
		slot3CommittableSlot := slot2CommittableSlot + 1
		ts.SetCurrentSlot(slot3CommittableSlot)
		alias3C0 := fmt.Sprintf("C.%d.0", slot3CommittableSlot)
		alias3A0 := fmt.Sprintf("A.%d.0", slot3CommittableSlot)
		alias3B0 := fmt.Sprintf("B.%d.0", slot3CommittableSlot)
		alias3C1 := fmt.Sprintf("C.%d.1", slot3CommittableSlot)
		ts.IssueValidationBlockWithHeaderOptions(alias3C0, nodeC, mock.WithSlotCommitment(slot1Commitment), mock.WithStrongParents(ts.BlockIDs(alias2A2)...))
		ts.IssueValidationBlockWithHeaderOptions(alias3A0, nodeA, mock.WithSlotCommitment(slot2Commitment), mock.WithStrongParents(ts.BlockIDs(alias3C0)...))
		ts.IssueValidationBlockWithHeaderOptions(alias3B0, nodeB, mock.WithSlotCommitment(slot2Commitment), mock.WithStrongParents(ts.BlockIDs(alias3C0)...))
		ts.IssueValidationBlockWithHeaderOptions(alias3C1, nodeC, mock.WithSlotCommitment(slot1Commitment), mock.WithStrongParents(ts.BlockIDs(alias3C0)...))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("A.3.0", alias1A1, alias2A0, alias2A1, alias2A2, alias2B0, alias3C0), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("A.3.0", alias1A1, alias2A0, alias2A1, alias2A2, alias2B0, alias3C0), true, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("A.3.0", alias1A1, alias2A0, alias2A1), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks(alias2A0, alias2A1), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks(alias3A0, alias3B0, alias3C1), false, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks(alias2B0, alias2A2, alias3C0), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks(alias3A0, alias3B0, alias3C1), false, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks(alias2B0, alias2A2, alias3C0), false, ts.Nodes()...)

		// Not confirmed because slot 3 <= 5 (ratifier slot) - 2 (confirmation ratification threshold).
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("A.3.0", alias1A1), false, ts.Nodes()...)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithLatestCommitmentSlotIndex(2),
			testsuite.WithEqualStoredCommitmentAtIndex(2),
			testsuite.WithSybilProtectionCommittee(ts.API.TimeProvider().EpochFromSlot(slot3CommittableSlot), expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(
				lo.Return1(lo.Return1(nodeA.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(nodeA.Validator.AccountData.ID)),
				lo.Return1(lo.Return1(nodeA.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(nodeB.Validator.AccountData.ID)),
				lo.Return1(lo.Return1(nodeA.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(nodeC.Validator.AccountData.ID)),
			),
			testsuite.WithEvictedSlot(2),
		)

		// Confirm C.5.0 -> slot 1 should not be finalized as there's no supermajority within slot 4 or slot 5.
		slot4CommittableSlot := slot3CommittableSlot + 1
		ts.SetCurrentSlot(slot4CommittableSlot)
		alias4A0 := fmt.Sprintf("A.%d.0", slot4CommittableSlot)
		alias4B0 := fmt.Sprintf("B.%d.0", slot4CommittableSlot)
		alias4C0 := fmt.Sprintf("C.%d.0", slot4CommittableSlot)
		ts.IssueValidationBlockWithHeaderOptions(alias4A0, nodeA, mock.WithSlotCommitment(slot2Commitment), mock.WithStrongParents(ts.BlockIDs(alias3A0, alias3B0, alias3C1)...))
		ts.IssueValidationBlockWithHeaderOptions(alias4B0, nodeB, mock.WithSlotCommitment(slot2Commitment), mock.WithStrongParents(ts.BlockIDs(alias3A0, alias3B0, alias3C1)...))
		ts.IssueValidationBlockWithHeaderOptions(alias4C0, nodeC, mock.WithSlotCommitment(slot2Commitment), mock.WithStrongParents(ts.BlockIDs(alias3A0, alias3B0, alias3C1)...))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks(alias3A0, alias3B0, alias3C1), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks(alias3A0, alias3B0, alias3C1), true, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks(alias2B0, alias2A1, alias3C0), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks(alias2B0, alias2A1, alias3C0), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks(alias4A0, alias4B0, alias4C0), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks(alias4A0, alias4B0, alias4C0), false, ts.Nodes()...)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithLatestCommitmentSlotIndex(3),
			testsuite.WithEqualStoredCommitmentAtIndex(3),
			testsuite.WithEvictedSlot(3),
		)

		// PreConfirm "A.6.0", "B.6.0", "C.6.0", Confirm "A.5.0", "B.5.0", "C.5.1" -> slot 1 should be finalized.
		alias4A1 := fmt.Sprintf("A.%d.1", slot4CommittableSlot)
		alias4B1 := fmt.Sprintf("B.%d.1", slot4CommittableSlot)
		alias4C1 := fmt.Sprintf("C.%d.1", slot4CommittableSlot)
		ts.IssueValidationBlockWithHeaderOptions(alias4A1, nodeA, mock.WithSlotCommitment(slot2Commitment), mock.WithStrongParents(ts.BlockIDs(alias4A0, alias4B0, alias4C0)...))
		ts.IssueValidationBlockWithHeaderOptions(alias4B1, nodeB, mock.WithSlotCommitment(slot2Commitment), mock.WithStrongParents(ts.BlockIDs(alias4A0, alias4B0, alias4C0)...))
		ts.IssueValidationBlockWithHeaderOptions(alias4C1, nodeC, mock.WithSlotCommitment(slot2Commitment), mock.WithStrongParents(ts.BlockIDs(alias4A0, alias4B0, alias4C0)...))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks(alias4A0, alias4B0, alias4C0), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks(alias4A0, alias4B0, alias4C0), true, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks(alias2B0, alias2A1, alias3C0, alias3A0, alias3B0, alias3C1), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks(alias2B0, alias2A1, alias3C0, alias3A0, alias3B0, alias3C1), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks(alias4A1, alias4B1, alias4C1), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks(alias4A1, alias4B1, alias4C1), false, ts.Nodes()...)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(1),
			testsuite.WithLatestCommitmentSlotIndex(3),
			testsuite.WithEqualStoredCommitmentAtIndex(3),
			testsuite.WithSybilProtectionCommittee(ts.API.TimeProvider().EpochFromSlot(slot4CommittableSlot), expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(
				lo.Return1(lo.Return1(nodeA.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(nodeA.Validator.AccountData.ID)),
				lo.Return1(lo.Return1(nodeA.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(nodeB.Validator.AccountData.ID)),
				lo.Return1(lo.Return1(nodeA.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(nodeC.Validator.AccountData.ID)),
			),
			testsuite.WithEvictedSlot(3),
		)
	}
}

func TestConfirmationOverEpochBoundary(t *testing.T) {
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
				3,
				4,
				5,
			),
		),
	)
	defer ts.Shutdown()

	ts.AddValidatorNode("node0")
	ts.AddValidatorNode("node1")
	ts.AddValidatorNode("node2")
	ts.AddValidatorNode("node3")
	ts.AddNode("node4")

	ts.Run(true)

	// Issue blocks up until 1 slot more than the epoch.
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{1, 2, 3, 4, 5, 6, 7, 8, 9}, 4, "Genesis", ts.Nodes(), true, false)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(5),
			testsuite.WithLatestCommitmentSlotIndex(6),
			testsuite.WithEqualStoredCommitmentAtIndex(6),
			testsuite.WithEvictedSlot(6),
		)

		// Verify propagation of witness weight over epoch boundaries (slot 7).
		{
			// We propagate witness weight for pre-acceptance and acceptance over epoch boundaries.
			ts.AssertBlocksInCachePreAccepted(ts.BlocksWithPrefixes("7.0", "7.1", "7.2", "7.3"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.BlocksWithPrefixes("7.0", "7.1", "7.2", "7.3"), true, ts.Nodes()...)

			// We don't propagate pre-confirmation and confirmation over epoch boundaries:
			// There's 4 rows in a slot, everything except the last row should be pre-confirmed.
			ts.AssertBlocksInCachePreConfirmed(ts.BlocksWithPrefixes("7.0", "7.1", "7.2"), true, ts.Nodes()...)
			ts.AssertBlocksInCachePreConfirmed(ts.BlocksWithPrefixes("7.3"), false, ts.Nodes()...)
			// Accordingly, only the first 2 rows are confirmed.
			ts.AssertBlocksInCacheConfirmed(ts.BlocksWithPrefixes("7.0", "7.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheConfirmed(ts.BlocksWithPrefixes("7.2", "7.3"), false, ts.Nodes()...)

		}

		// Slot 8 and 9 behaves normally, as they are in the new epoch.
		{
			ts.AssertBlocksInCacheAccepted(ts.BlocksWithPrefixes("8.0", "8.1", "8.2", "8.3", "9.0", "9.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.BlocksWithPrefixes("9.2", "9.3"), false, ts.Nodes()...)
			ts.AssertBlocksInCachePreAccepted(ts.BlocksWithPrefixes("9.2"), true, ts.Nodes()...)
			ts.AssertBlocksInCachePreAccepted(ts.BlocksWithPrefixes("9.3"), false, ts.Nodes()...)

			ts.AssertBlocksInCacheConfirmed(ts.BlocksWithPrefixes("8.0", "8.1", "8.2", "8.3", "9.0", "9.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheConfirmed(ts.BlocksWithPrefixes("9.2", "9.3"), false, ts.Nodes()...)
			ts.AssertBlocksInCachePreConfirmed(ts.BlocksWithPrefixes("9.2"), true, ts.Nodes()...)
			ts.AssertBlocksInCachePreConfirmed(ts.BlocksWithPrefixes("9.3"), false, ts.Nodes()...)
		}
	}

	// Issue more so that blocks at end of epoch become confirmed via finalization.
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{10, 11, 12}, 4, "9.3", ts.Nodes(), true, false)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(8),
			testsuite.WithLatestCommitmentSlotIndex(9),
			testsuite.WithEqualStoredCommitmentAtIndex(9),
			testsuite.WithEvictedSlot(9),
		)

		ts.AssertRetainerBlocksState(ts.BlocksWithPrefixes("7", "8"), api.BlockStateFinalized, ts.Nodes()...)
		ts.AssertRetainerBlocksState(ts.BlocksWithPrefixes("9", "10", "11"), api.BlockStateConfirmed, ts.Nodes()...)
	}
}
