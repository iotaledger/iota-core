package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/storage"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	iotago "github.com/iotaledger/iota.go/v4"
)

func Test_StartNodeFromSnapshotAndDisk(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithLivenessThreshold(1),
		testsuite.WithMinCommittableAge(1),
		testsuite.WithMaxCommittableAge(4),
		testsuite.WithEpochNearingThreshold(2),
		testsuite.WithSlotsPerEpochExponent(3),
		testsuite.WithGenesisTimestampOffset(1000*10),
	)
	defer ts.Shutdown()

	nodeA := ts.AddValidatorNode("nodeA")
	nodeB := ts.AddValidatorNode("nodeB")
	ts.AddNode("nodeC")

	ts.Run(map[string][]options.Option[protocol.Protocol]{
		"nodeA": {
			protocol.WithStorageOptions(
				storage.WithPrunableManagerOptions(prunable.WithGranularity(1)),
				storage.WithPruningDelay(1000),
			),
		},
		"nodeB": {
			protocol.WithStorageOptions(
				storage.WithPrunableManagerOptions(prunable.WithGranularity(1)),
				storage.WithPruningDelay(1000),
			),
		},
		"nodeC": {
			protocol.WithStorageOptions(
				storage.WithPrunableManagerOptions(prunable.WithGranularity(1)),
				storage.WithPruningDelay(1000),
			),
		},
	})
	ts.HookLogging()

	ts.Wait()

	expectedCommittee := []iotago.AccountID{
		nodeA.AccountID,
		nodeB.AccountID,
	}

	expectedOnlineCommittee := []account.SeatIndex{
		lo.Return1(nodeA.Protocol.MainEngineInstance().SybilProtection.SeatManager().Committee(1).GetSeat(nodeA.AccountID)),
		lo.Return1(nodeA.Protocol.MainEngineInstance().SybilProtection.SeatManager().Committee(1).GetSeat(nodeB.AccountID)),
	}

	// Verify that nodes have the expected states.
	ts.AssertNodeState(ts.Nodes(),
		testsuite.WithSnapshotImported(true),
		testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
		testsuite.WithLatestCommitment(iotago.NewEmptyCommitment(ts.API.ProtocolParameters().Version())),
		testsuite.WithLatestFinalizedSlot(0),
		testsuite.WithChainID(iotago.NewEmptyCommitment(ts.API.ProtocolParameters().Version()).MustID()),
		testsuite.WithStorageCommitments([]*iotago.Commitment{iotago.NewEmptyCommitment(ts.API.ProtocolParameters().Version())}),
		testsuite.WithSybilProtectionCommittee(0, expectedCommittee),
		testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommittee...),
		testsuite.WithEvictedSlot(0),
		testsuite.WithActiveRootBlocks(ts.Blocks("Genesis")),
		testsuite.WithStorageRootBlocks(ts.Blocks("Genesis")),
	)

	var expectedStorageRootBlocksFrom0, expectedStorageRootBlocksFrom9 []*blocks.Block

	// Epoch 1: issue 4 rows per slot.
	{
		ts.IssueBlocksAtEpoch("", 1, 4, "Genesis", ts.Nodes(), true, nil)

		ts.AssertBlocksExist(ts.BlocksWithPrefixes("1", "2", "3", "4", "5", "6", "7"), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.BlocksWithPrefixes("7.3"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.BlocksWithPrefixes("7.3"), false, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.BlocksWithPrefixes("6", "7.0", "7.1", "7.2"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.BlocksWithPrefixes("6", "7.0", "7.1", "7.2"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.BlocksWithPrefixes("6", "7.0", "7.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.BlocksWithPrefixes("6", "7.0", "7.1"), true, ts.Nodes()...)

		var expectedActiveRootBlocks []*blocks.Block
		for _, slot := range []iotago.SlotIndex{3, 4, 5} {
			expectedActiveRootBlocks = append(expectedActiveRootBlocks, ts.BlocksWithPrefix(fmt.Sprintf("%d.3-", slot))...)
		}

		for _, slot := range []iotago.SlotIndex{1, 2, 3, 4, 5, 6} {
			expectedStorageRootBlocksFrom0 = append(expectedStorageRootBlocksFrom0, ts.BlocksWithPrefix(fmt.Sprintf("%d.3-", slot))...)
		}

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithChainID(iotago.NewEmptyCommitment(ts.API.ProtocolParameters().Version()).MustID()),
			testsuite.WithLatestFinalizedSlot(4),
			testsuite.WithLatestCommitmentSlotIndex(5),
			testsuite.WithEqualStoredCommitmentAtIndex(5),
			testsuite.WithLatestCommitmentCumulativeWeight(4), // 2 for each slot starting from 4
			testsuite.WithSybilProtectionCommittee(5, expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommittee...),
			testsuite.WithEvictedSlot(5),
			testsuite.WithActiveRootBlocks(expectedActiveRootBlocks),
			testsuite.WithStorageRootBlocks(expectedStorageRootBlocksFrom0),
		)

		for _, slot := range []iotago.SlotIndex{4, 5} {
			aliases := lo.Map([]string{"nodeA", "nodeB"}, func(s string) string {
				return fmt.Sprintf("%d.3-%s", slot, s)
			})
			ts.AssertAttestationsForSlot(slot, ts.Blocks(aliases...), ts.Nodes()...)
		}
	}

	// Epoch 2: skip slot 10 and issue 6 rows per slot
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{8, 9, 11, 12, 13}, 6, "7.3", ts.Nodes(), true, nil)

		ts.AssertBlocksExist(ts.BlocksWithPrefixes("8", "9", "11", "12", "13"), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.BlocksWithPrefixes("13.5"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.BlocksWithPrefixes("13.5"), false, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.BlocksWithPrefixes("12", "13.0", "13.1", "13.2", "13.3", "13.4"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.BlocksWithPrefixes("12", "13.0", "13.1", "13.2", "13.3", "13.4"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.BlocksWithPrefixes("12", "13.0", "13.1", "13.2", "13.3"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.BlocksWithPrefixes("12", "13.0", "13.1", "13.2", "13.3"), true, ts.Nodes()...)

		var expectedActiveRootBlocks []*blocks.Block
		for _, slot := range []iotago.SlotIndex{9, 11} {
			b := ts.BlocksWithPrefix(fmt.Sprintf("%d.5-", slot))
			expectedActiveRootBlocks = append(expectedActiveRootBlocks, b...)
			expectedStorageRootBlocksFrom9 = append(expectedStorageRootBlocksFrom9, b...)
		}

		for _, slot := range []iotago.SlotIndex{8, 9, 11} {
			expectedStorageRootBlocksFrom0 = append(expectedStorageRootBlocksFrom0, ts.BlocksWithPrefix(fmt.Sprintf("%d.5-", slot))...)
		}

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithChainID(iotago.NewEmptyCommitment(ts.API.ProtocolParameters().Version()).MustID()),
			testsuite.WithLatestFinalizedSlot(11),
			testsuite.WithLatestCommitmentSlotIndex(11),
			testsuite.WithEqualStoredCommitmentAtIndex(11),
			testsuite.WithLatestCommitmentCumulativeWeight(16), // 2 for each slot starting from 4
			testsuite.WithSybilProtectionCommittee(11, expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommittee...),
			testsuite.WithEvictedSlot(11),
			testsuite.WithActiveRootBlocks(expectedActiveRootBlocks),
			testsuite.WithStorageRootBlocks(expectedStorageRootBlocksFrom0),
		)

		for _, slot := range []iotago.SlotIndex{8, 9, 11} {
			aliases := lo.Map([]string{"nodeA", "nodeB"}, func(s string) string {
				return fmt.Sprintf("%d.5-%s", slot, s)
			})
			ts.AssertAttestationsForSlot(slot, ts.Blocks(aliases...), ts.Nodes()...)
		}
		ts.AssertAttestationsForSlot(10, ts.Blocks("9.5-nodeA", "9.5-nodeB"), ts.Nodes()...) // We didn't issue in slot 10, but we still have the attestations from slot 9 in the window.

		{
			// Restart nodeC from disk. Verify state.
			{
				nodeC := ts.Node("nodeC")
				nodeC.Shutdown()
				ts.RemoveNode("nodeC")

				nodeC1 := ts.AddNode("nodeC-restarted")
				nodeC1.CopyIdentityFromNode(nodeC)
				nodeC1.Initialize(
					protocol.WithBaseDirectory(ts.Directory.Path(nodeC.Name)),
					protocol.WithStorageOptions(
						storage.WithPrunableManagerOptions(prunable.WithGranularity(1)),
						storage.WithPruningDelay(1000),
					),
					protocol.WithEngineOptions(
						engine.WithRequesterOptions(
							eventticker.RetryInterval[iotago.SlotIndex, iotago.BlockID](300*time.Millisecond),
							eventticker.RetryJitter[iotago.SlotIndex, iotago.BlockID](100*time.Millisecond),
						),
					),
				)
				nodeC1.HookLogging()
				ts.Wait()

				// Everything that was accepted before shutting down should be available on disk (verifying that restoring the block cache from disk works).
				ts.AssertBlocksExist(ts.BlocksWithPrefixes("8", "9", "11", "12", "13.0", "13.1", "13.2", "13.3"), true, ts.Nodes("nodeC-restarted")...)
				ts.AssertStorageRootBlocks(expectedStorageRootBlocksFrom0, ts.Nodes("nodeC-restarted")...)

				for _, slot := range []iotago.SlotIndex{8, 9, 11} {
					aliases := lo.Map([]string{"nodeA", "nodeB"}, func(s string) string {
						return fmt.Sprintf("%d.5-%s", slot, s)
					})
					ts.AssertAttestationsForSlot(slot, ts.Blocks(aliases...), ts.Nodes()...)
				}
				ts.AssertAttestationsForSlot(10, ts.Blocks("9.5-nodeA", "9.5-nodeB"), ts.Nodes()...) // We didn't issue in slot 10, but we still have the attestations from slot 9 in the window.
			}

			// Start a new node (nodeD) from a snapshot. Verify state.
			{
				// Create snapshot.
				snapshotPath := ts.Directory.Path(fmt.Sprintf("%d_snapshot", time.Now().Unix()))
				require.NoError(t, ts.Node("nodeA").Protocol.MainEngineInstance().WriteSnapshot(snapshotPath))

				nodeD := ts.AddNode("nodeD")
				nodeD.CopyIdentityFromNode(ts.Node("nodeC-restarted")) // we just want to be able to issue some stuff and don't care about the account for now.
				nodeD.Initialize(
					protocol.WithSnapshotPath(snapshotPath),
					protocol.WithBaseDirectory(ts.Directory.PathWithCreate(nodeD.Name)),
					protocol.WithEngineOptions(
						engine.WithRequesterOptions(
							eventticker.RetryInterval[iotago.SlotIndex, iotago.BlockID](300*time.Millisecond),
							eventticker.RetryJitter[iotago.SlotIndex, iotago.BlockID](100*time.Millisecond),
						),
					),
				)
				ts.Wait()
				nodeD.HookLogging()

				ts.AssertStorageRootBlocks(expectedStorageRootBlocksFrom9, ts.Nodes("nodeD")...)
			}

			slot7Commitment := lo.PanicOnErr(nodeA.Protocol.MainEngineInstance().Storage.Commitments().Load(7))

			ts.AssertNodeState(ts.Nodes("nodeC-restarted", "nodeD"),
				testsuite.WithSnapshotImported(true),
				testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
				testsuite.WithChainID(slot7Commitment.ID()),
				testsuite.WithLatestFinalizedSlot(11),
				testsuite.WithLatestCommitmentSlotIndex(11),
				testsuite.WithEqualStoredCommitmentAtIndex(11),
				testsuite.WithLatestCommitmentCumulativeWeight(16), // 2 for each slot starting from 4
				testsuite.WithSybilProtectionCommittee(11, expectedCommittee),
				testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommittee...),
				testsuite.WithEvictedSlot(11),
				testsuite.WithActiveRootBlocks(expectedActiveRootBlocks),
				testsuite.WithChainManagerIsSolid(),
			)
		}

		// Only issue on nodes that have the latest state in memory.
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{14, 15}, 6, "13.5", ts.Nodes("nodeA", "nodeB"), true, nil)

		for _, slot := range []iotago.SlotIndex{12, 13} {
			aliases := lo.Map([]string{"nodeA", "nodeB"}, func(s string) string {
				return fmt.Sprintf("%d.5-%s", slot, s)
			})
			ts.AssertAttestationsForSlot(slot, ts.Blocks(aliases...), ts.Nodes()...)

			rootBlocks := ts.BlocksWithPrefix(fmt.Sprintf("%d.5", slot))
			expectedStorageRootBlocksFrom0 = append(expectedStorageRootBlocksFrom0, rootBlocks...)
			expectedStorageRootBlocksFrom9 = append(expectedStorageRootBlocksFrom9, rootBlocks...)
		}

		ts.AssertStorageRootBlocks(expectedStorageRootBlocksFrom0, ts.Nodes("nodeA", "nodeB", "nodeC-restarted")...)
		ts.AssertStorageRootBlocks(expectedStorageRootBlocksFrom9, ts.Nodes("nodeD")...)
	}

	// Epoch 3-5
	{
		// Issue on all nodes except nodeD as its account is not yet known.
		ts.IssueBlocksAtEpoch("", 3, 4, "15.5", ts.Nodes(), true, nil)

		// Issue on all nodes.
		ts.IssueBlocksAtEpoch("", 4, 4, "23.3", ts.Nodes(), true, nil)
		ts.IssueBlocksAtEpoch("", 5, 4, "31.3", ts.Nodes(), true, nil)

		var expectedActiveRootBlocks []*blocks.Block
		for _, slot := range []iotago.SlotIndex{35, 36, 37} {
			expectedActiveRootBlocks = append(expectedActiveRootBlocks, ts.BlocksWithPrefix(fmt.Sprintf("%d.3-", slot))...)
		}

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestFinalizedSlot(36),
			testsuite.WithLatestCommitmentSlotIndex(37),
			testsuite.WithEqualStoredCommitmentAtIndex(37),
			testsuite.WithLatestCommitmentCumulativeWeight(68), // 2 for each slot starting from 4
			testsuite.WithSybilProtectionCommittee(37, expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommittee...),
			testsuite.WithEvictedSlot(37),
			testsuite.WithActiveRootBlocks(expectedActiveRootBlocks),
		)

		acceptedSlots := ts.SlotsForEpoch(3)
		acceptedSlots = append(acceptedSlots, ts.SlotsForEpoch(4)...)
		acceptedSlots = append(acceptedSlots, 32, 33, 34, 35, 36, 37)
		for _, slot := range acceptedSlots {
			aliases := lo.Map([]string{"nodeA", "nodeB"}, func(s string) string {
				return fmt.Sprintf("%d.3-%s", slot, s)
			})
			ts.AssertAttestationsForSlot(slot, ts.Blocks(aliases...), ts.Nodes()...)

			rootBlocks := ts.BlocksWithPrefix(fmt.Sprintf("%d.3", slot))
			expectedStorageRootBlocksFrom0 = append(expectedStorageRootBlocksFrom0, rootBlocks...)
			expectedStorageRootBlocksFrom9 = append(expectedStorageRootBlocksFrom9, rootBlocks...)
		}

		ts.AssertStorageRootBlocks(expectedStorageRootBlocksFrom0, ts.Nodes("nodeA", "nodeB", "nodeC-restarted")...)
		ts.AssertStorageRootBlocks(expectedStorageRootBlocksFrom9, ts.Nodes("nodeD")...)
	}
}
