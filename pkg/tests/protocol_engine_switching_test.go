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
	ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNodeToPartition("node1", 75, "P1")
	node2 := ts.AddValidatorNodeToPartition("node2", 75, "P1")
	node3 := ts.AddValidatorNodeToPartition("node3", 25, "P2")
	node4 := ts.AddValidatorNodeToPartition("node4", 25, "P2")

	ts.Run(map[string][]options.Option[protocol.Protocol]{
		"node1": {
			protocol.WithSybilProtectionProvider(poa.NewProvider(ts.Validators(), poa.WithOnlineCommitteeStartup(node1.AccountID, node2.AccountID))),
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
		},
		"node2": {
			protocol.WithSybilProtectionProvider(poa.NewProvider(ts.Validators(), poa.WithOnlineCommitteeStartup(node1.AccountID, node2.AccountID))),
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
		},
		"node3": {
			protocol.WithSybilProtectionProvider(poa.NewProvider(ts.Validators(), poa.WithOnlineCommitteeStartup(node3.AccountID, node4.AccountID))),
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
		},
		"node4": {
			protocol.WithSybilProtectionProvider(poa.NewProvider(ts.Validators(), poa.WithOnlineCommitteeStartup(node3.AccountID, node4.AccountID))),
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
		},
	})
	ts.HookLogging()

	expectedCommittee := map[iotago.AccountID]int64{
		node1.AccountID: 75,
		node2.AccountID: 75,
		node3.AccountID: 25,
		node4.AccountID: 25,
	}
	expectedP1Committee := map[iotago.AccountID]int64{
		node1.AccountID: 75,
		node2.AccountID: 75,
	}
	expectedP2Committee := map[iotago.AccountID]int64{
		node3.AccountID: 25,
		node4.AccountID: 25,
	}

	// Verify that nodes have the expected states.
	{
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitment(iotago.NewEmptyCommitment()),
			testsuite.WithLatestStateMutationSlot(0),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithStorageCommitments([]*iotago.Commitment{iotago.NewEmptyCommitment()}),

			testsuite.WithSybilProtectionCommittee(expectedCommittee),
			testsuite.WithEvictedSlot(0),
			testsuite.WithActiveRootBlocks(ts.Blocks("Genesis")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis")),
		)

		ts.AssertSybilProtectionOnlineCommittee(expectedP1Committee, node1, node2)
		ts.AssertSybilProtectionOnlineCommittee(expectedP2Committee, node3, node4)
	}

	// Issue blocks on partition 1.
	{
		// Issue until slot 7 becomes committable.
		{
			ts.IssueBlockAtSlot("P1.A", 5, iotago.NewEmptyCommitment(), node1, iotago.EmptyBlockID())
			ts.IssueBlockAtSlot("P1.B", 6, iotago.NewEmptyCommitment(), node2, ts.Block("P1.A").ID())
			ts.IssueBlockAtSlot("P1.C", 7, iotago.NewEmptyCommitment(), node1, ts.Block("P1.B").ID())
			ts.IssueBlockAtSlot("P1.D", 8, iotago.NewEmptyCommitment(), node2, ts.Block("P1.C").ID())

			ts.IssueBlockAtSlot("P1.E", 9, iotago.NewEmptyCommitment(), node1, ts.Block("P1.D").ID())
			ts.IssueBlockAtSlot("P1.E2", 9, iotago.NewEmptyCommitment(), node2, ts.Block("P1.E").ID())
			ts.IssueBlockAtSlot("P1.E3", 9, iotago.NewEmptyCommitment(), node1, ts.Block("P1.E2").ID())
			ts.IssueBlockAtSlot("P1.E4", 9, iotago.NewEmptyCommitment(), node2, ts.Block("P1.E3").ID())

			ts.AssertBlocksInCacheAccepted(ts.Blocks("P1.E2", "P1.E3"), true, node1, node2)
			ts.AssertBlocksInCacheRatifiedAccepted(ts.Blocks("P1.D", "P1.E"), true, node1, node2)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("P1.D", "P1.E"), true, node1, node2)

			ts.AssertNodeState(ts.Nodes("node1", "node2"),
				testsuite.WithLatestCommitmentSlotIndex(7),
				testsuite.WithLatestStateMutationSlot(0),
				testsuite.WithLatestFinalizedSlot(0), // Blocks do only commit to Genesis -> can't finalize a slot.
				testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),

				testsuite.WithSybilProtectionOnlineCommittee(expectedP1Committee),
				testsuite.WithSybilProtectionCommittee(expectedCommittee),
				testsuite.WithEvictedSlot(7),
			)
			require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
		}

		// Issue while committing to slot 7.
		{
			slot7Commitment := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()
			ts.IssueBlockAtSlot("P1.E5", 9, slot7Commitment, node1, ts.Block("P1.E4").ID())
			ts.IssueBlockAtSlot("P1.E6", 9, slot7Commitment, node2, ts.Block("P1.E5").ID())

			ts.IssueBlockAtSlot("P1.F", 10, slot7Commitment, node2, ts.Block("P1.E6").ID())
			ts.IssueBlockAtSlot("P1.G", 11, slot7Commitment, node1, ts.Block("P1.F").ID())
			ts.IssueBlockAtSlot("P1.G2", 11, slot7Commitment, node2, ts.Block("P1.G").ID())
			ts.IssueBlockAtSlot("P1.G3", 11, slot7Commitment, node1, ts.Block("P1.G2").ID())
			ts.IssueBlockAtSlot("P1.G4", 11, slot7Commitment, node2, ts.Block("P1.G3").ID())

			ts.AssertBlocksInCacheAccepted(ts.Blocks("P1.G2", "P1.G3"), true, node1, node2)
			ts.AssertBlocksInCacheRatifiedAccepted(ts.Blocks("P1.F", "P1.G"), true, node1, node2)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("P1.F", "P1.G"), true, node1, node2)

			// Verify that nodes have the expected states.
			ts.AssertNodeState(ts.Nodes("node1", "node2"),
				testsuite.WithLatestCommitmentCumulativeWeight(150),
				testsuite.WithLatestCommitmentSlotIndex(9),
				testsuite.WithLatestStateMutationSlot(0),
				testsuite.WithLatestFinalizedSlot(7),
				testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),

				testsuite.WithSybilProtectionOnlineCommittee(expectedP1Committee),
				testsuite.WithSybilProtectionCommittee(expectedCommittee),
				testsuite.WithEvictedSlot(9),
			)

			// Upon committing 9, we included attestations up to slot 9 that committed at least to slot 7.
			ts.AssertAttestationsForSlot(9, ts.Blocks("P1.E5", "P1.E6"), node1, node2)

			require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
		}

		{
			slot9Commitment := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()
			ts.IssueBlockAtSlot("P1.H", 12, slot9Commitment, node2, ts.Block("P1.G4").ID())
			ts.IssueBlockAtSlot("P1.I", 13, slot9Commitment, node1, ts.Block("P1.H").ID())
			ts.IssueBlockAtSlot("P1.I2", 13, slot9Commitment, node2, ts.Block("P1.I").ID())
			ts.IssueBlockAtSlot("P1.I3", 13, slot9Commitment, node1, ts.Block("P1.I2").ID())

			ts.AssertBlocksInCacheAccepted(ts.Blocks("P1.I", "P1.I2"), true, node1, node2)
			ts.AssertBlocksInCacheRatifiedAccepted(ts.Blocks("P1.H"), true, node1, node2)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("P1.H"), true, node1, node2)

			// Verify that nodes have the expected states.
			ts.AssertNodeState(ts.Nodes("node1", "node2"),
				// We have the same CW of Slot 9, because we didn't observe any attestation on top of 8 that we could include.
				testsuite.WithLatestCommitmentCumulativeWeight(150),
				testsuite.WithLatestCommitmentSlotIndex(10),
				testsuite.WithLatestStateMutationSlot(0),
				testsuite.WithLatestFinalizedSlot(7),
				testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),

				testsuite.WithSybilProtectionOnlineCommittee(expectedP1Committee),
				testsuite.WithSybilProtectionCommittee(expectedCommittee),
				testsuite.WithEvictedSlot(10),
			)

			// Upon committing 10, we included attestations up to slot 10 that committed at least to slot 8, but we haven't seen any.
			ts.AssertAttestationsForSlot(10, ts.Blocks(), node1, node2)
			// Upon committing 9, we included attestations up to slot 9 that committed at least to slot 7.
			ts.AssertAttestationsForSlot(9, ts.Blocks("P1.E5", "P1.E6"), node1, node2)
			// Upon committing 8, we included attestations up to slot 6 that committed at least to slot 6: we didn't have any.
			ts.AssertAttestationsForSlot(8, ts.Blocks(), node1, node2)

			ts.AssertAttestationsForSlot(7, ts.Blocks(), node1, node2)
			ts.AssertAttestationsForSlot(6, ts.Blocks(), node1, node2)
			ts.AssertAttestationsForSlot(5, ts.Blocks(), node1, node2)
			ts.AssertAttestationsForSlot(4, ts.Blocks(), node1, node2)

			require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
		}

		{
			slot10Commitment := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()
			ts.IssueBlockAtSlot("P1.L1", 13, slot10Commitment, node1, ts.Block("P1.I3").ID())
			ts.IssueBlockAtSlot("P1.L2", 13, slot10Commitment, node2, ts.Block("P1.L1").ID())
			ts.IssueBlockAtSlot("P1.M", 13, slot10Commitment, node1, ts.Block("P1.L2").ID())
			ts.IssueBlockAtSlot("P1.N", 13, slot10Commitment, node2, ts.Block("P1.M").ID())
			ts.IssueBlockAtSlot("P1.O", 13, slot10Commitment, node1, ts.Block("P1.N").ID())
			ts.IssueBlockAtSlot("P1.P", 13, slot10Commitment, node2, ts.Block("P1.O").ID())

			ts.AssertBlocksInCacheAccepted(ts.Blocks("P1.L1", "P1.L1", "P1.L2", "P1.M", "P1.N", "P1.O"), true, node1, node2)
			ts.AssertBlocksInCacheRatifiedAccepted(ts.Blocks("P1.L1", "P1.L2", "P1.M"), true, node1, node2)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("P1.L1", "P1.L2", "P1.M"), true, node1, node2)

			// Verify that nodes have the expected states.
			ts.AssertNodeState(ts.Nodes("node1", "node2"),
				// We have the same CW of Slot 9, because we didn't observe any attestation on top of 8 that we could include.
				testsuite.WithLatestCommitmentCumulativeWeight(150),
				testsuite.WithLatestCommitmentSlotIndex(11),
				testsuite.WithLatestStateMutationSlot(0),
				testsuite.WithLatestFinalizedSlot(10),
				testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),

				testsuite.WithSybilProtectionOnlineCommittee(expectedP1Committee),
				testsuite.WithSybilProtectionCommittee(expectedCommittee),
				testsuite.WithEvictedSlot(11),
			)

			// Upon committing 11, we included attestations up to slot 11 that committed at least to slot 9: we don't have any.
			ts.AssertAttestationsForSlot(11, ts.Blocks(), node1, node2)

			require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
		}
	}

	{
		// Make sure that no blocks of partition 1 are known on partition 2.
		ts.AssertBlocksExist(ts.BlocksWithPrefix("P1"), true, node1, node2)
		ts.AssertBlocksExist(ts.BlocksWithPrefix("P1"), false, node3, node4)
	}

	// TODO: extend P2 with more blocks and checks on cumulative weight.
	// Issue blocks on partition 2.
	{
		{
			ts.IssueBlockAtSlot("P2.A", 5, iotago.NewEmptyCommitment(), node3, iotago.EmptyBlockID())
			ts.IssueBlockAtSlot("P2.B", 6, iotago.NewEmptyCommitment(), node4, ts.Block("P2.A").ID())
			ts.IssueBlockAtSlot("P2.C", 7, iotago.NewEmptyCommitment(), node3, ts.Block("P2.B").ID())
			ts.IssueBlockAtSlot("P2.D", 8, iotago.NewEmptyCommitment(), node4, ts.Block("P2.C").ID())
			ts.IssueBlockAtSlot("P2.E", 9, iotago.NewEmptyCommitment(), node3, ts.Block("P2.D").ID())
			ts.IssueBlockAtSlot("P2.F", 10, iotago.NewEmptyCommitment(), node4, ts.Block("P2.E").ID())
			ts.IssueBlockAtSlot("P2.G", 11, iotago.NewEmptyCommitment(), node3, ts.Block("P2.F").ID())
			ts.IssueBlockAtSlot("P2.H", 12, iotago.NewEmptyCommitment(), node4, ts.Block("P2.G").ID())
			ts.IssueBlockAtSlot("P2.I", 13, iotago.NewEmptyCommitment(), node3, ts.Block("P2.H").ID())

			ts.AssertBlocksExist(ts.BlocksWithPrefix("P2"), true, node3, node4)
			ts.AssertBlocksExist(ts.BlocksWithPrefix("P2"), false, node1, node2)

			ts.AssertBlocksInCacheAccepted(ts.Blocks("P2.G", "P2.H"), true, node3, node4)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("P2.I"), false, node3, node4) // block not referenced yet
			ts.AssertBlocksInCacheRatifiedAccepted(ts.Blocks("P2.E", "P2.F"), true, node3, node4)

			ts.AssertBlocksInCacheConfirmed(ts.Blocks("P2.E", "P2.F"), false, node3, node4)

			// Verify that nodes have the expected states.
			ts.AssertNodeState(ts.Nodes("node3", "node4"),
				testsuite.WithLatestCommitmentSlotIndex(8),
				testsuite.WithLatestStateMutationSlot(0),
				testsuite.WithLatestFinalizedSlot(0), // Blocks do only commit to Genesis -> can't finalize a slot.
				testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),

				testsuite.WithSybilProtectionOnlineCommittee(expectedP2Committee),
				testsuite.WithSybilProtectionCommittee(expectedCommittee),
				testsuite.WithEvictedSlot(8),
			)
		}

		{
			slot8Commitment := node3.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()
			ts.IssueBlockAtSlot("P2.L1", 13, slot8Commitment, node4, ts.Block("P2.I").ID())
			ts.IssueBlockAtSlot("P2.L2", 13, slot8Commitment, node3, ts.Block("P2.L1").ID())
			ts.IssueBlockAtSlot("P2.L3", 13, slot8Commitment, node4, ts.Block("P2.L2").ID())
			ts.IssueBlockAtSlot("P2.L4", 13, slot8Commitment, node3, ts.Block("P2.L3").ID())
			ts.IssueBlockAtSlot("P2.L5", 13, slot8Commitment, node4, ts.Block("P2.L4").ID())

			ts.AssertBlocksInCacheAccepted(ts.Blocks("P2.L1", "P2.L2", "P2.L3", "P2.L4"), true, node3, node4)
			ts.AssertBlocksInCacheRatifiedAccepted(ts.Blocks("P2.L1", "P2.L2"), true, node3, node4)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("P2.L1", "P2.L2"), false, node3, node4) // No supermajority

			// Verify that nodes have the expected states.
			ts.AssertNodeState(ts.Nodes("node3", "node4"),
				testsuite.WithLatestCommitmentSlotIndex(11),
				testsuite.WithLatestCommitmentCumulativeWeight(0), // We haven't collected any attestation yet.
				testsuite.WithLatestStateMutationSlot(0),
				testsuite.WithLatestFinalizedSlot(0), // Blocks do only commit to Genesis -> can't finalize a slot.
				testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),

				testsuite.WithSybilProtectionOnlineCommittee(expectedP2Committee),
				testsuite.WithSybilProtectionCommittee(expectedCommittee),
				testsuite.WithEvictedSlot(11),
			)

			slot11Commitment := node3.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()
			ts.IssueBlockAtSlot("P2.M1", 13, slot8Commitment, node3, ts.Block("P2.L5").ID())
			ts.IssueBlockAtSlot("P2.M2", 13, slot8Commitment, node4, ts.Block("P2.M1").ID())
			ts.IssueBlockAtSlot("P2.M3", 13, slot11Commitment, node3, ts.Block("P2.M2").ID())
			ts.IssueBlockAtSlot("P2.M4", 13, slot11Commitment, node4, ts.Block("P2.M3").ID())

			// We are going to commit 13
			ts.IssueBlockAtSlot("P2.M5", 14, slot11Commitment, node3, ts.Block("P2.M4").ID())
			ts.IssueBlockAtSlot("P2.M6", 15, slot11Commitment, node4, ts.Block("P2.M5").ID())
			ts.IssueBlockAtSlot("P2.M7", 16, slot11Commitment, node3, ts.Block("P2.M6").ID())
			ts.IssueBlockAtSlot("P2.M8", 17, slot11Commitment, node4, ts.Block("P2.M7").ID())
			ts.IssueBlockAtSlot("P2.M9", 18, slot11Commitment, node3, ts.Block("P2.M8").ID())

			// Verify that nodes have the expected states.
			ts.AssertNodeState(ts.Nodes("node3", "node4"),
				testsuite.WithLatestCommitmentSlotIndex(13),
				testsuite.WithLatestCommitmentCumulativeWeight(50), // We haven't collected any attestation yet.
				testsuite.WithLatestStateMutationSlot(0),
				testsuite.WithLatestFinalizedSlot(0), // Blocks do only commit to Genesis -> can't finalize a slot.
				testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),

				testsuite.WithSybilProtectionOnlineCommittee(expectedP2Committee),
				testsuite.WithSybilProtectionCommittee(expectedCommittee),
				testsuite.WithEvictedSlot(13),
			)

			// Upon committing 13, we included attestations up to slot 13 that committed at least to slot 11.
			ts.AssertAttestationsForSlot(13, ts.Blocks("P2.M3", "P2.M4"), node3, node4)
		}
	}

	// Both partitions should have committed slot 8 and have different commitments
	{
		// TODO: ts.AssertLatestCommitmentSlotIndex(8, ts.Nodes()...)

		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
		require.Equal(t, node3.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node4.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
		require.NotEqual(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node3.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
	}

	// Merge the partitions
	{
		ts.Network.MergePartitionsToMain()
		fmt.Println("\n=========================\nMerged network partitions\n=========================")
	}

	wg := &sync.WaitGroup{}

	detectFork := func(node *mock.Node, done func()) {
		forkDetected := make(chan struct{}, 1)
		node.Protocol.Events.ChainManager.ForkDetected.Hook(func(fork *chainmanager.Fork) {
			forkDetected <- struct{}{}
		})

		go func() {
			require.Eventually(t, func() bool {
				select {
				case <-forkDetected:
					done()
					return true
				default:
					return false
				}
			}, 25*time.Second, 10*time.Millisecond)
		}()
	}
	node1Ctx, node1Cancel := context.WithCancel(context.Background())
	defer node1Cancel()

	node2Ctx, node2Cancel := context.WithCancel(context.Background())
	defer node2Cancel()

	node3Ctx, node3Cancel := context.WithCancel(context.Background())
	defer node3Cancel()

	node4Ctx, node4Cancel := context.WithCancel(context.Background())
	defer node4Cancel()

	detectFork(node1, node1Cancel)
	detectFork(node2, node2Cancel)
	detectFork(node3, node3Cancel)
	detectFork(node4, node4Cancel)

	// Issue blocks after merging the networks
	{
		wg.Add(4)

		node1.IssueActivity(node1Ctx, 40*time.Second, wg)
		node2.IssueActivity(node2Ctx, 40*time.Second, wg)
		node3.IssueActivity(node3Ctx, 40*time.Second, wg)
		node4.IssueActivity(node4Ctx, 40*time.Second, wg)
	}

	wg.Wait()
}
