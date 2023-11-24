package tests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/syncmanager/trivialsyncmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager"
	mock2 "github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager/mock"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1"
	"github.com/iotaledger/iota-core/pkg/storage"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestProtocol_EngineSwitching(t *testing.T) {
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

		testsuite.WithWaitFor(15*time.Second),
	)
	defer ts.Shutdown()

	node0 := ts.AddValidatorNode("node0")
	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddValidatorNode("node2")
	node3 := ts.AddValidatorNode("node3")
	node4 := ts.AddValidatorNode("node4")
	node5 := ts.AddNode("node5")
	node6 := ts.AddValidatorNode("node6")
	node7 := ts.AddValidatorNode("node7")
	node8 := ts.AddNode("node8")
	ts.AddDefaultWallet(node0)

	const expectedCommittedSlotAfterPartitionMerge = 19
	nodesP1 := []*mock.Node{node0, node1, node2, node3, node4, node5}
	nodesP2 := []*mock.Node{node6, node7, node8}

	poaProvider := func() module.Provider[*engine.Engine, seatmanager.SeatManager] {
		return module.Provide(func(e *engine.Engine) seatmanager.SeatManager {
			poa := mock2.NewManualPOAProvider()(e).(*mock2.ManualPOA)

			for _, node := range append(nodesP1, nodesP2...) {
				if node.IsValidator() {
					poa.AddAccount(node.Validator.AccountID, node.Name)
				}
			}
			poa.SetOnline("node0", "node1", "node2", "node3", "node4")

			return poa
		})
	}

	nodeOptions := make(map[string][]options.Option[protocol.Protocol])
	for _, node := range ts.Nodes() {
		nodeOptions[node.Name] = []options.Option[protocol.Protocol]{
			protocol.WithChainManagerOptions(
				chainmanager.WithCommitmentRequesterOptions(
					eventticker.RetryInterval[iotago.SlotIndex, iotago.CommitmentID](1*time.Second),
					eventticker.RetryJitter[iotago.SlotIndex, iotago.CommitmentID](500*time.Millisecond),
				),
			),
			protocol.WithSybilProtectionProvider(
				sybilprotectionv1.NewProvider(
					sybilprotectionv1.WithSeatManagerProvider(
						poaProvider(),
					),
				),
			),
			protocol.WithEngineOptions(
				engine.WithBlockRequesterOptions(
					eventticker.RetryInterval[iotago.SlotIndex, iotago.BlockID](1*time.Second),
					eventticker.RetryJitter[iotago.SlotIndex, iotago.BlockID](500*time.Millisecond),
				),
			),
			protocol.WithSyncManagerProvider(
				trivialsyncmanager.NewProvider(
					trivialsyncmanager.WithBootstrappedFunc(func(e *engine.Engine) bool {
						return e.Storage.Settings().LatestCommitment().Slot() >= expectedCommittedSlotAfterPartitionMerge && e.Notarization.IsBootstrapped()
					}),
				),
			),
			protocol.WithStorageOptions(
				storage.WithPruningDelay(20),
			),
		}
	}

	ts.Run(false, nodeOptions)

	expectedCommittee := []iotago.AccountID{
		node0.Validator.AccountID,
		node1.Validator.AccountID,
		node2.Validator.AccountID,
		node3.Validator.AccountID,
		node4.Validator.AccountID,
		node6.Validator.AccountID,
		node7.Validator.AccountID,
	}
	expectedP1OnlineCommittee := []account.SeatIndex{
		lo.Return1(lo.Return1(node0.Protocol.MainEngineInstance().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node0.Validator.AccountID)),
		lo.Return1(lo.Return1(node0.Protocol.MainEngineInstance().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node1.Validator.AccountID)),
		lo.Return1(lo.Return1(node0.Protocol.MainEngineInstance().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node2.Validator.AccountID)),
		lo.Return1(lo.Return1(node0.Protocol.MainEngineInstance().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node3.Validator.AccountID)),
		lo.Return1(lo.Return1(node0.Protocol.MainEngineInstance().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node4.Validator.AccountID)),
	}
	expectedP2OnlineCommittee := []account.SeatIndex{
		lo.Return1(lo.Return1(node0.Protocol.MainEngineInstance().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node6.Validator.AccountID)),
		lo.Return1(lo.Return1(node0.Protocol.MainEngineInstance().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node7.Validator.AccountID)),
	}
	expectedOnlineCommittee := append(expectedP1OnlineCommittee, expectedP2OnlineCommittee...)

	for _, node := range ts.Nodes() {
		node.Protocol.MainEngineInstance().SybilProtection.SeatManager().(*mock2.ManualPOA).SetOnline("node0", "node1", "node2", "node3", "node4", "node6", "node7")
	}

	// Verify that nodes have the expected states.

	{
		genesisCommitment := iotago.NewEmptyCommitment(ts.API)
		genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitment(genesisCommitment),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithChainID(genesisCommitment.MustID()),
			testsuite.WithStorageCommitments([]*iotago.Commitment{genesisCommitment}),

			testsuite.WithSybilProtectionCommittee(0, expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommittee...),
			testsuite.WithEvictedSlot(0),
			testsuite.WithActiveRootBlocks(ts.Blocks("Genesis")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis")),
		)
	}

	// Issue up to slot 13 in P0 (main partition with all nodes) and verify that the nodes have the expected states.
	{
		ts.IssueBlocksAtSlots("P0:", []iotago.SlotIndex{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}, 4, "Genesis", ts.Nodes(), true, nil)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(10),
			testsuite.WithLatestCommitmentSlotIndex(11),
			testsuite.WithEqualStoredCommitmentAtIndex(11),
			testsuite.WithLatestCommitmentCumulativeWeight(56), // 7 for each slot starting from 4
			testsuite.WithSybilProtectionCommittee(ts.API.TimeProvider().EpochFromSlot(11), expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommittee...),
			testsuite.WithEvictedSlot(11),
		)

		for _, slot := range []iotago.SlotIndex{4, 5, 6, 7, 8, 9, 10, 11} {
			var attestationBlocks []*blocks.Block
			for _, node := range ts.Nodes() {
				if node.IsValidator() {
					attestationBlocks = append(attestationBlocks, ts.Block(fmt.Sprintf("P0:%d.3-%s", slot, node.Name)))
				}
			}
			ts.AssertAttestationsForSlot(slot, attestationBlocks, ts.Nodes()...)
		}

		// Make sure the tips are properly set.
		var tipBlocks []*blocks.Block
		for _, node := range ts.Nodes() {
			tipBlocks = append(tipBlocks, ts.Block(fmt.Sprintf("P0:13.3-%s", node.Name)))
		}
		ts.AssertStrongTips(tipBlocks, ts.Nodes()...)

		ts.AssertBlocksExist(ts.BlocksWithPrefix("P0"), true, ts.Nodes()...)
	}

	// Split into partitions P1 and P2.
	ts.SplitIntoPartitions(map[string][]*mock.Node{
		"P1": nodesP1,
		"P2": nodesP2,
	})

	// Set online committee for each partition.
	for _, node := range ts.Nodes() {
		manualPOA := node.Protocol.MainEngineInstance().SybilProtection.SeatManager().(*mock2.ManualPOA)
		if node.Partition == "P1" {
			manualPOA.SetOnline("node0", "node1", "node2", "node3", "node4")
			manualPOA.SetOffline("node6", "node7")
		} else {
			manualPOA.SetOnline("node6", "node7")
			manualPOA.SetOffline("node0", "node1", "node2", "node3", "node4")
		}
	}

	ts.AssertSybilProtectionOnlineCommittee(expectedP1OnlineCommittee, nodesP1...)
	ts.AssertSybilProtectionOnlineCommittee(expectedP2OnlineCommittee, nodesP2...)

	// Issue blocks in partition 1.
	{
		ts.IssueBlocksAtSlots("P1:", []iotago.SlotIndex{14, 15, 16, 17, 18, 19, 20}, 4, "P0:13.3", nodesP1[:len(nodesP1)-1], true, nil)

		ts.AssertNodeState(nodesP1,
			testsuite.WithLatestFinalizedSlot(17),
			testsuite.WithLatestCommitmentSlotIndex(18),
			testsuite.WithEqualStoredCommitmentAtIndex(18),
			testsuite.WithLatestCommitmentCumulativeWeight(99), // 56 + slot 12-15=7 + 5 for each slot starting from 16
			testsuite.WithSybilProtectionCommittee(ts.API.TimeProvider().EpochFromSlot(18), expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedP1OnlineCommittee...),
			testsuite.WithEvictedSlot(18),
		)

		for _, slot := range []iotago.SlotIndex{12, 13, 14, 15} {
			var attestationBlocks []*blocks.Block
			for _, node := range nodesP1 {
				if node.IsValidator() {
					if slot <= 13 {
						attestationBlocks = append(attestationBlocks, ts.Block(fmt.Sprintf("P0:%d.3-%s", slot, node.Name)))
					} else {
						attestationBlocks = append(attestationBlocks, ts.Block(fmt.Sprintf("P1:%d.3-%s", slot, node.Name)))
					}
				}
			}

			// We carry these attestations forward with the window even though these nodes didn't issue in P1.
			for _, node := range nodesP2 {
				if node.IsValidator() {
					attestationBlocks = append(attestationBlocks, ts.Block(fmt.Sprintf("P0:%d.3-%s", lo.Min(slot, 13), node.Name)))
				}
			}

			ts.AssertAttestationsForSlot(slot, attestationBlocks, nodesP1...)
		}

		for _, slot := range []iotago.SlotIndex{16, 17, 18} {
			var attestationBlocks []*blocks.Block
			for _, node := range nodesP1 {
				if node.IsValidator() {
					attestationBlocks = append(attestationBlocks, ts.Block(fmt.Sprintf("P1:%d.3-%s", slot, node.Name)))
				}
			}
			ts.AssertAttestationsForSlot(slot, attestationBlocks, nodesP1...)
		}

		// Make sure the tips are properly set.
		var tipBlocks []*blocks.Block
		for _, node := range nodesP1[:len(nodesP1)-1] {
			tipBlocks = append(tipBlocks, ts.Block(fmt.Sprintf("P1:20.3-%s", node.Name)))
		}
		ts.AssertStrongTips(tipBlocks, nodesP1...)
	}

	// Issue blocks in partition 2.
	{
		ts.IssueBlocksAtSlots("P2:", []iotago.SlotIndex{14, 15, 16, 17, 18, 19, 20}, 4, "P0:13.3", nodesP2[:len(nodesP2)-1], true, nil)

		ts.AssertNodeState(nodesP2,
			testsuite.WithLatestFinalizedSlot(10),
			testsuite.WithLatestCommitmentSlotIndex(18),
			testsuite.WithEqualStoredCommitmentAtIndex(18),
			testsuite.WithLatestCommitmentCumulativeWeight(90), // 56 + slot 12-15=7 + 2 for each slot starting from 16
			testsuite.WithSybilProtectionCommittee(ts.API.TimeProvider().EpochFromSlot(18), expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedP2OnlineCommittee...),
			testsuite.WithEvictedSlot(18),
		)

		for _, slot := range []iotago.SlotIndex{12, 13, 14, 15} {
			var attestationBlocks []*blocks.Block
			for _, node := range nodesP2 {
				if node.IsValidator() {
					if slot <= 13 {
						attestationBlocks = append(attestationBlocks, ts.Block(fmt.Sprintf("P0:%d.3-%s", slot, node.Name)))
					} else {
						attestationBlocks = append(attestationBlocks, ts.Block(fmt.Sprintf("P2:%d.3-%s", slot, node.Name)))
					}
				}
			}

			// We carry these attestations forward with the window even though these nodes didn't issue in P1.
			for _, node := range nodesP1 {
				if node.IsValidator() {
					attestationBlocks = append(attestationBlocks, ts.Block(fmt.Sprintf("P0:%d.3-%s", lo.Min(slot, 13), node.Name)))
				}
			}

			ts.AssertAttestationsForSlot(slot, attestationBlocks, nodesP2...)
		}

		for _, slot := range []iotago.SlotIndex{16, 17, 18} {
			var attestationBlocks []*blocks.Block
			for _, node := range nodesP2 {
				if node.IsValidator() {
					attestationBlocks = append(attestationBlocks, ts.Block(fmt.Sprintf("P2:%d.3-%s", slot, node.Name)))
				}
			}
			ts.AssertAttestationsForSlot(slot, attestationBlocks, nodesP2...)
		}

		// Make sure the tips are properly set.
		var tipBlocks []*blocks.Block
		for _, node := range nodesP2[:len(nodesP2)-1] {
			tipBlocks = append(tipBlocks, ts.Block(fmt.Sprintf("P2:20.3-%s", node.Name)))
		}
		ts.AssertStrongTips(tipBlocks, nodesP2...)
	}

	for _, node := range ts.Nodes() {
		manualPOA := node.Protocol.MainEngineInstance().SybilProtection.SeatManager().(*mock2.ManualPOA)
		manualPOA.SetOnline("node0", "node1", "node2", "node3", "node4", "node6", "node7")
	}
	// Merge the partitions
	{
		ts.MergePartitionsToMain()
		fmt.Println("\n=========================\nMerged network partitions\n=========================")

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		ctxP1, ctxP1Cancel := context.WithCancel(ctx)
		ctxP2, ctxP2Cancel := context.WithCancel(ctx)

		wg := &sync.WaitGroup{}

		// Issue blocks on both partitions after merging the networks.
		node0.Validator.IssueActivity(ctxP1, wg, 21, node0)
		node1.Validator.IssueActivity(ctxP1, wg, 21, node1)
		node2.Validator.IssueActivity(ctxP1, wg, 21, node2)
		node3.Validator.IssueActivity(ctxP1, wg, 21, node3)
		node4.Validator.IssueActivity(ctxP1, wg, 21, node4)
		// node5.Validator.IssueActivity(ctxP1, wg, 21, node5)

		node6.Validator.IssueActivity(ctxP2, wg, 21, node6)
		node7.Validator.IssueActivity(ctxP2, wg, 21, node7)
		// node8.Validator.IssueActivity(ctxP2, wg, 21, node8)

		// P1 finalized until slot 18. We do not expect any forks here because our CW is higher than the other partition's.
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

	// Make sure that nodes that switched their engine still have blocks with prefix P0 from before the fork.
	// Those nodes should also have all the blocks from the target fork P1 and should not have blocks from P2.
	// This is to make sure that the storage was copied correctly during engine switching.
	ts.AssertBlocksExist(ts.BlocksWithPrefix("P0"), true, ts.Nodes()...)
	ts.AssertBlocksExist(ts.BlocksWithPrefix("P1"), true, ts.Nodes()...)
	ts.AssertBlocksExist(ts.BlocksWithPrefix("P2"), false, ts.Nodes()...)

	ts.AssertEqualStoredCommitmentAtIndex(expectedCommittedSlotAfterPartitionMerge, ts.Nodes()...)
}
