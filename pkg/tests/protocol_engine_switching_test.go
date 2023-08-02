package tests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager/poa"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestProtocol_EngineSwitching(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithLivenessThreshold(1),
		testsuite.WithMinCommittableAge(1),
		testsuite.WithMaxCommittableAge(4),
		testsuite.WithEpochNearingThreshold(2),
		testsuite.WithSlotsPerEpochExponent(3),
		testsuite.WithGenesisTimestampOffset(10*10),

		// testsuite.WithWaitFor(12*time.Second),
	)
	defer ts.Shutdown()

	node0 := ts.AddValidatorNodeToPartition("node0", "P1")
	node1 := ts.AddValidatorNodeToPartition("node1", "P1")
	node2 := ts.AddValidatorNodeToPartition("node2", "P1")
	node3 := ts.AddValidatorNodeToPartition("node3", "P1")
	node4 := ts.AddValidatorNodeToPartition("node4", "P1")
	node5 := ts.AddNodeToPartition("node5", "P1")

	node6 := ts.AddValidatorNodeToPartition("node6", "P2")
	node7 := ts.AddValidatorNodeToPartition("node7", "P2")
	node8 := ts.AddNodeToPartition("node8", "P2")

	nodesP1 := []*mock.Node{node0, node1, node2, node3, node4, node5}
	nodesP2 := []*mock.Node{node6, node7, node8}

	nodeOptions := []options.Option[protocol.Protocol]{
		protocol.WithChainManagerOptions(
			chainmanager.WithCommitmentRequesterOptions(
				eventticker.RetryInterval[iotago.SlotIndex, iotago.CommitmentID](1*time.Second),
				eventticker.RetryJitter[iotago.SlotIndex, iotago.CommitmentID](500*time.Millisecond),
			),
		),
	}

	nodeP1Options := append(nodeOptions,
		protocol.WithSybilProtectionProvider(
			sybilprotectionv1.NewProvider(
				sybilprotectionv1.WithSeatManagerProvider(
					poa.NewProvider(
						poa.WithOnlineCommitteeStartup(node0.AccountID, node1.AccountID, node2.AccountID, node3.AccountID, node4.AccountID),
						poa.WithActivityWindow(1*time.Minute),
					),
				),
			),
		),
	)

	nodeP2Options := append(nodeOptions,
		protocol.WithSybilProtectionProvider(
			sybilprotectionv1.NewProvider(
				sybilprotectionv1.WithSeatManagerProvider(
					poa.NewProvider(
						poa.WithOnlineCommitteeStartup(node6.AccountID, node7.AccountID),
						poa.WithActivityWindow(1*time.Minute),
					),
				),
			),
		),
	)

	ts.Run(map[string][]options.Option[protocol.Protocol]{
		"node0": nodeP1Options,
		"node1": nodeP1Options,
		"node2": nodeP1Options,
		"node3": nodeP1Options,
		"node4": nodeP1Options,
		"node5": nodeP1Options,
		"node6": nodeP2Options,
		"node7": nodeP2Options,
		"node8": nodeP2Options,
	})
	ts.HookLogging()

	expectedCommittee := []iotago.AccountID{
		node0.AccountID,
		node1.AccountID,
		node2.AccountID,
		node3.AccountID,
		node4.AccountID,
		node6.AccountID,
		node7.AccountID,
	}
	expectedP1Committee := []account.SeatIndex{
		lo.Return1(node0.Protocol.MainEngineInstance().SybilProtection.SeatManager().Committee(1).GetSeat(node0.AccountID)),
		lo.Return1(node0.Protocol.MainEngineInstance().SybilProtection.SeatManager().Committee(1).GetSeat(node1.AccountID)),
		lo.Return1(node0.Protocol.MainEngineInstance().SybilProtection.SeatManager().Committee(1).GetSeat(node2.AccountID)),
		lo.Return1(node0.Protocol.MainEngineInstance().SybilProtection.SeatManager().Committee(1).GetSeat(node3.AccountID)),
		lo.Return1(node0.Protocol.MainEngineInstance().SybilProtection.SeatManager().Committee(1).GetSeat(node4.AccountID)),
	}
	expectedP2Committee := []account.SeatIndex{
		lo.Return1(node6.Protocol.MainEngineInstance().SybilProtection.SeatManager().Committee(1).GetSeat(node6.AccountID)),
		lo.Return1(node6.Protocol.MainEngineInstance().SybilProtection.SeatManager().Committee(1).GetSeat(node7.AccountID)),
	}

	// Verify that nodes have the expected states.
	{
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitment(iotago.NewEmptyCommitment(ts.API.ProtocolParameters().Version())),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithChainID(iotago.NewEmptyCommitment(ts.API.ProtocolParameters().Version()).MustID()),
			testsuite.WithStorageCommitments([]*iotago.Commitment{iotago.NewEmptyCommitment(ts.API.ProtocolParameters().Version())}),

			testsuite.WithSybilProtectionCommittee(0, expectedCommittee),
			testsuite.WithEvictedSlot(0),
			testsuite.WithActiveRootBlocks(ts.Blocks("Genesis")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis")),
		)

		ts.AssertSybilProtectionOnlineCommittee(expectedP1Committee, nodesP1...)
		ts.AssertSybilProtectionOnlineCommittee(expectedP2Committee, nodesP2...)
	}

	// Issue blocks in partition 1.
	{
		ts.IssueBlocksAtSlots("P1:", []iotago.SlotIndex{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 4, "Genesis", nodesP1, true, nil)

		ts.AssertNodeState(nodesP1,
			testsuite.WithLatestFinalizedSlot(7),
			testsuite.WithLatestCommitmentSlotIndex(8),
			testsuite.WithEqualStoredCommitmentAtIndex(8),
			testsuite.WithLatestCommitmentCumulativeWeight(25), // 5 for each slot starting from 4
			testsuite.WithSybilProtectionCommittee(0, expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedP1Committee...),
			testsuite.WithEvictedSlot(8),
		)

		for _, slot := range []iotago.SlotIndex{4, 5, 6, 7, 8} {
			var attestationBlocks []*blocks.Block
			for _, node := range nodesP1 {
				if node.Validator {
					attestationBlocks = append(attestationBlocks, ts.Block(fmt.Sprintf("P1:%d.3-%s", slot, node.Name)))
				}
			}
			ts.AssertAttestationsForSlot(slot, attestationBlocks, nodesP1...)
		}

		// Make sure the tips are properly set.
		var tipBlocks []*blocks.Block
		for _, node := range nodesP1 {
			tipBlocks = append(tipBlocks, ts.Block(fmt.Sprintf("P1:10.3-%s", node.Name)))
		}
		ts.AssertStrongTips(tipBlocks, nodesP1...)
	}

	// Issue blocks in partition 2.
	{
		// TODO: we should issue with all nodes in this partition
		ts.IssueBlocksAtSlots("P2:", []iotago.SlotIndex{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 4, "Genesis", ts.Nodes("node6", "node7"), true, nil)

		ts.AssertNodeState(nodesP2,
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithLatestCommitmentSlotIndex(8),
			testsuite.WithEqualStoredCommitmentAtIndex(8),
			testsuite.WithLatestCommitmentCumulativeWeight(10), // 2 for each slot starting from 4
			testsuite.WithSybilProtectionCommittee(0, expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedP2Committee...),
			testsuite.WithEvictedSlot(8),
		)

		for _, slot := range []iotago.SlotIndex{4, 5, 6, 7, 8} {
			var attestationBlocks []*blocks.Block
			for _, node := range nodesP2 {
				if node.Validator {
					attestationBlocks = append(attestationBlocks, ts.Block(fmt.Sprintf("P2:%d.3-%s", slot, node.Name)))
				}
			}
			ts.AssertAttestationsForSlot(slot, attestationBlocks, nodesP2...)
		}

		// Make sure the tips are properly set.
		var tipBlocks []*blocks.Block
		for _, node := range nodesP2 {
			if node.Validator {
				tipBlocks = append(tipBlocks, ts.Block(fmt.Sprintf("P2:10.3-%s", node.Name)))
			}
		}
		ts.AssertStrongTips(tipBlocks, nodesP2...)
	}

	// Make sure that no blocks of partition 1 are known on partition 2 and vice versa.
	{
		ts.AssertBlocksExist(ts.BlocksWithPrefix("P1"), true, nodesP1...)
		ts.AssertBlocksExist(ts.BlocksWithPrefix("P1"), false, nodesP2...)

		ts.AssertBlocksExist(ts.BlocksWithPrefix("P2"), true, nodesP2...)
		ts.AssertBlocksExist(ts.BlocksWithPrefix("P2"), false, nodesP1...)
	}

	// Both partitions should have committed slot 8 with different commitments.
	{
		ts.AssertLatestCommitmentSlotIndex(8, ts.Nodes()...)

		ts.AssertEqualStoredCommitmentAtIndex(8, nodesP1...)
		ts.AssertEqualStoredCommitmentAtIndex(8, nodesP2...)

		require.NotEqual(t, node0.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node6.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
	}

	// 	// Merge the partitions
	{
		ts.Network.MergePartitionsToMain()
		fmt.Println("\n=========================\nMerged network partitions\n=========================")

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		ctxP1, ctxP1Cancel := context.WithCancel(ctx)
		ctxP2, ctxP2Cancel := context.WithCancel(ctx)

		wg := &sync.WaitGroup{}

		// Issue blocks on both partitions after merging the networks.
		node0.IssueActivity(ctxP1, wg, 11)
		node1.IssueActivity(ctxP1, wg, 11)
		node2.IssueActivity(ctxP1, wg, 11)
		node3.IssueActivity(ctxP1, wg, 11)
		node4.IssueActivity(ctxP1, wg, 11)
		node5.IssueActivity(ctxP1, wg, 11)

		node6.IssueActivity(ctxP2, wg, 11)
		node7.IssueActivity(ctxP2, wg, 11)
		node8.IssueActivity(ctxP2, wg, 11)

		// P1 finalized until slot 7. We do not expect any forks here because our CW is higher than the other partition's.
		ts.AssertForkDetectedCount(0, nodesP1...)
		// P1's chain is heavier, they should not consider switching the chain.
		ts.AssertCandidateEngineActivatedCount(0, nodesP1...)
		ctxP2Cancel() // we can stop issuing on P2.

		// Nodes from P2 should switch the chain.
		ts.AssertForkDetectedCount(1, nodesP2...)
		ts.AssertCandidateEngineActivatedCount(1, nodesP2...)

		// TODO: there's a bunch of issues when creating a snapshot from slot 0:
		//  1. Exporting attestation snapshot SlotIndex(0) SlotIndex(8) SlotIndex(18446744073709551612) SlotIndex(4)
		//  2. prunedSlot, hasPruned := e.Storage.LastPrunedSlot() fix
		//  3. EOF etc

		// Here we need to let enough time pass for the nodes to sync up the candidate engines and switch them
		ts.AssertMainEngineSwitchedCount(1, nodesP2...)

		ctxP1Cancel()
		wg.Wait()
	}

	ts.AssertEqualStoredCommitmentAtIndex(11, ts.Nodes()...)
}
