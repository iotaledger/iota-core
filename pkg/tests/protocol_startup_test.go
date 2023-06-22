package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/attestation/slotattestation"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection/poa"
	"github.com/iotaledger/iota-core/pkg/storage"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TODO: implement a similar test, but in which one slot is skipped
// (no committment - no account diffs, no root blocks etc. to make sure that this scenario is handled properly).
func TestProtocol_StartNodeFromSnapshotAndDisk(t *testing.T) {
	ts := testsuite.NewTestSuite(t, testsuite.WithGenesisTimestampOffset(100*10))
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddValidatorNode("node2")

	ts.Run(map[string][]options.Option[protocol.Protocol]{
		"node1": {
			protocol.WithStorageOptions(
				storage.WithPrunableManagerOptions(prunable.WithGranularity(1)),
				storage.WithPruningDelay(3),
			),
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(1),
			),
			protocol.WithAttestationProvider(
				slotattestation.NewProvider(3),
			),
		},
		"node2": {
			protocol.WithStorageOptions(
				storage.WithPrunableManagerOptions(prunable.WithGranularity(1)),
				storage.WithPruningDelay(4),
			),
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(1),
			),
			protocol.WithAttestationProvider(
				slotattestation.NewProvider(3),
			),
		},
	})
	ts.HookLogging()

	ts.Wait()

	expectedCommittee := []iotago.AccountID{
		node1.AccountID,
		node2.AccountID,
	}

	// Verify that nodes have the expected states.
	ts.AssertNodeState(ts.Nodes(),
		testsuite.WithSnapshotImported(true),
		testsuite.WithProtocolParameters(ts.ProtocolParameters),
		testsuite.WithLatestCommitment(iotago.NewEmptyCommitment()),
		testsuite.WithLatestFinalizedSlot(0),
		testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
		testsuite.WithStorageCommitments([]*iotago.Commitment{iotago.NewEmptyCommitment()}),
		testsuite.WithSybilProtectionCommittee(0, expectedCommittee),
		testsuite.WithSybilProtectionOnlineCommittee([]account.SeatIndex{0, 1}),
		testsuite.WithEvictedSlot(0),
		testsuite.WithActiveRootBlocks(ts.Blocks("Genesis")),
		testsuite.WithStorageRootBlocks(ts.Blocks("Genesis")),
	)

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
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("1.1", "1.2", "1.1*"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("2.2", "2.2*"), false, ts.Nodes()...)
	}

	// Slot 3-6
	{
		// Slot 3
		ts.IssueBlockAtSlot("3.1", 3, iotago.NewEmptyCommitment(), node1, ts.BlockIDs("2.2", "2.2*")...)

		ts.AssertBlocksExist(ts.Blocks("3.1"), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("2.2", "2.2*"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("2.2", "2.2*"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("3.1"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("3.1"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("1.2"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("1.2"), true, ts.Nodes()...)

		// Slot 4
		ts.IssueBlockAtSlot("4.2", 4, iotago.NewEmptyCommitment(), node2, ts.BlockID("3.1"))

		ts.AssertBlocksExist(ts.Blocks("4.2"), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("3.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("3.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("4.2"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("4.2"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("1.1", "1.1*"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("1.1", "1.1*"), false, ts.Nodes()...) // too old. confirmation ratification threshold = 2

		// Slot 5
		ts.IssueBlockAtSlot("5.1", 5, iotago.NewEmptyCommitment(), node1, ts.BlockID("4.2"))

		ts.AssertBlocksExist(ts.Blocks("5.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("4.2"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("4.2"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("5.1"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("5.1"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("2.2", "2.2*"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("2.2", "2.2*"), false, ts.Nodes()...) // too old. confirmation ratification threshold = 2

		// Slot 6
		ts.IssueBlockAtSlot("6.2", 6, iotago.NewEmptyCommitment(), node2, ts.BlockID("5.1"))

		ts.AssertBlocksExist(ts.Blocks("6.2"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("5.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("5.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("6.2"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("6.2"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("3.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("3.1"), false, ts.Nodes()...) // too old. confirmation ratification threshold = 2

		// Verify nodes' states: Slot 1 should be committed as the MinCommittableSlotAge is 1, and we accepted a block at slot 3.
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(1),
			testsuite.WithEqualStoredCommitmentAtIndex(1),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(6, expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee([]account.SeatIndex{0, 1}),
			testsuite.WithEvictedSlot(1),
			testsuite.WithActiveRootBlocks(ts.Blocks("Genesis", "1.1", "1.1*")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis", "1.1", "1.1*", "2.2", "2.2*")),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
	}

	// Slot 7-8
	{
		slot1Commitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(1)).Commitment()

		// Slot 7
		ts.IssueBlockAtSlot("7.1", 7, slot1Commitment, node1, ts.BlockID("6.2"))
		// Slot 8
		ts.IssueBlockAtSlot("8.2", 8, slot1Commitment, node2, ts.BlockID("7.1"))

		ts.AssertBlocksExist(ts.Blocks("7.1", "8.2"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("6.2", "7.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("6.2", "7.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("8.2"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("8.2"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("4.2", "5.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("4.2", "5.1"), false, ts.Nodes()...) // too old. confirmation ratification threshold = 2

		// Verify nodes' states:
		// - Slot 3 should be committed as the MinCommittableSlotAge is 1, and we accepted a block at slot 5.
		// - 5.1 is accepted and commits to slot 1 -> slot 1 should be evicted.
		// - rootblocks are still not evicted as RootBlocksEvictionDelay is 3.
		// - slot 1 is still not finalized: there is no supermajority of confirmed blocks that commits to it.
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(3),
			testsuite.WithEqualStoredCommitmentAtIndex(3),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(8, expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee([]account.SeatIndex{0, 1}),
			testsuite.WithEvictedSlot(3),
			testsuite.WithActiveRootBlocks(ts.Blocks("1.1", "1.1*", "2.2", "2.2*", "3.1")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis", "1.1", "1.1*", "2.2", "2.2*", "3.1", "4.2")),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())

		ts.AssertLatestCommitmentCumulativeWeight(2, ts.Nodes()...)
		ts.AssertAttestationsForSlot(3, ts.Blocks("3.1", "2.2*"), ts.Nodes()...)
	}

	// Make slot 7 committed.
	{
		slot3Commitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(3)).Commitment()
		ts.IssueBlockAtSlot("9.1.2", 9, slot3Commitment, node1, ts.BlockID("8.2"))
		ts.IssueBlockAtSlot("9.2.2", 9, slot3Commitment, node2, ts.BlockID("9.1.2"))
		ts.IssueBlockAtSlot("9.1.3", 9, slot3Commitment, node1, ts.BlockID("9.2.2"))
		ts.IssueBlockAtSlot("9.2.3", 9, slot3Commitment, node2, ts.BlockID("9.1.3"))

		ts.AssertBlocksExist(ts.Blocks("9.1.2", "9.2.2", "9.1.3", "9.2.3"), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("9.1.2", "9.2.2", "9.1.3"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("9.1.2", "9.2.2", "9.1.3"), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("9.2.3"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("9.2.3"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("8.2", "9.1.2"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("8.2", "9.1.2"), true, ts.Nodes()...)

		// Verify nodes' states:
		// - Slot 3 should be committed as the MinCommittableSlotAge is 1, and we accepted a block at slot 5.
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(7),
			testsuite.WithEqualStoredCommitmentAtIndex(7),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(9, expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee([]account.SeatIndex{0, 1}),
			testsuite.WithEvictedSlot(7),
			testsuite.WithActiveRootBlocks(ts.Blocks("5.1", "6.2", "7.1")),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
	}

	// Slot 9-12
	{
		slot7Commitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(7)).Commitment()

		// Slot 9
		ts.IssueBlockAtSlot("9.1", 9, slot7Commitment, node1, ts.BlockID("9.2.3"))
		ts.IssueBlockAtSlot("9.2", 9, slot7Commitment, node2, ts.BlockID("9.1"))
		// Slot 10
		ts.IssueBlockAtSlot("10.2", 10, slot7Commitment, node2, ts.BlockID("9.2"))
		// Slot 11
		ts.IssueBlockAtSlot("11.1", 11, slot7Commitment, node1, ts.BlockID("10.2"))
		// Slot 12
		ts.IssueBlockAtSlot("12.2", 12, slot7Commitment, node2, ts.BlockID("11.1"))
		ts.IssueBlockAtSlot("12.1", 12, slot7Commitment, node1, ts.BlockID("11.1"))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("10.2", "11.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("10.2", "11.1"), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("12.2", "12.1"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("12.2", "12.1"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("9.1.2", "9.2.2", "9.1.3", "9.2.3", "9.1", "9.2"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("9.1.2", "9.2.2", "9.1.3", "9.2.3"), true, ts.Nodes()...) // too old. confirmation ratification threshold = 2

		// Verify nodes' states:
		// - Slot 7 should be committed as the MinCommittableSlotAge is 1, and we accepted a block at slot 9.
		// - rootblocks are evicted until slot 5 as RootBlocksEvictionDelay is 3.
		// - slot 3 is finalized as "9.1.2", "9.2.2", "9.1.3", "9.2.3" are confirmed within slot 9 being a supermajority.
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(7),
			testsuite.WithEqualStoredCommitmentAtIndex(7),
			testsuite.WithLatestFinalizedSlot(3),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(12, expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee([]account.SeatIndex{0, 1}),
			testsuite.WithEvictedSlot(7),
			testsuite.WithActiveRootBlocks(ts.Blocks("5.1", "6.2", "7.1")),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())

		ts.AssertNodeState(ts.Nodes("node1"),
			testsuite.WithStorageRootBlocks(ts.Blocks("1.1", "1.1*", "2.2", "2.2*", "3.1", "4.2", "5.1", "6.2", "7.1")),
			testsuite.WithPrunedSlot(0, true),
		)

		ts.AssertNodeState(ts.Nodes("node2"),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis", "1.1", "1.1*", "2.2", "2.2*", "3.1", "4.2", "5.1", "6.2", "7.1")),
			testsuite.WithPrunedSlot(0, false),
		)
	}

	// Slot 13
	{
		slot7Commitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(7)).Commitment()
		ts.IssueBlockAtSlot("13.1", 13, slot7Commitment, node1, ts.BlockIDs("12.2", "12.1")...)
		ts.IssueBlockAtSlot("13.2", 13, slot7Commitment, node2, ts.BlockIDs("12.2", "12.1")...)
		ts.IssueBlockAtSlot("13.1.1", 13, slot7Commitment, node1, ts.BlockIDs("13.1", "13.2")...)
		ts.IssueBlockAtSlot("13.2.1", 13, slot7Commitment, node2, ts.BlockIDs("13.1", "13.2")...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("12.2", "12.1", "13.2", "13.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("12.2", "12.1", "13.2", "13.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("13.1.1", "13.2.1"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("13.1.1", "13.2.1"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("11.1", "12.2", "12.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("11.1", "12.2", "12.1"), true, ts.Nodes()...)

		// Verify nodes' states:
		// - Slot 10 should be committed as the MinCommittableSlotAge is 1, and we accepted a block at slot 12.
		// - rootblocks are evicted until slot 8 as RootBlocksEvictionDelay is 3.
		// - Slot 7 is finalized: there is a supermajority of confirmed blocks that commits to it.
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(10),
			testsuite.WithEqualStoredCommitmentAtIndex(10),
			testsuite.WithLatestFinalizedSlot(7),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(13, expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee([]account.SeatIndex{0, 1}),
			testsuite.WithEvictedSlot(10),
			testsuite.WithActiveRootBlocks(ts.Blocks("8.2", "9.2", "10.2")),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())

		ts.AssertNodeState(ts.Nodes("node1"),
			testsuite.WithStorageRootBlocks(ts.Blocks("5.1", "6.2", "7.1", "8.2", "9.2")),
			testsuite.WithPrunedSlot(4, true),
		)

		ts.AssertNodeState(ts.Nodes("node2"),
			testsuite.WithStorageRootBlocks(ts.Blocks("4.2", "5.1", "6.2", "7.1", "8.2", "9.2")),
			testsuite.WithPrunedSlot(3, true),
		)
	}

	// Slot 14
	{
		slot8Commitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(8)).Commitment()
		ts.IssueBlockAtSlot("14.2", 14, slot8Commitment, node2, ts.BlockIDs("13.1.1", "13.2.1")...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("13.1.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("13.1.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("13.2.1", "14.2"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("13.2.1", "14.2"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("12.2", "12.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("12.2", "12.1"), true, ts.Nodes()...)

		// Verify nodes' states:
		// - Slot 10 should be committed as the MinCommittableSlotAge is 1, and we accepted a block at slot 12.
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(10),
			testsuite.WithEqualStoredCommitmentAtIndex(10),
			testsuite.WithLatestFinalizedSlot(7),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(14, expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee([]account.SeatIndex{0, 1}),
			testsuite.WithEvictedSlot(10),
			testsuite.WithActiveRootBlocks(ts.Blocks("8.2", "9.2", "10.2")),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())

		// We have committed to slot 9 where we referenced slot 7 with commitments -> there should be cumulative weight and attestations for slot 9.
		ts.AssertLatestCommitmentCumulativeWeight(6, ts.Nodes()...)
		ts.AssertAttestationsForSlot(9, ts.Blocks("9.1", "9.2"), ts.Nodes()...)
		ts.AssertAttestationsForSlot(10, ts.Blocks("9.1", "10.2"), ts.Nodes()...)

		ts.AssertNodeState(ts.Nodes("node1"),
			testsuite.WithStorageRootBlocks(ts.Blocks("5.1", "6.2", "7.1", "8.2", "9.2")),
			testsuite.WithPrunedSlot(4, true),
		)

		ts.AssertNodeState(ts.Nodes("node2"),
			testsuite.WithStorageRootBlocks(ts.Blocks("4.2", "5.1", "6.2", "7.1", "8.2", "9.2")),
			testsuite.WithPrunedSlot(3, true),
		)
	}

	// Shutdown node2 and restart it from disk. Verify state.
	{
		node2.Shutdown()
		ts.RemoveNode("node2")

		node21 := ts.AddNode("node2.1")
		node21.CopyIdentityFromNode(node2)
		node21.Initialize(
			protocol.WithBaseDirectory(ts.Directory.Path(node2.Name)),
			protocol.WithSybilProtectionProvider(
				poa.NewProvider(ts.Validators()),
			),
			protocol.WithStorageOptions(
				storage.WithPrunableManagerOptions(prunable.WithGranularity(1)),
				storage.WithPruningDelay(4),
			),
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(1),
			),
			protocol.WithAttestationProvider(
				slotattestation.NewProvider(3),
			),
		)
		ts.Wait()

		ts.AssertNodeState(ts.Nodes("node2.1"),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(10),
			testsuite.WithEqualStoredCommitmentAtIndex(10),
			testsuite.WithLatestFinalizedSlot(7),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(10, expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee([]account.SeatIndex{0, 1}),
			testsuite.WithEvictedSlot(10),
			testsuite.WithActiveRootBlocks(ts.Blocks("8.2", "9.2", "10.2")),
			testsuite.WithStorageRootBlocks(ts.Blocks("4.2", "5.1", "6.2", "7.1", "8.2", "9.2")),
			testsuite.WithPrunedSlot(3, true),
			testsuite.WithChainManagerIsSolid(),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node21.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())

		// Verify attestations state.
		ts.AssertLatestCommitmentCumulativeWeight(6, ts.Nodes()...)
		ts.AssertAttestationsForSlot(9, ts.Blocks("9.1", "9.2"), ts.Nodes()...)
		ts.AssertAttestationsForSlot(10, ts.Blocks("9.1", "10.2"), ts.Nodes()...)
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
			protocol.WithStorageOptions(
				storage.WithPrunableManagerOptions(prunable.WithGranularity(1)),
				storage.WithPruningDelay(4),
			),
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(1),
			),
			protocol.WithAttestationProvider(
				slotattestation.NewProvider(3),
			),
		)
		ts.Wait()

		latestCommitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(10)).Commitment()
		// Verify node3 state:
		// - Commitment at slot 10 should be the latest commitment.
		// - 10-3 (RootBlocksEvictionDelay) = 7 -> rootblocks from slot 8 until 10 (count of 3).
		// - ChainID is defined by the earliest commitment of the rootblocks -> block 8.2 commits to slot 1.
		// - slot 3 is finalized as per snapshot.
		ts.AssertNodeState(ts.Nodes("node3"),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(10),
			testsuite.WithLatestCommitment(latestCommitment),
			testsuite.WithLatestFinalizedSlot(7),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(10, expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee([]account.SeatIndex{0, 1}),
			testsuite.WithEvictedSlot(10),
			testsuite.WithActiveRootBlocks(ts.Blocks("8.2", "9.2", "10.2")),
			testsuite.WithStorageRootBlocks(ts.Blocks("5.1", "6.2", "7.1", "8.2", "9.2", "10.2")),
			testsuite.WithPrunedSlot(3, true), // latestFinalizedSlot - PruningDelay
			testsuite.WithChainManagerIsSolid(),
		)
		require.Nil(t, node3.Protocol.MainEngineInstance().Storage.RootBlocks(3))
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node3.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())

		// Verify attestations state.
		ts.AssertLatestCommitmentCumulativeWeight(6, ts.Nodes()...)
	}

	{
		// Slot 15
		{
			node21 := ts.Node("node2.1")
			node3 := ts.Node("node3")

			slot9Commitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(9)).Commitment()
			ts.IssueBlockAtSlot("15.1", 15, slot9Commitment, node1, ts.BlockID("14.2"))
			ts.IssueBlockAtSlot("16.2", 16, slot9Commitment, node21, ts.BlockID("15.1"))

			ts.AssertBlocksInCachePreAccepted(ts.Blocks("14.2", "15.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCachePreConfirmed(ts.Blocks("14.2", "15.1"), true, ts.Nodes()...)

			ts.AssertBlocksInCachePreAccepted(ts.Blocks("16.2"), false, ts.Nodes()...)
			ts.AssertBlocksInCachePreConfirmed(ts.Blocks("16.2"), false, ts.Nodes()...)

			ts.AssertBlocksInCacheAccepted(ts.Blocks("13.1", "13.2", "13.1.1", "13.2.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("13.1", "13.2"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("13.1.1", "13.2.1"), false, ts.Nodes()...) // too old. confirmation ratification threshold = 2

			ts.AssertNodeState(ts.Nodes(),
				testsuite.WithSnapshotImported(true),
				testsuite.WithProtocolParameters(ts.ProtocolParameters),
				testsuite.WithLatestCommitmentSlotIndex(11),
				testsuite.WithLatestFinalizedSlot(7),
				testsuite.WithSybilProtectionCommittee(16, expectedCommittee),
				testsuite.WithSybilProtectionOnlineCommittee([]account.SeatIndex{0, 1}),
				testsuite.WithEvictedSlot(11),
				testsuite.WithActiveRootBlocks(ts.Blocks("9.2", "10.2", "11.1")),
				testsuite.WithChainManagerIsSolid(),
			)
			require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node21.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
			require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node3.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())

			ts.AssertLatestCommitmentCumulativeWeight(6, ts.Nodes()...)
			// TODO: for node3: we are not actually exporting already created attestations. Do we need to?
			ts.AssertAttestationsForSlot(10, ts.Blocks("9.1", "10.2"), ts.Nodes("node1", "node2.1")...)
			ts.AssertAttestationsForSlot(11, ts.Blocks(), ts.Nodes()...)
		}
	}
}

func TestProtocol_StartNodeFromSnapshotAndDiskWithEmptySlot(t *testing.T) {
	ts := testsuite.NewTestSuite(t, testsuite.WithGenesisTimestampOffset(100*10))
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1", 50)
	node2 := ts.AddValidatorNode("node2", 50)

	ts.Run(map[string][]options.Option[protocol.Protocol]{
		"node1": {
			protocol.WithStorageOptions(
				storage.WithPrunableManagerOptions(prunable.WithGranularity(1)),
				storage.WithPruningDelay(3),
			),
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(1),
			),
			protocol.WithAttestationProvider(
				slotattestation.NewProvider(3),
			),
		},
		"node2": {
			protocol.WithStorageOptions(
				storage.WithPrunableManagerOptions(prunable.WithGranularity(1)),
				storage.WithPruningDelay(4),
			),
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(1),
			),
			protocol.WithAttestationProvider(
				slotattestation.NewProvider(3),
			),
		},
	})
	//ts.HookLogging()

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
		testsuite.WithLatestFinalizedSlot(0),
		testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
		testsuite.WithStorageCommitments([]*iotago.Commitment{iotago.NewEmptyCommitment()}),
		testsuite.WithSybilProtectionCommittee(expectedCommittee),
		testsuite.WithSybilProtectionOnlineCommittee(expectedCommittee),
		testsuite.WithEvictedSlot(0),
		testsuite.WithActiveRootBlocks(ts.Blocks("Genesis")),
		testsuite.WithStorageRootBlocks(ts.Blocks("Genesis")),
	)

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
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("1.1", "1.2", "1.1*"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("2.2", "2.2*"), false, ts.Nodes()...)
	}

	// Slot 3-6
	{
		// Slot 3
		ts.IssueBlockAtSlot("3.1", 3, iotago.NewEmptyCommitment(), node1, ts.BlockIDs("2.2", "2.2*")...)

		ts.AssertBlocksExist(ts.Blocks("3.1"), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("2.2", "2.2*"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("2.2", "2.2*"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("3.1"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("3.1"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("1.2"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("1.2"), true, ts.Nodes()...)

		// Slot 4
		ts.IssueBlockAtSlot("4.2", 4, iotago.NewEmptyCommitment(), node2, ts.BlockID("3.1"))

		ts.AssertBlocksExist(ts.Blocks("4.2"), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("3.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("3.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("4.2"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("4.2"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("1.1", "1.1*"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("1.1", "1.1*"), false, ts.Nodes()...) // too old. confirmation ratification threshold = 2

		// Slot 5
		ts.IssueBlockAtSlot("5.1", 5, iotago.NewEmptyCommitment(), node1, ts.BlockID("4.2"))

		ts.AssertBlocksExist(ts.Blocks("5.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("4.2"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("4.2"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("5.1"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("5.1"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("2.2", "2.2*"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("2.2", "2.2*"), false, ts.Nodes()...) // too old. confirmation ratification threshold = 2

		// Slot 6
		ts.IssueBlockAtSlot("6.2", 6, iotago.NewEmptyCommitment(), node2, ts.BlockID("5.1"))

		ts.AssertBlocksExist(ts.Blocks("6.2"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("5.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("5.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("6.2"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("6.2"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("3.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("3.1"), false, ts.Nodes()...) // too old. confirmation ratification threshold = 2

		// Verify nodes' states: Slot 1 should be committed as the MinCommittableSlotAge is 1, and we accepted a block at slot 3.
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(1),
			testsuite.WithEqualStoredCommitmentAtIndex(1),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedCommittee),
			testsuite.WithEvictedSlot(1),
			testsuite.WithActiveRootBlocks(ts.Blocks("Genesis", "1.1", "1.1*")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis", "1.1", "1.1*", "2.2", "2.2*")),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
	}

	// Slot 7 does not contain any blocks
	// Slot 8-9
	{
		slot1Commitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(1)).Commitment()

		// Slot 8
		ts.IssueBlockAtSlot("8.1", 8, slot1Commitment, node1, ts.BlockID("6.2"))
		// Slot 9
		ts.IssueBlockAtSlot("9.2", 9, slot1Commitment, node2, ts.BlockID("8.1"))

		ts.AssertBlocksExist(ts.Blocks("8.1", "9.2"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("6.2", "8.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("6.2", "8.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("9.2"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("9.2"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("4.2", "5.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("4.2", "5.1"), false, ts.Nodes()...) // too old. confirmation ratification threshold = 2

		// Verify nodes' states:
		// - Slot 3 should be committed as the MinCommittableSlotAge is 1, and we accepted a block at slot 5.
		// - 5.1 is accepted and commits to slot 1 -> slot 1 should be evicted.
		// - rootblocks are still not evicted as RootBlocksEvictionDelay is 3.
		// - slot 1 is still not finalized: there is no supermajority of confirmed blocks that commits to it.
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(3),
			testsuite.WithEqualStoredCommitmentAtIndex(3),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedCommittee),
			testsuite.WithEvictedSlot(3),
			testsuite.WithActiveRootBlocks(ts.Blocks("1.1", "1.1*", "2.2", "2.2*", "3.1")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis", "1.1", "1.1*", "2.2", "2.2*", "3.1", "4.2")),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())

		ts.AssertLatestCommitmentCumulativeWeight(100, ts.Nodes()...)
		ts.AssertAttestationsForSlot(3, ts.Blocks("3.1", "2.2*"), ts.Nodes()...)
	}

	// Make slot 7 and 8 committed.
	{
		slot3Commitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(3)).Commitment()
		ts.IssueBlockAtSlot("10.1.2", 10, slot3Commitment, node1, ts.BlockID("9.2"))
		ts.IssueBlockAtSlot("10.2.2", 10, slot3Commitment, node2, ts.BlockID("10.1.2"))
		ts.IssueBlockAtSlot("10.1.3", 10, slot3Commitment, node1, ts.BlockID("10.2.2"))
		ts.IssueBlockAtSlot("10.2.3", 10, slot3Commitment, node2, ts.BlockID("10.1.3"))

		ts.AssertBlocksExist(ts.Blocks("10.1.2", "10.2.2", "10.1.3", "10.2.3"), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("10.1.2", "10.2.2", "10.1.3"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("10.1.2", "10.2.2", "10.1.3"), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("10.2.3"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("10.2.3"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("9.2", "10.1.2"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("9.2", "10.1.2"), true, ts.Nodes()...)

		// Verify nodes' states:
		// - Slot 3 should be committed as the MinCommittableSlotAge is 1, and we accepted a block at slot 5.
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(8),
			testsuite.WithEqualStoredCommitmentAtIndex(8),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedCommittee),
			testsuite.WithEvictedSlot(8),
			testsuite.WithActiveRootBlocks(ts.Blocks("6.2", "8.1")),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
	}

	// Slot 10-13
	{
		slot7Commitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(8)).Commitment()

		// Slot 10
		ts.IssueBlockAtSlot("10.1", 10, slot7Commitment, node1, ts.BlockID("10.2.3"))
		ts.IssueBlockAtSlot("10.2", 10, slot7Commitment, node2, ts.BlockID("10.1"))
		// Slot 11
		ts.IssueBlockAtSlot("11.2", 11, slot7Commitment, node2, ts.BlockID("10.2"))
		// Slot 12
		ts.IssueBlockAtSlot("12.1", 12, slot7Commitment, node1, ts.BlockID("11.2"))
		// Slot 13
		ts.IssueBlockAtSlot("13.2", 13, slot7Commitment, node2, ts.BlockID("12.1"))
		ts.IssueBlockAtSlot("13.1", 13, slot7Commitment, node1, ts.BlockID("12.1"))

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("11.2", "12.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("11.2", "12.1"), true, ts.Nodes()...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("13.2", "13.1"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("13.2", "13.1"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("10.1.2", "10.2.2", "10.1.3", "10.2.3", "10.1", "10.2"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("10.1.2", "10.2.2", "10.1.3", "10.2.3"), true, ts.Nodes()...) // too old. confirmation ratification threshold = 2

		// Verify nodes' states:
		// - Slot 8 should be committed as the MinCommittableSlotAge is 1, and we accepted a block at slot 10.
		// - rootblocks are evicted until slot 5 as RootBlocksEvictionDelay is 3.
		// - slot 3 is finalized as "10.1.2", "10.2.2", "10.1.3", "10.2.3" are confirmed within slot 9 being a supermajority.
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(8),
			testsuite.WithEqualStoredCommitmentAtIndex(8),
			testsuite.WithLatestFinalizedSlot(3),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedCommittee),
			testsuite.WithEvictedSlot(8),
			testsuite.WithActiveRootBlocks(ts.Blocks("6.2", "8.1")),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())

		ts.AssertNodeState(ts.Nodes("node1"),
			testsuite.WithStorageRootBlocks(ts.Blocks("1.1", "1.1*", "2.2", "2.2*", "3.1", "4.2", "5.1", "6.2", "8.1")),
			testsuite.WithPrunedSlot(0, true),
		)

		ts.AssertNodeState(ts.Nodes("node2"),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis", "1.1", "1.1*", "2.2", "2.2*", "3.1", "4.2", "5.1", "6.2", "8.1")),
			testsuite.WithPrunedSlot(0, false),
		)
	}

	// Slot 14
	{
		slot8Commitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(8)).Commitment()
		ts.IssueBlockAtSlot("14.1", 14, slot8Commitment, node1, ts.BlockIDs("13.2", "13.1")...)
		ts.IssueBlockAtSlot("14.2", 14, slot8Commitment, node2, ts.BlockIDs("13.2", "13.1")...)
		ts.IssueBlockAtSlot("14.1.1", 14, slot8Commitment, node1, ts.BlockIDs("14.1", "14.2")...)
		ts.IssueBlockAtSlot("14.2.1", 14, slot8Commitment, node2, ts.BlockIDs("14.1", "14.2")...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("13.2", "13.1", "14.2", "14.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("13.2", "13.1", "14.2", "14.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("14.1.1", "14.2.1"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("14.1.1", "14.2.1"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("12.1", "13.2", "13.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("12.1", "13.2", "13.1"), true, ts.Nodes()...)

		// Verify nodes' states:
		// - Slot 10 should be committed as the MinCommittableSlotAge is 1, and we accepted a block at slot 12.
		// - rootblocks are evicted until slot 8 as RootBlocksEvictionDelay is 3.
		// - slot 7 is finalized: there is a supermajority of confirmed blocks that commits to it.
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(11),
			testsuite.WithEqualStoredCommitmentAtIndex(11),
			testsuite.WithLatestFinalizedSlot(8),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedCommittee),
			testsuite.WithEvictedSlot(11),
			testsuite.WithActiveRootBlocks(ts.Blocks("9.2", "10.2", "11.2")),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())

		ts.AssertNodeState(ts.Nodes("node1"),
			testsuite.WithStorageRootBlocks(ts.Blocks("6.2", "8.1", "9.2", "10.2")),
			testsuite.WithPrunedSlot(5, true),
		)

		ts.AssertNodeState(ts.Nodes("node2"),
			testsuite.WithStorageRootBlocks(ts.Blocks("5.1", "6.2", "8.1", "9.2", "10.2")),
			testsuite.WithPrunedSlot(4, true),
		)
	}

	// Slot 15
	{
		slot9Commitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(9)).Commitment()
		ts.IssueBlockAtSlot("15.2", 15, slot9Commitment, node2, ts.BlockIDs("14.1.1", "14.2.1")...)

		ts.AssertBlocksInCachePreAccepted(ts.Blocks("14.1.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("14.1.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCachePreAccepted(ts.Blocks("14.2.1", "15.2"), false, ts.Nodes()...)
		ts.AssertBlocksInCachePreConfirmed(ts.Blocks("14.2.1", "15.2"), false, ts.Nodes()...)

		ts.AssertBlocksInCacheAccepted(ts.Blocks("13.2", "13.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheConfirmed(ts.Blocks("13.2", "13.1"), true, ts.Nodes()...)

		// Verify nodes' states:
		// - Slot 11 should be committed as the MinCommittableSlotAge is 1, and we accepted a block at slot 13.
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(11),
			testsuite.WithEqualStoredCommitmentAtIndex(11),
			testsuite.WithLatestFinalizedSlot(8),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedCommittee),
			testsuite.WithEvictedSlot(11),
			testsuite.WithActiveRootBlocks(ts.Blocks("9.2", "10.2", "11.2")),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())

		// We have committed to slot 10 where we referenced slot 8 with commitments -> there should be cumulative weight and attestations for slot 9.
		ts.AssertLatestCommitmentCumulativeWeight(300, ts.Nodes()...)
		ts.AssertAttestationsForSlot(10, ts.Blocks("10.1", "10.2"), ts.Nodes()...)
		ts.AssertAttestationsForSlot(11, ts.Blocks("10.1", "11.2"), ts.Nodes()...)

		ts.AssertNodeState(ts.Nodes("node1"),
			testsuite.WithStorageRootBlocks(ts.Blocks("6.2", "8.1", "9.2", "10.2")),
			testsuite.WithPrunedSlot(5, true),
		)

		ts.AssertNodeState(ts.Nodes("node2"),
			testsuite.WithStorageRootBlocks(ts.Blocks("5.1", "6.2", "8.1", "9.2", "10.2")),
			testsuite.WithPrunedSlot(4, true),
		)
	}

	// Shutdown node2 and restart it from disk. Verify state.
	{
		node2.Shutdown()
		ts.RemoveNode("node2")

		node21 := ts.AddNode("node2.1")
		node21.CopyIdentityFromNode(node2)
		node21.Initialize(
			protocol.WithBaseDirectory(ts.Directory.Path(node2.Name)),
			protocol.WithSybilProtectionProvider(
				poa.NewProvider(ts.Validators()),
			),
			protocol.WithStorageOptions(
				storage.WithPrunableManagerOptions(prunable.WithGranularity(1)),
				storage.WithPruningDelay(4),
			),
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(1),
			),
			protocol.WithAttestationProvider(
				slotattestation.NewProvider(3),
			),
		)
		ts.Wait()

		ts.AssertNodeState(ts.Nodes("node2.1"),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(11),
			testsuite.WithEqualStoredCommitmentAtIndex(11),
			testsuite.WithLatestFinalizedSlot(8),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedCommittee),
			testsuite.WithEvictedSlot(11),
			testsuite.WithActiveRootBlocks(ts.Blocks("9.2", "10.2", "11.2")),
			testsuite.WithStorageRootBlocks(ts.Blocks("5.1", "6.2", "8.1", "9.2", "10.2")),
			testsuite.WithPrunedSlot(4, true),
			testsuite.WithChainManagerIsSolid(),
		)
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node21.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())

		// Verify attestations state.
		ts.AssertLatestCommitmentCumulativeWeight(300, ts.Nodes()...)
		ts.AssertAttestationsForSlot(10, ts.Blocks("10.1", "10.2"), ts.Nodes()...)
		ts.AssertAttestationsForSlot(11, ts.Blocks("10.1", "11.2"), ts.Nodes()...)
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
			protocol.WithStorageOptions(
				storage.WithPrunableManagerOptions(prunable.WithGranularity(1)),
				storage.WithPruningDelay(4),
			),
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(1),
			),
			protocol.WithAttestationProvider(
				slotattestation.NewProvider(3),
			),
		)
		ts.Wait()

		latestCommitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(11)).Commitment()
		// Verify node3 state:
		// - Commitment at slot 11 should be the latest commitment.
		// - 11-3 (RootBlocksEvictionDelay) = 8 -> rootblocks from slot 9 until 11 (count of 2).
		// - ChainID is defined by the earliest commitment of the rootblocks -> block 9.2 commits to slot 1.
		// - slot 4 is finalized as per snapshot.
		ts.AssertNodeState(ts.Nodes("node3"),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.ProtocolParameters),
			testsuite.WithLatestCommitmentSlotIndex(11),
			testsuite.WithLatestCommitment(latestCommitment),
			testsuite.WithLatestFinalizedSlot(8),
			testsuite.WithChainID(iotago.NewEmptyCommitment().MustID()),
			testsuite.WithSybilProtectionCommittee(expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedCommittee),
			testsuite.WithEvictedSlot(11),
			testsuite.WithActiveRootBlocks(ts.Blocks("9.2", "10.2", "11.2")),
			testsuite.WithStorageRootBlocks(ts.Blocks("6.2", "8.1", "9.2", "10.2", "11.2")),
			testsuite.WithPrunedSlot(4, true), // latestFinalizedSlot - PruningDelay
			testsuite.WithChainManagerIsSolid(),
		)
		require.Nil(t, node3.Protocol.MainEngineInstance().Storage.RootBlocks(2))
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node3.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())

		// Verify attestations state.
		ts.AssertLatestCommitmentCumulativeWeight(300, ts.Nodes()...)
	}

	{
		// Slot 16-17
		{
			node21 := ts.Node("node2.1")
			node3 := ts.Node("node3")

			slot10Commitment := lo.PanicOnErr(node1.Protocol.MainEngineInstance().Storage.Commitments().Load(10)).Commitment()
			ts.IssueBlockAtSlot("16.1", 16, slot10Commitment, node1, ts.BlockID("15.2"))
			ts.IssueBlockAtSlot("17.2", 17, slot10Commitment, node21, ts.BlockID("16.1"))

			ts.AssertBlocksInCachePreAccepted(ts.Blocks("15.2", "16.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCachePreConfirmed(ts.Blocks("15.2", "16.1"), true, ts.Nodes()...)

			ts.AssertBlocksInCachePreAccepted(ts.Blocks("17.2"), false, ts.Nodes()...)
			ts.AssertBlocksInCachePreConfirmed(ts.Blocks("17.2"), false, ts.Nodes()...)

			ts.AssertBlocksInCacheAccepted(ts.Blocks("14.1", "14.2", "14.1.1", "14.2.1"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("14.1", "14.2"), true, ts.Nodes()...)
			ts.AssertBlocksInCacheConfirmed(ts.Blocks("14.1.1", "14.2.1"), false, ts.Nodes()...) // too old. confirmation ratification threshold = 2

			ts.AssertNodeState(ts.Nodes(),
				testsuite.WithSnapshotImported(true),
				testsuite.WithProtocolParameters(ts.ProtocolParameters),
				testsuite.WithLatestCommitmentSlotIndex(12),
				testsuite.WithLatestFinalizedSlot(8),
				testsuite.WithSybilProtectionCommittee(expectedCommittee),
				testsuite.WithSybilProtectionOnlineCommittee(expectedCommittee),
				testsuite.WithEvictedSlot(12),
				testsuite.WithActiveRootBlocks(ts.Blocks("10.2", "11.2", "12.1")),
				testsuite.WithChainManagerIsSolid(),
			)
			require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node21.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
			require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node3.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())

			ts.AssertLatestCommitmentCumulativeWeight(300, ts.Nodes()...)
			// TODO: for node3: we are not actually exporting already created attestations. Do we need to?
			ts.AssertAttestationsForSlot(11, ts.Blocks("10.1", "11.2"), ts.Nodes("node1", "node2.1")...)
			ts.AssertAttestationsForSlot(12, ts.Blocks(), ts.Nodes()...)
		}
	}
}
