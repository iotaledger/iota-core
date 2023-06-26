package tests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/attestation/slotattestation"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection/poa"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestProtocol_EngineSwitching(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithGenesisTimestampOffset(19*10),
		testsuite.WithLivenessThreshold(1), // TODO: remove this opt and use a proper value when refactoring the test with scheduler
		testsuite.WithEvictionAge(1),       // TODO: remove this opt and use a proper value when refactoring the test with scheduler
	)
	defer ts.Shutdown()

	node0 := ts.AddValidatorNodeToPartition("node0", "P1")
	node1 := ts.AddValidatorNodeToPartition("node1", "P1")
	node2 := ts.AddValidatorNodeToPartition("node2", "P1")
	node3 := ts.AddValidatorNodeToPartition("node3", "P1")
	node4 := ts.AddValidatorNodeToPartition("node4", "P1")
	node5 := ts.AddValidatorNodeToPartition("node5", "P2")
	node6 := ts.AddValidatorNodeToPartition("node6", "P2")

	nodesP1 := []*mock.Node{node0, node1, node2, node3, node4}
	nodesP2 := []*mock.Node{node5, node6}

	validators := ts.Validators()

	nodeOptions := []options.Option[protocol.Protocol]{
		protocol.WithNotarizationProvider(
			slotnotarization.NewProvider(1),
		),
		protocol.WithAttestationProvider(
			slotattestation.NewProvider(2),
		),
		protocol.WithChainManagerOptions(
			chainmanager.WithCommitmentRequesterOptions(
				eventticker.RetryInterval[iotago.SlotIndex, iotago.CommitmentID](1*time.Second),
				eventticker.RetryJitter[iotago.SlotIndex, iotago.CommitmentID](500*time.Millisecond),
			),
		),
	}

	nodeP1Options := append(nodeOptions,
		protocol.WithSybilProtectionProvider(
			poa.NewProvider(validators,
				poa.WithOnlineCommitteeStartup(node0.AccountID, node1.AccountID, node2.AccountID, node3.AccountID, node4.AccountID),
				poa.WithActivityWindow(1*time.Minute),
			),
		),
	)

	nodeP2Options := append(nodeOptions,
		protocol.WithSybilProtectionProvider(
			poa.NewProvider(validators,
				poa.WithOnlineCommitteeStartup(node5.AccountID, node6.AccountID),
				poa.WithActivityWindow(1*time.Minute),
			),
		),
	)

	ts.Run(map[string][]options.Option[protocol.Protocol]{
		"node0": nodeP1Options,
		"node1": nodeP1Options,
		"node2": nodeP1Options,
		"node3": nodeP1Options,
		"node4": nodeP1Options,
		"node5": nodeP2Options,
		"node6": nodeP2Options,
	})
	ts.HookLogging()

	expectedCommittee := []iotago.AccountID{
		node0.AccountID,
		node1.AccountID,
		node2.AccountID,
		node3.AccountID,
		node4.AccountID,
		node5.AccountID,
		node6.AccountID,
	}
	expectedP1Committee := []account.SeatIndex{
		node0.ValidatorSeat,
		node1.ValidatorSeat,
		node2.ValidatorSeat,
		node3.ValidatorSeat,
		node4.ValidatorSeat,
	}
	expectedP2Committee := []account.SeatIndex{
		node5.ValidatorSeat,
		node6.ValidatorSeat,
	}

	// Verify that nodes have the expected states.
	{
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitment(iotago.NewEmptyCommitment()),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithStorageCommitments([]*iotago.Commitment{iotago.NewEmptyCommitment()}),

			testsuite.WithSybilProtectionCommittee(0, expectedCommittee),
			testsuite.WithEvictedSlot(0),
			testsuite.WithActiveRootBlocks(ts.Blocks("Genesis")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis")),
		)

		ts.AssertSybilProtectionOnlineCommittee(expectedP1Committee, nodesP1...)
		ts.AssertSybilProtectionOnlineCommittee(expectedP2Committee, nodesP2...)
	}

	// Issue blocks on partition 1.
	{
		// Issue until slot 7 becomes committable.
		{
			ts.IssueBlockAtSlot("P1.A0", 5, iotago.NewEmptyCommitment(), node0, iotago.EmptyBlockID())
			ts.IssueBlockAtSlot("P1.A1", 5, iotago.NewEmptyCommitment(), node1, ts.Block("P1.A0").ID())
			ts.IssueBlockAtSlot("P1.A2", 5, iotago.NewEmptyCommitment(), node2, ts.Block("P1.A1").ID())
			ts.IssueBlockAtSlot("P1.A3", 5, iotago.NewEmptyCommitment(), node3, ts.Block("P1.A2").ID())
			ts.IssueBlockAtSlot("P1.A4", 5, iotago.NewEmptyCommitment(), node4, ts.Block("P1.A3").ID())

			ts.IssueBlockAtSlot("P1.B0", 6, iotago.NewEmptyCommitment(), node0, ts.Block("P1.A4").ID())
			ts.IssueBlockAtSlot("P1.B1", 6, iotago.NewEmptyCommitment(), node1, ts.Block("P1.B0").ID())
			ts.IssueBlockAtSlot("P1.B2", 6, iotago.NewEmptyCommitment(), node2, ts.Block("P1.B1").ID())
			ts.IssueBlockAtSlot("P1.B3", 6, iotago.NewEmptyCommitment(), node3, ts.Block("P1.B2").ID())
			ts.IssueBlockAtSlot("P1.B4", 6, iotago.NewEmptyCommitment(), node4, ts.Block("P1.B3").ID())

			ts.IssueBlockAtSlot("P1.C0", 7, iotago.NewEmptyCommitment(), node0, ts.Block("P1.B4").ID())
			ts.IssueBlockAtSlot("P1.C1", 7, iotago.NewEmptyCommitment(), node1, ts.Block("P1.C0").ID())
			ts.IssueBlockAtSlot("P1.C2", 7, iotago.NewEmptyCommitment(), node2, ts.Block("P1.C1").ID())
			ts.IssueBlockAtSlot("P1.C3", 7, iotago.NewEmptyCommitment(), node3, ts.Block("P1.C2").ID())
			ts.IssueBlockAtSlot("P1.C4", 7, iotago.NewEmptyCommitment(), node4, ts.Block("P1.C3").ID())

			ts.IssueBlockAtSlot("P1.D0", 8, iotago.NewEmptyCommitment(), node0, ts.Block("P1.C4").ID())
			ts.IssueBlockAtSlot("P1.D1", 8, iotago.NewEmptyCommitment(), node1, ts.Block("P1.D0").ID())
			ts.IssueBlockAtSlot("P1.D2", 8, iotago.NewEmptyCommitment(), node2, ts.Block("P1.D1").ID())
			ts.IssueBlockAtSlot("P1.D3", 8, iotago.NewEmptyCommitment(), node3, ts.Block("P1.D2").ID())
			ts.IssueBlockAtSlot("P1.D4", 8, iotago.NewEmptyCommitment(), node4, ts.Block("P1.D3").ID())

			ts.IssueBlockAtSlot("P1.E0", 9, iotago.NewEmptyCommitment(), node0, ts.Block("P1.D4").ID())
			ts.IssueBlockAtSlot("P1.E1", 9, iotago.NewEmptyCommitment(), node1, ts.Block("P1.E0").ID())
			ts.IssueBlockAtSlot("P1.E2", 9, iotago.NewEmptyCommitment(), node2, ts.Block("P1.E1").ID())
			ts.IssueBlockAtSlot("P1.E3", 9, iotago.NewEmptyCommitment(), node3, ts.Block("P1.E2").ID())
			ts.IssueBlockAtSlot("P1.E4", 9, iotago.NewEmptyCommitment(), node4, ts.Block("P1.E3").ID())
			ts.IssueBlockAtSlot("P1.E5", 9, iotago.NewEmptyCommitment(), node0, ts.Block("P1.E4").ID())
			ts.IssueBlockAtSlot("P1.E6", 9, iotago.NewEmptyCommitment(), node1, ts.Block("P1.E5").ID())
			ts.IssueBlockAtSlot("P1.E7", 9, iotago.NewEmptyCommitment(), node2, ts.Block("P1.E6").ID())
			ts.IssueBlockAtSlot("P1.E8", 9, iotago.NewEmptyCommitment(), node3, ts.Block("P1.E7").ID())
			ts.IssueBlockAtSlot("P1.E9", 9, iotago.NewEmptyCommitment(), node4, ts.Block("P1.E8").ID())

			ts.AssertBlocksInCachePreAccepted(ts.Blocks("P1.E6"), true, nodesP1...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("P1.E2"), true, nodesP1...)
			ts.AssertBlocksInCachePreConfirmed(ts.Blocks("P1.E5"), true, nodesP1...)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("P1.E0"), true, nodesP1...)

			ts.AssertNodeState(nodesP1,
				testsuite.WithLatestCommitmentSlotIndex(7),
				testsuite.WithEqualStoredCommitmentAtIndex(7),
				testsuite.WithLatestFinalizedSlot(0), // Blocks do only commit to Genesis -> can't finalize a slot.
				testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),

				testsuite.WithSybilProtectionOnlineCommittee(expectedP1Committee...),
				testsuite.WithSybilProtectionCommittee(9, expectedCommittee),
				testsuite.WithEvictedSlot(7),
			)
			ts.AssertLatestCommitmentSlotIndex(7, nodesP1...)
		}

		// Issue while committing to slot 7.
		{
			slot7Commitment := node0.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()
			ts.IssueBlockAtSlot("P1.E10", 9, slot7Commitment, node0, ts.Block("P1.E9").ID())
			ts.IssueBlockAtSlot("P1.E11", 9, slot7Commitment, node1, ts.Block("P1.E10").ID())
			ts.IssueBlockAtSlot("P1.E12", 9, slot7Commitment, node2, ts.Block("P1.E11").ID())

			ts.IssueBlockAtSlot("P1.F0", 10, slot7Commitment, node2, ts.Block("P1.E12").ID())
			ts.IssueBlockAtSlot("P1.F1", 10, slot7Commitment, node3, ts.Block("P1.F0").ID())
			ts.IssueBlockAtSlot("P1.F2", 10, slot7Commitment, node4, ts.Block("P1.F1").ID())
			ts.IssueBlockAtSlot("P1.F3", 10, slot7Commitment, node0, ts.Block("P1.F2").ID())
			ts.IssueBlockAtSlot("P1.F4", 10, slot7Commitment, node1, ts.Block("P1.F3").ID())

			ts.IssueBlockAtSlot("P1.G0", 11, slot7Commitment, node2, ts.BlockIDs("P1.F0", "P1.F4")...)
			ts.IssueBlockAtSlot("P1.G1", 11, slot7Commitment, node3, ts.Block("P1.G0").ID())
			ts.IssueBlockAtSlot("P1.G2", 11, slot7Commitment, node4, ts.Block("P1.G1").ID())
			ts.IssueBlockAtSlot("P1.G3", 11, slot7Commitment, node0, ts.Block("P1.G2").ID())
			ts.IssueBlockAtSlot("P1.G4", 11, slot7Commitment, node1, ts.Block("P1.G3").ID())
			ts.IssueBlockAtSlot("P1.G5", 11, slot7Commitment, node2, ts.Block("P1.G4").ID())
			ts.IssueBlockAtSlot("P1.G6", 11, slot7Commitment, node3, ts.Block("P1.G5").ID())
			ts.IssueBlockAtSlot("P1.G7", 11, slot7Commitment, node4, ts.Block("P1.G6").ID())
			ts.IssueBlockAtSlot("P1.G8", 11, slot7Commitment, node0, ts.Block("P1.G7").ID())

			ts.AssertBlocksInCachePreAccepted(ts.Blocks("P1.G5"), true, nodesP1...)
			ts.AssertBlocksInCachePreConfirmed(ts.Blocks("P1.G4"), true, nodesP1...)

			ts.AssertBlocksInCachePreAccepted(ts.Blocks("P1.G8"), false, nodesP1...)
			ts.AssertBlocksInCachePreConfirmed(ts.Blocks("P1.G8"), false, nodesP1...)

			ts.AssertBlocksInCacheAccepted(ts.Blocks("P1.G1"), true, nodesP1...)
			ts.AssertBlocksInCacheConfirmed(ts.BlocksWithPrefix("P1.F"), true, nodesP1...)

			// Verify that nodes have the expected states.
			ts.AssertNodeState(nodesP1,
				testsuite.WithLatestCommitmentCumulativeWeight(3),
				testsuite.WithLatestCommitmentSlotIndex(9),
				testsuite.WithEqualStoredCommitmentAtIndex(9),
				testsuite.WithLatestFinalizedSlot(7),
				testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),

				testsuite.WithSybilProtectionOnlineCommittee(expectedP1Committee...),
				testsuite.WithSybilProtectionCommittee(11, expectedCommittee),
				testsuite.WithEvictedSlot(9),
			)

			// Upon committing 9, we included attestations up to slot 9 that committed at least to slot 7.
			ts.AssertAttestationsForSlot(9, ts.Blocks("P1.E10", "P1.E11", "P1.E12"), nodesP1...)

			ts.AssertLatestCommitmentSlotIndex(9, nodesP1...)
		}

		{
			slot9Commitment := node0.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()
			ts.IssueBlockAtSlot("P1.H0", 12, slot9Commitment, node1, ts.Block("P1.G8").ID())
			ts.IssueBlockAtSlot("P1.H1", 12, slot9Commitment, node2, ts.Block("P1.H0").ID())
			ts.IssueBlockAtSlot("P1.H2", 12, slot9Commitment, node3, ts.Block("P1.H1").ID())

			ts.IssueBlockAtSlot("P1.I0", 13, slot9Commitment, node4, ts.Block("P1.H2").ID())
			ts.IssueBlockAtSlot("P1.I1", 13, slot9Commitment, node1, ts.Block("P1.I0").ID())
			ts.IssueBlockAtSlot("P1.I2", 13, slot9Commitment, node2, ts.Block("P1.I1").ID())
			ts.IssueBlockAtSlot("P1.I3", 13, slot9Commitment, node3, ts.Block("P1.I2").ID())
			ts.IssueBlockAtSlot("P1.I4", 13, slot9Commitment, node4, ts.Block("P1.I3").ID())
			ts.IssueBlockAtSlot("P1.I5", 13, slot9Commitment, node0, ts.Block("P1.I4").ID())
			ts.IssueBlockAtSlot("P1.I6", 13, slot9Commitment, node1, ts.Block("P1.I5").ID())

			ts.AssertBlocksInCachePreAccepted(ts.Blocks("P1.I3"), true, nodesP1...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("P1.H2"), true, nodesP1...)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("P1.G6"), true, nodesP1...)

			// Verify that nodes have the expected states.
			ts.AssertNodeState(nodesP1,
				// We have the same CW of Slot 9, because we didn't observe any attestation on top of 8 that we could include.
				testsuite.WithLatestCommitmentCumulativeWeight(3),
				testsuite.WithLatestCommitmentSlotIndex(10),
				testsuite.WithEqualStoredCommitmentAtIndex(10),
				testsuite.WithLatestFinalizedSlot(7),
				testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),

				testsuite.WithSybilProtectionOnlineCommittee(expectedP1Committee...),
				testsuite.WithSybilProtectionCommittee(13, expectedCommittee),
				testsuite.WithEvictedSlot(10),
			)

			// Upon committing 10, we included attestations up to slot 10 that committed at least to slot 8, but we haven't seen any.
			ts.AssertAttestationsForSlot(10, ts.Blocks(), nodesP1...)
			// Upon committing 9, we included attestations up to slot 9 that committed at least to slot 7.
			ts.AssertAttestationsForSlot(9, ts.Blocks("P1.E10", "P1.E11", "P1.E12"), nodesP1...)
			// Upon committing 8, we included attestations up to slot 6 that committed at least to slot 6: we didn't have any.
			ts.AssertAttestationsForSlot(8, ts.Blocks(), nodesP1...)

			ts.AssertAttestationsForSlot(7, ts.Blocks(), nodesP1...)
			ts.AssertAttestationsForSlot(6, ts.Blocks(), nodesP1...)
			ts.AssertAttestationsForSlot(5, ts.Blocks(), nodesP1...)
			ts.AssertAttestationsForSlot(4, ts.Blocks(), nodesP1...)

			ts.AssertLatestCommitmentSlotIndex(10, nodesP1...)
		}

		{
			slot10Commitment := node0.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()
			ts.IssueBlockAtSlot("P1.I7", 13, slot10Commitment, node2, ts.Block("P1.I6").ID())
			ts.IssueBlockAtSlot("P1.I8", 13, slot10Commitment, node3, ts.Block("P1.I7").ID())
			ts.IssueBlockAtSlot("P1.I9", 13, slot10Commitment, node4, ts.Block("P1.I8").ID())
			ts.IssueBlockAtSlot("P1.I10", 13, slot10Commitment, node0, ts.Block("P1.I9").ID())
			ts.IssueBlockAtSlot("P1.I11", 13, slot10Commitment, node1, ts.Block("P1.I10").ID())
			ts.IssueBlockAtSlot("P1.I12", 13, slot10Commitment, node2, ts.Block("P1.I11").ID())
			ts.IssueBlockAtSlot("P1.I13", 13, slot10Commitment, node3, ts.Block("P1.I12").ID())
			ts.IssueBlockAtSlot("P1.I14", 13, slot10Commitment, node4, ts.Block("P1.I13").ID())
			ts.IssueBlockAtSlot("P1.I15", 13, slot10Commitment, node0, ts.Block("P1.I14").ID())
			ts.IssueBlockAtSlot("P1.I16", 13, slot10Commitment, node1, ts.Block("P1.I15").ID())
			ts.IssueBlockAtSlot("P1.I17", 13, slot10Commitment, node2, ts.Block("P1.I16").ID())
			ts.IssueBlockAtSlot("P1.I18", 13, slot10Commitment, node3, ts.Block("P1.I17").ID())
			ts.IssueBlockAtSlot("P1.I19", 13, slot10Commitment, node4, ts.Block("P1.I18").ID())

			ts.AssertBlocksInCachePreAccepted(ts.Blocks("P1.I16"), true, nodesP1...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("P1.I12"), true, nodesP1...)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("P1.I10"), true, nodesP1...)

			// Verify that nodes have the expected states.
			ts.AssertNodeState(nodesP1,
				// We have the same CW of Slot 9, because we didn't observe any attestation on top of 8 that we could include.
				testsuite.WithLatestCommitmentCumulativeWeight(3),
				testsuite.WithLatestCommitmentSlotIndex(11),
				testsuite.WithEqualStoredCommitmentAtIndex(11),
				testsuite.WithLatestFinalizedSlot(9),
				testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),

				testsuite.WithSybilProtectionOnlineCommittee(expectedP1Committee...),
				testsuite.WithSybilProtectionCommittee(13, expectedCommittee),
				testsuite.WithEvictedSlot(11),
			)

			// Make sure the tips are properly set.
			ts.AssertStrongTips(ts.Blocks("P1.I19"), nodesP1...)

			// Upon committing 11, we included attestations up to slot 11 that committed at least to slot 9: we don't have any.
			ts.AssertAttestationsForSlot(11, ts.Blocks(), nodesP1...)

			require.Equal(t, node0.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
		}
	}

	// Issue blocks on partition 2.
	{
		{
			ts.IssueBlockAtSlot("P2.A", 5, iotago.NewEmptyCommitment(), node5, iotago.EmptyBlockID())
			ts.IssueBlockAtSlot("P2.B", 6, iotago.NewEmptyCommitment(), node6, ts.Block("P2.A").ID())
			ts.IssueBlockAtSlot("P2.C", 7, iotago.NewEmptyCommitment(), node5, ts.Block("P2.B").ID())
			ts.IssueBlockAtSlot("P2.D", 8, iotago.NewEmptyCommitment(), node6, ts.Block("P2.C").ID())
			ts.IssueBlockAtSlot("P2.E", 9, iotago.NewEmptyCommitment(), node5, ts.Block("P2.D").ID())
			ts.IssueBlockAtSlot("P2.F", 10, iotago.NewEmptyCommitment(), node6, ts.Block("P2.E").ID())
			ts.IssueBlockAtSlot("P2.G", 11, iotago.NewEmptyCommitment(), node5, ts.Block("P2.F").ID())
			ts.IssueBlockAtSlot("P2.H", 12, iotago.NewEmptyCommitment(), node6, ts.Block("P2.G").ID())
			ts.IssueBlockAtSlot("P2.I", 13, iotago.NewEmptyCommitment(), node5, ts.Block("P2.H").ID())

			ts.AssertBlocksInCachePreAccepted(ts.Blocks("P2.G", "P2.H"), true, nodesP2...)
			ts.AssertBlocksInCachePreAccepted(ts.Blocks("P2.I"), false, nodesP2...) // block not referenced yet
			ts.AssertBlocksInCacheAccepted(ts.Blocks("P2.E", "P2.F"), true, nodesP2...)

			ts.AssertBlocksInCachePreConfirmed(ts.Blocks("P2.E", "P2.F"), false, nodesP2...)

			// Verify that nodes have the expected states.
			ts.AssertNodeState(nodesP2,
				testsuite.WithLatestCommitmentSlotIndex(8),
				testsuite.WithEqualStoredCommitmentAtIndex(8),
				testsuite.WithLatestFinalizedSlot(0), // Blocks do only commit to Genesis -> can't finalize a slot.
				testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),

				testsuite.WithSybilProtectionOnlineCommittee(expectedP2Committee...),
				testsuite.WithSybilProtectionCommittee(13, expectedCommittee),
				testsuite.WithEvictedSlot(8),
			)
		}

		{
			slot8Commitment := node5.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()
			ts.IssueBlockAtSlot("P2.L1", 13, slot8Commitment, node6, ts.Block("P2.I").ID())
			ts.IssueBlockAtSlot("P2.L2", 13, slot8Commitment, node5, ts.Block("P2.L1").ID())
			ts.IssueBlockAtSlot("P2.L3", 13, slot8Commitment, node6, ts.Block("P2.L2").ID())
			ts.IssueBlockAtSlot("P2.L4", 13, slot8Commitment, node5, ts.Block("P2.L3").ID())
			ts.IssueBlockAtSlot("P2.L5", 13, slot8Commitment, node6, ts.Block("P2.L4").ID())

			ts.AssertBlocksInCachePreAccepted(ts.Blocks("P2.L1", "P2.L2", "P2.L3", "P2.L4"), true, nodesP2...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("P2.L1", "P2.L2"), true, nodesP2...)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("P2.L1", "P2.L2"), false, nodesP2...) // No supermajority

			// Verify that nodes have the expected states.
			ts.AssertNodeState(nodesP2,
				testsuite.WithLatestCommitmentSlotIndex(11),
				testsuite.WithEqualStoredCommitmentAtIndex(11),
				testsuite.WithLatestCommitmentCumulativeWeight(0), // We haven't collected any attestation yet.
				testsuite.WithLatestFinalizedSlot(0),              // Blocks do only commit to Genesis -> can't finalize a slot.
				testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),

				testsuite.WithSybilProtectionOnlineCommittee(expectedP2Committee...),
				testsuite.WithSybilProtectionCommittee(13, expectedCommittee),
				testsuite.WithEvictedSlot(11),
			)

			slot11Commitment := node5.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()
			ts.IssueBlockAtSlot("P2.M1", 13, slot8Commitment, node5, ts.Block("P2.L5").ID())
			ts.IssueBlockAtSlot("P2.M2", 13, slot8Commitment, node6, ts.Block("P2.M1").ID())
			ts.IssueBlockAtSlot("P2.M3", 13, slot11Commitment, node5, ts.Block("P2.M2").ID())
			ts.IssueBlockAtSlot("P2.M4", 13, slot11Commitment, node6, ts.Block("P2.M3").ID())

			// We are going to commit 13
			ts.IssueBlockAtSlot("P2.M5", 14, slot11Commitment, node5, ts.Block("P2.M4").ID())
			ts.IssueBlockAtSlot("P2.M6", 15, slot11Commitment, node6, ts.Block("P2.M5").ID())
			ts.IssueBlockAtSlot("P2.M7", 16, slot11Commitment, node5, ts.Block("P2.M6").ID())
			ts.IssueBlockAtSlot("P2.M8", 17, slot11Commitment, node6, ts.Block("P2.M7").ID())
			ts.IssueBlockAtSlot("P2.M9", 18, slot11Commitment, node5, ts.Block("P2.M8").ID())

			ts.AssertBlocksInCachePreAccepted(ts.Blocks("P2.M7", "P2.M8"), true, nodesP2...)
			ts.AssertBlocksInCachePreAccepted(ts.Blocks("P2.M9"), false, nodesP2...) // block not referenced yet
			ts.AssertBlocksInCacheAccepted(ts.Blocks("P2.M5", "P2.M6"), true, nodesP2...)

			// Verify that nodes have the expected states.
			ts.AssertNodeState(nodesP2,
				testsuite.WithLatestCommitmentSlotIndex(13),
				testsuite.WithEqualStoredCommitmentAtIndex(13),
				testsuite.WithLatestCommitmentCumulativeWeight(2),
				testsuite.WithLatestFinalizedSlot(0), // Blocks do only commit to Genesis -> can't finalize a slot.
				testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),

				testsuite.WithSybilProtectionOnlineCommittee(expectedP2Committee...),
				testsuite.WithSybilProtectionCommittee(18, expectedCommittee),
				testsuite.WithEvictedSlot(13),
			)

			// Make sure the tips are properly set.
			ts.AssertStrongTips(ts.Blocks("P2.M9"), nodesP2...)

			// Upon committing 13, we included attestations up to slot 13 that committed at least to slot 11.
			ts.AssertAttestationsForSlot(13, ts.Blocks("P2.M3", "P2.M4"), nodesP2...)
		}
	}

	// Make sure that no blocks of partition 1 are known on partition 2 and vice versa.
	{
		ts.AssertBlocksExist(ts.BlocksWithPrefix("P1"), true, nodesP1...)
		ts.AssertBlocksExist(ts.BlocksWithPrefix("P1"), false, nodesP2...)

		ts.AssertBlocksExist(ts.BlocksWithPrefix("P2"), true, nodesP2...)
		ts.AssertBlocksExist(ts.BlocksWithPrefix("P2"), false, nodesP1...)
	}

	// Both partitions should have committed slot 11 and 13 respectively and have different commitments.
	{
		ts.AssertLatestCommitmentSlotIndex(11, nodesP1...)
		ts.AssertLatestCommitmentSlotIndex(13, nodesP2...)

		ts.AssertEqualStoredCommitmentAtIndex(11, nodesP1...)
		ts.AssertEqualStoredCommitmentAtIndex(13, nodesP2...)
	}

	// Merge the partitions
	{
		ts.Network.MergePartitionsToMain()
		fmt.Println("\n=========================\nMerged network partitions\n=========================")

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		ctxP1, ctxP1Cancel := context.WithCancel(ctx)
		ctxP2, ctxP2Cancel := context.WithCancel(ctx)

		wg := &sync.WaitGroup{}

		// Issue blocks on both partitions after merging the networks.
		node0.IssueActivity(ctxP1, wg)
		node1.IssueActivity(ctxP1, wg)
		node2.IssueActivity(ctxP1, wg)
		node3.IssueActivity(ctxP1, wg)
		node4.IssueActivity(ctxP1, wg)

		node5.IssueActivity(ctxP2, wg)
		node6.IssueActivity(ctxP2, wg)

		// node 1 and 2 finalized until slot 10. We do not expect any forks here because our CW is higher than the other partition's
		ts.AssertForkDetectedCount(0, nodesP1...)
		// P1's chain is heavier, they should not consider switching the chain.
		ts.AssertCandidateEngineActivatedCount(0, nodesP1...)
		ctxP2Cancel() // we can stop issuing on P2.

		// Nodes from P2 should switch the chain.
		ts.AssertForkDetectedCount(1, nodesP2...)
		ts.AssertCandidateEngineActivatedCount(1, nodesP2...)

		// Here we need to let enough time pass for the nodes to sync up the candidate engines and switch them

		ts.AssertMainEngineSwitchedCount(1, nodesP2...)

		ctxP1Cancel()
		wg.Wait()
	}

	ts.AssertEqualStoredCommitmentAtIndex(18, ts.Nodes()...)
}
