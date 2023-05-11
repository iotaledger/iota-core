package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection/poa"
	"github.com/iotaledger/iota-core/pkg/storage"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestProtocol_StartNodeFromSnapshotAndDisk(t *testing.T) {
	ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1", 50)
	node2 := ts.AddValidatorNode("node2", 50)

	ts.Run(map[string][]options.Option[protocol.Protocol]{
		"node1": {
			protocol.WithPruningThreshold(2),
			protocol.WithStorageOptions(
				storage.WithPrunableManagerOptions(prunable.WithGranularity(1)),
			),
		},
		"node2": {
			protocol.WithPruningThreshold(4),
		},
	})
	ts.HookLogging()

	ts.Wait()

	expectedCommittee := map[iotago.AccountID]int64{
		node1.AccountID: 50,
		node2.AccountID: 50,
	}

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
		testsuite.WithSybilProtectionOnlineCommittee(expectedCommittee),
		testsuite.WithEvictedSlot(0),
		testsuite.WithActiveRootBlocks(ts.Blocks("Genesis")),
		testsuite.WithStorageRootBlocks(ts.Blocks("Genesis")),
	)

	// Issue blocks in subsequent slots and make sure that node state as well as accepted, ratified accepted, and confirmed blocks are correct.
	{
		// Slot 1-2
		{
			// Slot 1
			ts.IssueBlockAtSlot("1.1", 1, iotago.NewEmptyCommitment(), node1, iotago.EmptyBlockID())
			ts.IssueBlockAtSlot("1.2", 1, iotago.NewEmptyCommitment(), node2, iotago.EmptyBlockID())
			ts.IssueBlockAtSlot("1.1*", 1, iotago.NewEmptyCommitment(), node1, ts.BlockID("1.2"))

			// Slot 2
			ts.IssueBlockAtSlot("2.2", 2, iotago.NewEmptyCommitment(), node2, ts.BlockID("1.1"))
			ts.IssueBlockAtSlot("2.2*", 2, iotago.NewEmptyCommitment(), node2, ts.BlockID("1.1*"))

			ts.AssertBlocksExist(ts.Blocks("1.1", "1.2", "1.1*", "2.2", "2.2*"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("1.1", "1.2", "1.1*"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("2.2", "2.2*"), false, ts.Nodes()...)
		}

		// Slot 3-6
		{
			// Slot 3
			ts.IssueBlockAtSlot("3.1", 3, iotago.NewEmptyCommitment(), node1, ts.BlockIDs("2.2", "2.2*")...)

			ts.AssertBlocksExist(ts.Blocks("3.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("2.2", "2.2*"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("3.1"), false, ts.Nodes()...)

			ts.AssertBlocksInCacheRatifiedAccepted(ts.Blocks("1.2"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("1.2"), true, ts.Nodes()...)

			// Slot 4
			ts.IssueBlockAtSlot("4.2", 4, iotago.NewEmptyCommitment(), node2, ts.BlockID("3.1"))

			ts.AssertBlocksExist(ts.Blocks("4.2"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("3.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("4.2"), false, ts.Nodes()...)

			ts.AssertBlocksInCacheRatifiedAccepted(ts.Blocks("1.1", "1.1*"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("1.1", "1.1*"), true, ts.Nodes()...)

			// Slot 5
			ts.IssueBlockAtSlot("5.1", 5, iotago.NewEmptyCommitment(), node1, ts.BlockID("4.2"))

			ts.AssertBlocksExist(ts.Blocks("5.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("4.2"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("5.1"), false, ts.Nodes()...)

			ts.AssertBlocksInCacheRatifiedAccepted(ts.Blocks("2.2", "2.2*"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("2.2", "2.2*"), true, ts.Nodes()...)

			// Slot 6
			ts.IssueBlockAtSlot("6.2", 6, iotago.NewEmptyCommitment(), node2, ts.BlockID("5.1"))

			ts.AssertBlocksExist(ts.Blocks("6.2"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("5.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("6.2"), false, ts.Nodes()...)

			ts.AssertBlocksInCacheRatifiedAccepted(ts.Blocks("3.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("3.1"), true, ts.Nodes()...)
		}

		// Verify nodes' states: Slot 1 should be committed as the MinCommittableSlotAge is 1, and we accepted a block at slot 3.
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(1),
			testsuite.WithLatestStateMutationSlot(0),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedCommittee),
			testsuite.WithEvictedSlot(1),
			testsuite.WithActiveRootBlocks(ts.Blocks("Genesis", "1.1", "1.1*")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis", "1.1", "1.1*", "2.2", "2.2*")),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())

		// Slot 7-8
		{
			slot1Commitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(1)).Commitment()

			// Slot 7
			ts.IssueBlockAtSlot("7.1", 7, slot1Commitment, node1, ts.BlockID("6.2"))
			// Slot 8
			ts.IssueBlockAtSlot("8.2", 8, slot1Commitment, node2, ts.BlockID("7.1"))

			ts.AssertBlocksExist(ts.Blocks("7.1", "8.2"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("6.2", "7.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("8.2"), false, ts.Nodes()...)

			ts.AssertBlocksInCacheRatifiedAccepted(ts.Blocks("2.2", "2.2*", "3.1", "4.2", "5.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("2.2", "2.2*", "3.1", "4.2", "5.1"), true, ts.Nodes()...)
		}

		// Verify nodes' states:
		// - Slot 3 should be committed as the MinCommittableSlotAge is 1, and we ratified accepted a block at slot 5.
		// - 5.1 is ratified accepted and commits to slot 1 -> slot 1 should be evicted.
		// - rootblocks are still not evicted as RootBlocksEvictionDelay is 3.
		// - slot 1 is still not finalized: there is no supermajority of ratified accepted blocks that commits to it.
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(3),
			testsuite.WithLatestStateMutationSlot(0),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedCommittee),
			testsuite.WithEvictedSlot(3),
			testsuite.WithActiveRootBlocks(ts.Blocks("1.1", "1.1*", "2.2", "2.2*", "3.1")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis", "1.1", "1.1*", "2.2", "2.2*", "3.1", "4.2")),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())

		// Slot 9-12
		{
			slot3Commitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(3)).Commitment()

			// Slot 9
			ts.IssueBlockAtSlot("9.1", 9, slot3Commitment, node1, ts.BlockID("8.2"))
			// Slot 10
			ts.IssueBlockAtSlot("10.2", 10, slot3Commitment, node2, ts.BlockID("9.1"))
			// Slot 11
			ts.IssueBlockAtSlot("11.1", 11, slot3Commitment, node1, ts.BlockID("10.2"))
			// Slot 12
			ts.IssueBlockAtSlot("12.2", 12, slot3Commitment, node2, ts.BlockID("11.1"))

			ts.AssertBlocksExist(ts.Blocks("9.1", "10.2", "11.1", "12.2"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("8.2", "9.1", "10.2", "11.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("12.2"), false, ts.Nodes()...)

			ts.AssertBlocksInCacheRatifiedAccepted(ts.Blocks("6.2", "7.1", "8.2", "9.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("6.2", "7.1", "8.2", "9.1"), true, ts.Nodes()...)
		}

		// Verify nodes' states:
		// - Slot 9 should be committed as the MinCommittableSlotAge is 1, and we accepted a block at slot 11.
		// - 9.1 is ratified accepted and commits to slot 5 -> slot 5 should be evicted.
		// - rootblocks are evicted until slot 2 as RootBlocksEvictionDelay is 3.
		// - slot 1 is finalized: there is a supermajority of ratified accepted blocks that commits to it.
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(7),
			testsuite.WithLatestStateMutationSlot(0),
			testsuite.WithLatestFinalizedSlot(1),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedCommittee),
			testsuite.WithEvictedSlot(7),
			testsuite.WithActiveRootBlocks(ts.Blocks("5.1", "6.2", "7.1")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis", "1.1", "1.1*", "2.2", "2.2*", "3.1", "4.2", "5.1", "6.2", "7.1", "8.2")),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())

		// Slot 13
		{
			slot7Commitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(7)).Commitment()
			ts.IssueBlockAtSlot("13.1", 13, slot7Commitment, node1, ts.BlockID("12.2"))

			ts.AssertBlocksExist(ts.Blocks("13.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("12.2"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("13.1"), false, ts.Nodes()...)

			ts.AssertBlocksInCacheRatifiedAccepted(ts.Blocks("10.2"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("10.2"), true, ts.Nodes()...)
		}

		// Verify nodes' states:
		// - Slot 10 should be committed as the MinCommittableSlotAge is 1, and we accepted a block at slot 12.
		// - 10.2 is ratified accepted and commits to slot 5 -> slot 5 should be evicted.
		// - rootblocks are evicted until slot 2 as RootBlocksEvictionDelay is 3.
		// - slot 5 is finalized: there is a supermajority of ratified accepted blocks that commits to it.

		ts.AssertNodeState(ts.Nodes("node1"),
			testsuite.WithStorageRootBlocks(ts.Blocks("2.2", "2.2*", "3.1", "4.2", "5.1", "6.2", "7.1", "8.2", "9.1")),
			testsuite.WithPrunedSlot(1, true),
		)

		ts.AssertNodeState(ts.Nodes("node2"),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis", "1.1", "1.1*", "2.2", "2.2*", "3.1", "4.2", "5.1", "6.2", "7.1", "8.2", "9.1")),
			testsuite.WithPrunedSlot(0, false),
		)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(8),
			testsuite.WithLatestStateMutationSlot(0),
			testsuite.WithLatestFinalizedSlot(3),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedCommittee),
			testsuite.WithEvictedSlot(8),
			testsuite.WithActiveRootBlocks(ts.Blocks("6.2", "7.1", "8.2")),
			// testsuite.WithStorageRootBlocks(ts.Blocks("Genesis", "1.1", "1.1*", "2.2", "2.2*", "3.1", "4.2", "5.1", "6.2", "7.1", "8.2", "9.1")),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())

		// Slot 14
		{
			slot8Commitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(8)).Commitment()
			ts.IssueBlockAtSlot("14.2", 14, slot8Commitment, node2, ts.BlockID("13.1"))

			ts.AssertBlocksExist(ts.Blocks("14.2"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("13.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("14.2"), false, ts.Nodes()...)

			ts.AssertBlocksInCacheRatifiedAccepted(ts.Blocks("11.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("11.1"), true, ts.Nodes()...)
		}

		// Verify nodes' states:
		// - Slot 10 should be committed as the MinCommittableSlotAge is 1, and we accepted a block at slot 12.
		// - 10.2 is ratified accepted and commits to slot 5 -> slot 5 should be evicted.
		// - rootblocks are evicted until slot 2 as RootBlocksEvictionDelay is 3.
		// - slot 5 is finalized: there is a supermajority of ratified accepted blocks that commits to it.
		ts.AssertNodeState(ts.Nodes("node1"),
			testsuite.WithStorageRootBlocks(ts.Blocks("2.2", "2.2*", "3.1", "4.2", "5.1", "6.2", "7.1", "8.2", "9.1")),
			testsuite.WithPrunedSlot(1, true),
		)

		ts.AssertNodeState(ts.Nodes("node2"),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis", "1.1", "1.1*", "2.2", "2.2*", "3.1", "4.2", "5.1", "6.2", "7.1", "8.2", "9.1")),
			testsuite.WithPrunedSlot(0, false),
		)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(9),
			testsuite.WithLatestStateMutationSlot(0),
			testsuite.WithLatestFinalizedSlot(3),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedCommittee),
			testsuite.WithEvictedSlot(9),
			testsuite.WithActiveRootBlocks(ts.Blocks("7.1", "8.2", "9.1")),
			// testsuite.WithStorageRootBlocks(ts.Blocks("Genesis", "1.1", "1.1*", "2.2", "2.2*", "3.1", "4.2", "5.1", "6.2", "7.1", "8.2", "9.1", "10.2")),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
	}

	// Shutdown node2 and restart it from disk. Verify state.
	{
		node2.Shutdown()
		ts.RemoveNode("node2")

		node21 := ts.AddNode("node2.1")
		node21.CopyIdentityFromNode(node2)
		fmt.Println("validators", ts.Validators())
		node21.Initialize(
			protocol.WithBaseDirectory(ts.Directory.Path(node2.Name)),
			protocol.WithSybilProtectionProvider(
				poa.NewProvider(ts.Validators()),
			),
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(slotnotarization.WithMinCommittableSlotAge(1)),
			),
		)
		ts.Wait()

		slot1Commitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(1)).Commitment()
		ts.AssertNodeState(ts.Nodes("node2.1"),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(9),
			testsuite.WithLatestStateMutationSlot(0),
			testsuite.WithLatestFinalizedSlot(3),
			testsuite.WithChainID(slot1Commitment.MustID()),
			testsuite.WithSybilProtectionCommittee(expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedCommittee),
			testsuite.WithEvictedSlot(9),
			testsuite.WithActiveRootBlocks(ts.Blocks("7.1", "8.2", "9.1")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis", "1.1", "1.1*", "2.2", "2.2*", "3.1", "4.2", "5.1", "6.2", "7.1", "8.2", "9.1", "10.2")),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node21.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
	}

	// Create snapshot.
	snapshotPath := ts.Directory.Path(fmt.Sprintf("%d_snapshot", time.Now().Unix()))
	require.NoError(t, node1.Protocol.MainEngineInstance().WriteSnapshot(snapshotPath))

	// Load node3 from created snapshot and verify state.
	{
		node3 := ts.AddNode("node3")
		node3.Initialize(
			protocol.WithSnapshotPath(snapshotPath),
			protocol.WithBaseDirectory(ts.Directory.PathWithCreate(node3.Name)),
			protocol.WithSybilProtectionProvider(
				poa.NewProvider(ts.Validators()),
			),
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(slotnotarization.WithMinCommittableSlotAge(1)),
			),
			protocol.WithPruningThreshold(2),
			protocol.WithStorageOptions(
				storage.WithPrunableManagerOptions(prunable.WithGranularity(1)),
			),
		)
		ts.Wait()

		slot1Commitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(1)).Commitment()
		latestCommitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(9)).Commitment()
		// Verify node3 state:
		// - Commitment at slot 8 should be the latest commitment.
		// - 8-3 (RootBlocksEvictionDelay) = 5 -> rootblocks from slot 6 until 8 (count of 3).
		// - ChainID is defined by the earliest commitment of the rootblocks -> block 8.2 commits to slot 1.
		// - slot 3 is finalized as per snapshot.
		ts.AssertNodeState(ts.Nodes("node3"),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(9),
			testsuite.WithLatestCommitment(latestCommitment),
			testsuite.WithLatestStateMutationSlot(0),
			testsuite.WithLatestFinalizedSlot(3),
			testsuite.WithChainID(slot1Commitment.MustID()),
			testsuite.WithSybilProtectionCommittee(expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedCommittee),
			testsuite.WithEvictedSlot(9),
			testsuite.WithActiveRootBlocks(ts.Blocks("7.1", "8.2", "9.1")),
			testsuite.WithStorageRootBlocks(ts.Blocks("7.1", "8.2", "9.1")),
			testsuite.WithPrunedSlot(3, true),
		)
		require.Nil(t, node3.Protocol.MainEngineInstance().Storage.RootBlocks(2))
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node3.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
	}

	{
		// Slot 15
		{
			node21 := ts.Node("node2.1")
			slot9Commitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(9)).Commitment()
			ts.IssueBlockAtSlot("15.1", 15, slot9Commitment, node1, ts.BlockID("14.2"))
			ts.IssueBlockAtSlot("16.2", 16, slot9Commitment, node21, ts.BlockID("15.1"))

			ts.AssertBlocksExist(ts.Blocks("15.1", "16.2"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("14.2", "15.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheAccepted(ts.Blocks("16.2"), false, ts.Nodes()...)

			ts.AssertBlocksInCacheRatifiedAccepted(ts.Blocks("13.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("13.1"), true, ts.Nodes()...)
		}
	}
}
