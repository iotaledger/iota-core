package tests

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/syncmanager/trivialsyncmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager"
	mock2 "github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager/mock"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager/topstakers"
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

	const expectedCommittedSlotAfterPartitionMerge = 18
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
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node0.Validator.AccountID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node1.Validator.AccountID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node2.Validator.AccountID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node3.Validator.AccountID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node4.Validator.AccountID)),
	}
	expectedP2OnlineCommittee := []account.SeatIndex{
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node6.Validator.AccountID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node7.Validator.AccountID)),
	}
	expectedOnlineCommittee := append(expectedP1OnlineCommittee, expectedP2OnlineCommittee...)

	for _, node := range ts.Nodes() {
		node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().(*mock2.ManualPOA).SetOnline("node0", "node1", "node2", "node3", "node4", "node6", "node7")
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
		ts.IssueBlocksAtSlots("P0:", []iotago.SlotIndex{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}, 4, "Genesis", ts.Nodes(), true, false)

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
		manualPOA := node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().(*mock2.ManualPOA)
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
		ts.IssueBlocksAtSlots("P1:", []iotago.SlotIndex{14, 15, 16, 17, 18, 19, 20}, 4, "P0:13.3", nodesP1[:len(nodesP1)-1], true, false)

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
		ts.IssueBlocksAtSlots("P2:", []iotago.SlotIndex{14, 15, 16, 17, 18, 19, 20}, 4, "P0:13.3", nodesP2[:len(nodesP2)-1], true, false)

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
		manualPOA := node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().(*mock2.ManualPOA)
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

func TestProtocol_EngineSwitching_CommitteeRotation(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithWaitFor(15*time.Second),

		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(1000, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				4, // 16 slots per epoch
			),
			iotago.WithLivenessOptions(
				10,
				10,
				3,
				5,
				10,
			),
		),
	)
	defer ts.Shutdown()

	node0 := ts.AddValidatorNode("node0")
	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddValidatorNode("node2")
	node3 := ts.AddValidatorNode("node3")

	const expectedCommittedSlotAfterPartitionMerge = 18
	nodesP1 := []*mock.Node{node0, node1, node2}
	nodesP2 := []*mock.Node{node3}

	nodeOpts := []options.Option[protocol.Protocol]{
		protocol.WithNotarizationProvider(
			slotnotarization.NewProvider(),
		),
		protocol.WithSybilProtectionProvider(
			sybilprotectionv1.NewProvider(
				sybilprotectionv1.WithSeatManagerProvider(
					topstakers.NewProvider(
						// We need to make sure that inactive nodes are evicted from the committee to continue acceptance.
						topstakers.WithActivityWindow(10 * time.Second),
					),
				),
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
			storage.WithPruningDelay(20), // make sure nodes don't prune
		),
	}

	ts.Run(false, map[string][]options.Option[protocol.Protocol]{
		"node0": nodeOpts,
		"node1": nodeOpts,
		"node2": nodeOpts,
		"node3": nodeOpts,
	})

	// Verify that nodes have the expected states after startup.
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

			testsuite.WithSybilProtectionCommittee(0, ts.AccountsOfNodes("node0", "node1", "node2", "node3")),
			testsuite.WithSybilProtectionOnlineCommittee(ts.SeatOfNodes(0, "node0", "node1", "node2", "node3")...),
			testsuite.WithEvictedSlot(0),
			testsuite.WithActiveRootBlocks(ts.Blocks("Genesis")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis")),
		)
	}

	// Issue up to slot 8 in P0 (main partition with all nodes) and verify that the nodes have the expected states.
	{
		ts.IssueBlocksAtSlots("P0:", []iotago.SlotIndex{1, 2}, 4, "Genesis", ts.Nodes(), true, true)

		// Register candidates (node1, node2) in P0 for epoch 1.
		{
			ts.IssueCandidacyAnnouncementInSlot("P0:node1-candidacy:1", 3, "P0:2.3", ts.Wallet("node1"))
			ts.IssueCandidacyAnnouncementInSlot("P0:node2-candidacy:1", 3, "P0:node1-candidacy:1", ts.Wallet("node2"))
		}

		ts.IssueBlocksAtSlots("P0:", []iotago.SlotIndex{3, 4, 5, 6, 7, 8}, 4, "P0:node2-candidacy:1", ts.Nodes(), true, true)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(4),
			testsuite.WithLatestCommitmentSlotIndex(5),
			testsuite.WithEqualStoredCommitmentAtIndex(5),
			testsuite.WithLatestCommitmentCumulativeWeight(4), // 4 * slot 5
			testsuite.WithSybilProtectionOnlineCommittee(ts.SeatOfNodes(5, "node0", "node1", "node2", "node3")...),
			testsuite.WithSybilProtectionCandidates(0, ts.AccountsOfNodes("node1", "node2")),
			testsuite.WithEvictedSlot(5),
		)

		ts.AssertAttestationsForSlot(5, ts.Blocks("P0:5.3-node0", "P0:5.3-node1", "P0:5.3-node2", "P0:5.3-node3"), ts.Nodes()...)

		ts.AssertStrongTips(ts.Blocks("P0:8.3-node0", "P0:8.3-node1", "P0:8.3-node2", "P0:8.3-node3"), ts.Nodes()...)

		ts.AssertBlocksExist(ts.BlocksWithPrefix("P0"), true, ts.Nodes()...)
	}

	// Split into partitions P1 and P2.
	ts.SplitIntoPartitions(map[string][]*mock.Node{
		"P1": nodesP1,
		"P2": nodesP2,
	})

	// Issue blocks in partition 1.
	{
		ts.IssueBlocksAtSlots("P1:", []iotago.SlotIndex{9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}, 4, "P0:8.3", nodesP1, true, true)

		ts.AssertNodeState(nodesP1,
			testsuite.WithLatestFinalizedSlot(16),
			testsuite.WithLatestCommitmentSlotIndex(17),
			testsuite.WithEqualStoredCommitmentAtIndex(17),
			testsuite.WithLatestCommitmentCumulativeWeight(43), // 4 + see attestation assertions below for how to compute
			testsuite.WithSybilProtectionOnlineCommittee(ts.SeatOfNodes(17, "node1", "node2")...),
			testsuite.WithEvictedSlot(17),
		)

		// Assert committee in epoch 1.
		ts.AssertSybilProtectionCandidates(0, ts.AccountsOfNodes("node1", "node2"), nodesP1...)
		ts.AssertSybilProtectionCommittee(1, ts.AccountsOfNodes("node1", "node2"), nodesP1...) // we selected a new committee for epoch 1

		// Assert committee in epoch 2.
		ts.AssertSybilProtectionCandidates(1, iotago.AccountIDs{}, nodesP1...)

		ts.AssertAttestationsForSlot(6, ts.Blocks("P0:6.3-node0", "P0:6.3-node1", "P0:6.3-node2", "P0:6.3-node3"), nodesP1...) // Committee in epoch 1 is all nodes
		ts.AssertAttestationsForSlot(7, ts.Blocks("P0:7.3-node0", "P0:7.3-node1", "P0:7.3-node2", "P0:7.3-node3"), nodesP1...) // Committee in epoch 1 is all nodes
		ts.AssertAttestationsForSlot(8, ts.Blocks("P0:8.3-node0", "P0:8.3-node1", "P0:8.3-node2", "P0:8.3-node3"), nodesP1...) // Committee in epoch 1 is all nodes
		ts.AssertAttestationsForSlot(9, ts.Blocks("P1:9.3-node0", "P1:9.3-node1", "P1:9.3-node2", "P0:8.3-node3"), nodesP1...) // Committee in epoch 1 is all nodes; and we carry attestations of others because of window
		ts.AssertAttestationsForSlot(10, ts.Blocks("P1:10.3-node0", "P1:10.3-node1", "P1:10.3-node2"), nodesP1...)             // Committee in epoch 1 is all nodes; node3 is in P2
		ts.AssertAttestationsForSlot(11, ts.Blocks("P1:11.3-node0", "P1:11.3-node1", "P1:11.3-node2"), nodesP1...)             // Committee in epoch 1 is all nodes; node3 is in P2
		ts.AssertAttestationsForSlot(12, ts.Blocks("P1:12.3-node0", "P1:12.3-node1", "P1:12.3-node2"), nodesP1...)             // Committee in epoch 1 is all nodes; node3 is in P2
		ts.AssertAttestationsForSlot(13, ts.Blocks("P1:13.3-node0", "P1:13.3-node1", "P1:13.3-node2"), nodesP1...)             // Committee in epoch 1 is all nodes; node3 is in P2
		ts.AssertAttestationsForSlot(14, ts.Blocks("P1:14.3-node0", "P1:14.3-node1", "P1:14.3-node2"), nodesP1...)             // Committee in epoch 1 is all nodes; node3 is in P2
		ts.AssertAttestationsForSlot(15, ts.Blocks("P1:15.3-node0", "P1:15.3-node1", "P1:15.3-node2"), nodesP1...)             // Committee in epoch 1 is all nodes; node3 is in P2
		ts.AssertAttestationsForSlot(16, ts.Blocks("P1:15.3-node0", "P1:16.3-node1", "P1:16.3-node2"), nodesP1...)             // We're in Epoch 2 (only node1, node2) but we carry attestations of others because of window
		ts.AssertAttestationsForSlot(17, ts.Blocks("P1:17.3-node1", "P1:17.3-node2"), nodesP1...)                              // Committee in epoch 2 is only node1, node2

		ts.AssertStrongTips(ts.Blocks("P1:20.3-node0", "P1:20.3-node1", "P1:20.3-node2"), nodesP1...)

		ts.AssertBlocksExist(ts.BlocksWithPrefix("P1"), true, nodesP1...)
		ts.AssertBlocksExist(ts.BlocksWithPrefix("P1"), false, nodesP2...)
	}

	// Issue blocks in partition 2.
	{
		ts.IssueBlocksAtSlots("P2:", []iotago.SlotIndex{9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}, 4, "P0:8.3", nodesP2, true, true)

		ts.AssertNodeState(nodesP2,
			testsuite.WithLatestFinalizedSlot(4),
			testsuite.WithLatestCommitmentSlotIndex(17),
			testsuite.WithEqualStoredCommitmentAtIndex(17),
			// testsuite.WithLatestCommitmentCumulativeWeight(43), // 4 + see attestation assertions below for how to compute
			testsuite.WithSybilProtectionOnlineCommittee(ts.SeatOfNodes(17, "node3")...),
			testsuite.WithEvictedSlot(17),
		)

		// Assert committee in epoch 1.
		ts.AssertSybilProtectionCandidates(0, ts.AccountsOfNodes("node1", "node2"), nodesP2...)
		ts.AssertSybilProtectionCommittee(1, ts.AccountsOfNodes("node0", "node1", "node2", "node3"), nodesP2...) // committee was reused due to no finalization at epochNearingThreshold

		// Assert committee in epoch 2.
		ts.AssertSybilProtectionCandidates(1, iotago.AccountIDs{}, nodesP2...)

		ts.AssertAttestationsForSlot(6, ts.Blocks("P0:6.3-node0", "P0:6.3-node1", "P0:6.3-node2", "P0:6.3-node3"), nodesP2...) // Committee in epoch 1 is all nodes
		ts.AssertAttestationsForSlot(7, ts.Blocks("P0:7.3-node0", "P0:7.3-node1", "P0:7.3-node2", "P0:7.3-node3"), nodesP2...) // Committee in epoch 1 is all nodes
		ts.AssertAttestationsForSlot(8, ts.Blocks("P0:8.3-node0", "P0:8.3-node1", "P0:8.3-node2", "P0:8.3-node3"), nodesP2...) // Committee in epoch 1 is all nodes
		ts.AssertAttestationsForSlot(9, ts.Blocks("P0:8.3-node0", "P0:8.3-node1", "P0:8.3-node2", "P2:9.3-node3"), nodesP2...) // Committee in epoch 1 is all nodes; and we carry attestations of others because of window
		ts.AssertAttestationsForSlot(10, ts.Blocks("P2:10.3-node3"), nodesP2...)                                               // Committee in epoch 1 is all nodes; only node3 is in P2
		ts.AssertAttestationsForSlot(11, ts.Blocks("P2:11.3-node3"), nodesP2...)                                               // Committee in epoch 1 is all nodes; only node3 is in P2
		ts.AssertAttestationsForSlot(12, ts.Blocks("P2:12.3-node3"), nodesP2...)                                               // Committee in epoch 1 is all nodes; only node3 is in P2
		ts.AssertAttestationsForSlot(13, ts.Blocks("P2:13.3-node3"), nodesP2...)                                               // Committee in epoch 1 is all nodes; only node3 is in P2
		ts.AssertAttestationsForSlot(14, ts.Blocks("P2:14.3-node3"), nodesP2...)                                               // Committee in epoch 1 is all nodes; only node3 is in P2
		ts.AssertAttestationsForSlot(15, ts.Blocks("P2:15.3-node3"), nodesP2...)                                               // Committee in epoch 1 is all nodes; only node3 is in P2
		ts.AssertAttestationsForSlot(16, ts.Blocks("P2:16.3-node3"), nodesP2...)                                               // Committee in epoch 2 (reused) is all nodes; only node3 is in P2
		ts.AssertAttestationsForSlot(17, ts.Blocks("P2:17.3-node3"), nodesP2...)                                               // Committee in epoch 2 (reused) is all nodes; only node3 is in P2

		ts.AssertStrongTips(ts.Blocks("P2:20.3-node3"), nodesP2...)

		ts.AssertBlocksExist(ts.BlocksWithPrefix("P2"), true, nodesP2...)
		ts.AssertBlocksExist(ts.BlocksWithPrefix("P2"), false, nodesP1...)
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

		node3.Validator.IssueActivity(ctxP2, wg, 21, node3)

		// P1 finalized until slot 16. We do not expect any forks here because our CW is higher than the other partition's.
		ts.AssertForkDetectedCount(0, nodesP1...)
		// P1's chain is heavier, they should not consider switching the chain.
		ts.AssertCandidateEngineActivatedCount(0, nodesP1...)
		ctxP2Cancel() // we can stop issuing on P2.

		// Nodes from P2 should switch the chain.
		ts.AssertForkDetectedCount(1, nodesP2...)
		ts.AssertCandidateEngineActivatedCount(1, nodesP2...)

		// Here we need to let enough time pass for the nodes to sync up the candidate engines and switch them
		ts.AssertMainEngineSwitchedCount(1, nodesP2...)

		// Make sure that enough activity messages are issued so that a block in slot 21 gets accepted and triggers commitment of slot 18.
		time.Sleep(3 * time.Second)

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

	// Assert committee in epoch 1.
	ts.AssertSybilProtectionCandidates(0, ts.AccountsOfNodes("node1", "node2"), ts.Nodes()...)
	ts.AssertSybilProtectionCommittee(1, ts.AccountsOfNodes("node1", "node2"), ts.Nodes()...) // we selected a new committee for epoch 1

	// Assert committee in epoch 2.
	ts.AssertSybilProtectionCandidates(1, iotago.AccountIDs{}, ts.Nodes()...)

	ts.AssertAttestationsForSlot(6, ts.Blocks("P0:6.3-node0", "P0:6.3-node1", "P0:6.3-node2", "P0:6.3-node3"), ts.Nodes()...) // Committee in epoch 1 is all nodes
	ts.AssertAttestationsForSlot(7, ts.Blocks("P0:7.3-node0", "P0:7.3-node1", "P0:7.3-node2", "P0:7.3-node3"), ts.Nodes()...) // Committee in epoch 1 is all nodes
	ts.AssertAttestationsForSlot(8, ts.Blocks("P0:8.3-node0", "P0:8.3-node1", "P0:8.3-node2", "P0:8.3-node3"), ts.Nodes()...) // Committee in epoch 1 is all nodes
	ts.AssertAttestationsForSlot(9, ts.Blocks("P1:9.3-node0", "P1:9.3-node1", "P1:9.3-node2", "P0:8.3-node3"), ts.Nodes()...) // Committee in epoch 1 is all nodes; and we carry attestations of others because of window
	ts.AssertAttestationsForSlot(10, ts.Blocks("P1:10.3-node0", "P1:10.3-node1", "P1:10.3-node2"), ts.Nodes()...)             // Committee in epoch 1 is all nodes; node3 is in P2
	ts.AssertAttestationsForSlot(11, ts.Blocks("P1:11.3-node0", "P1:11.3-node1", "P1:11.3-node2"), ts.Nodes()...)             // Committee in epoch 1 is all nodes; node3 is in P2
	ts.AssertAttestationsForSlot(12, ts.Blocks("P1:12.3-node0", "P1:12.3-node1", "P1:12.3-node2"), ts.Nodes()...)             // Committee in epoch 1 is all nodes; node3 is in P2
	ts.AssertAttestationsForSlot(13, ts.Blocks("P1:13.3-node0", "P1:13.3-node1", "P1:13.3-node2"), ts.Nodes()...)             // Committee in epoch 1 is all nodes; node3 is in P2
	ts.AssertAttestationsForSlot(14, ts.Blocks("P1:14.3-node0", "P1:14.3-node1", "P1:14.3-node2"), ts.Nodes()...)             // Committee in epoch 1 is all nodes; node3 is in P2
	ts.AssertAttestationsForSlot(15, ts.Blocks("P1:15.3-node0", "P1:15.3-node1", "P1:15.3-node2"), ts.Nodes()...)             // Committee in epoch 1 is all nodes; node3 is in P2
	ts.AssertAttestationsForSlot(16, ts.Blocks("P1:15.3-node0", "P1:16.3-node1", "P1:16.3-node2"), ts.Nodes()...)             // We're in Epoch 2 (only node1, node2) but we carry attestations of others because of window
	ts.AssertAttestationsForSlot(17, ts.Blocks("P1:17.3-node1", "P1:17.3-node2"), ts.Nodes()...)                              // Committee in epoch 2 is only node1, node2
}

func TestProtocol_EngineSwitching_Tie(t *testing.T) {
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

	nodes := []*mock.Node{
		ts.AddValidatorNode("node0"),
		ts.AddValidatorNode("node1"),
		ts.AddValidatorNode("node2"),
	}

	ts.AddDefaultWallet(nodes[0])

	const expectedCommittedSlotAfterPartitionMerge = 18

	nodeOptions := make(map[string][]options.Option[protocol.Protocol])
	for _, node := range ts.Nodes() {
		nodeOptions[node.Name] = []options.Option[protocol.Protocol]{
			protocol.WithSybilProtectionProvider(
				sybilprotectionv1.NewProvider(
					sybilprotectionv1.WithSeatManagerProvider(module.Provide(func(e *engine.Engine) seatmanager.SeatManager {
						poa := mock2.NewManualPOAProvider()(e).(*mock2.ManualPOA)
						for _, node := range lo.Filter(nodes, (*mock.Node).IsValidator) {
							poa.AddAccount(node.Validator.AccountID, node.Name)
						}

						poa.SetOnline(lo.Map(nodes, func(sourceType *mock.Node) string { return sourceType.Name })...)

						return poa
					})),
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

	nodes[0].Protocol.SetLogLevel(log.LevelPanic)
	nodes[1].Protocol.SetLogLevel(log.LevelPanic)
	nodes[2].Protocol.SetLogLevel(log.LevelPanic)
	nodes[2].Protocol.Chains.WithInitializedEngines(func(_ *protocol.Chain, engine *engine.Engine) (shutdown func()) {
		engine.BlockDAG.SetLogLevel(log.LevelError)

		return nil
	})

	expectedCommittee := []iotago.AccountID{nodes[0].Validator.AccountID, nodes[1].Validator.AccountID, nodes[2].Validator.AccountID}

	seatIndexes := []account.SeatIndex{
		lo.Return1(lo.Return1(nodes[0].Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(nodes[0].Validator.AccountID)),
		lo.Return1(lo.Return1(nodes[0].Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(nodes[1].Validator.AccountID)),
		lo.Return1(lo.Return1(nodes[0].Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(nodes[2].Validator.AccountID)),
	}

	for _, node := range ts.Nodes() {
		node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().(*mock2.ManualPOA).SetOnline("node0", "node1", "node2")
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
			testsuite.WithSybilProtectionOnlineCommittee(seatIndexes...),
			testsuite.WithEvictedSlot(0),
			testsuite.WithActiveRootBlocks(ts.Blocks("Genesis")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis")),
		)
	}

	// Issue up to slot 13 in P0 (main partition with all nodes) and verify that the nodes have the expected states.
	{
		ts.IssueBlocksAtSlots("P0:", []iotago.SlotIndex{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}, 4, "Genesis", ts.Nodes(), true, false)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(10),
			testsuite.WithLatestCommitmentSlotIndex(11),
			testsuite.WithEqualStoredCommitmentAtIndex(11),
			testsuite.WithLatestCommitmentCumulativeWeight(24), // 4 for each slot starting from 4
			testsuite.WithSybilProtectionCommittee(ts.API.TimeProvider().EpochFromSlot(11), expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(seatIndexes...),
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

	// Split into partitions P1, P2 and P3.
	ts.SplitIntoPartitions(map[string][]*mock.Node{
		"P1": {nodes[0]},
		"P2": {nodes[1]},
		"P3": {nodes[2]},
	})

	// Set online committee for each partition.
	for _, node := range ts.Nodes() {
		manualPOA := node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().(*mock2.ManualPOA)
		if node.Partition == "P1" {
			manualPOA.SetOnline("node0")
			manualPOA.SetOffline("node1", "node2")
		} else if node.Partition == "P2" {
			manualPOA.SetOnline("node1")
			manualPOA.SetOffline("node0", "node2")
		} else {
			manualPOA.SetOnline("node2")
			manualPOA.SetOffline("node0", "node1")
		}
	}

	ts.AssertSybilProtectionOnlineCommittee(seatIndexes[0:1], nodes[0])
	ts.AssertSybilProtectionOnlineCommittee(seatIndexes[1:2], nodes[1])
	ts.AssertSybilProtectionOnlineCommittee(seatIndexes[2:3], nodes[2])

	nodesInPartition := func(partition int) []*mock.Node {
		switch {
		case partition == 1:
			return nodes[0:1]
		case partition == 2:
			return nodes[1:2]
		case partition == 3:
			return nodes[2:3]
		default:
			return nodes
		}
	}

	nodesOutsidePartition := func(partition int) []*mock.Node {
		switch {
		case partition == 1:
			return []*mock.Node{nodes[1], nodes[2]}
		case partition == 2:
			return []*mock.Node{nodes[0], nodes[2]}
		case partition == 3:
			return []*mock.Node{nodes[0], nodes[1]}
		default:
			return []*mock.Node{}
		}
	}

	onlineCommittee := func(partition int) []account.SeatIndex {
		switch {
		case partition == 1:
			return []account.SeatIndex{seatIndexes[0]}
		case partition == 2:
			return []account.SeatIndex{seatIndexes[1]}
		case partition == 3:
			return []account.SeatIndex{seatIndexes[2]}
		default:
			return seatIndexes
		}
	}

	issueBlocks := func(partition int, slots []iotago.SlotIndex) {
		parentSlot := slots[0] - 1
		lastIssuedSlot := slots[len(slots)-1]
		targetNodes := nodesInPartition(partition)
		otherNodes := nodesOutsidePartition(partition)

		ts.IssueBlocksAtSlots(slotPrefix(partition, slots[0]), slots, 4, slotPrefix(partition, parentSlot)+strconv.Itoa(int(parentSlot))+".3", targetNodes, true, false)

		ts.AssertNodeState(targetNodes,
			testsuite.WithLatestFinalizedSlot(10),
			testsuite.WithLatestCommitmentSlotIndex(18),
			testsuite.WithEqualStoredCommitmentAtIndex(18),
			testsuite.WithLatestCommitmentCumulativeWeight(39),
			testsuite.WithSybilProtectionCommittee(ts.API.TimeProvider().EpochFromSlot(18), expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(onlineCommittee(partition)...),
			testsuite.WithEvictedSlot(18),
		)

		for _, slot := range []iotago.SlotIndex{12, 13, 14, 15} {
			var attestationBlocks Blocks
			for _, node := range targetNodes {
				attestationBlocks.Add(ts, node, partition, slot)
			}
			for _, node := range otherNodes {
				attestationBlocks.Add(ts, node, partition, min(slot, 13)) // carry forward last known attestations
			}

			ts.AssertAttestationsForSlot(slot, attestationBlocks, targetNodes...)
		}

		for _, slot := range []iotago.SlotIndex{16, 17, 18} {
			var attestationBlocks Blocks
			for _, node := range targetNodes {
				attestationBlocks.Add(ts, node, partition, slot)
			}

			ts.AssertAttestationsForSlot(slot, attestationBlocks, targetNodes...)
		}

		var tipBlocks Blocks
		for _, node := range targetNodes {
			tipBlocks.Add(ts, node, partition, lastIssuedSlot)
		}

		ts.AssertStrongTips(tipBlocks, targetNodes...)
	}

	issueBlocks(1, []iotago.SlotIndex{14, 15, 16, 17, 18, 19, 20})
	issueBlocks(2, []iotago.SlotIndex{14, 15, 16, 17, 18, 19, 20})
	issueBlocks(3, []iotago.SlotIndex{14, 15, 16, 17, 18, 19, 20})

	//for _, node := range ts.Nodes() {
	//	manualPOA := node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().(*mock2.ManualPOA)
	//	manualPOA.SetOnline("node0", "node1", "node2")
	//}

	// Merge the partitions
	{
		fmt.Println("\n=========================\nMerging network partitions\n=========================")

		ts.MergePartitionsToMain()
	}
	//
	//	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	//	defer cancel()
	//
	//	ctxP1, ctxP1Cancel := context.WithCancel(ctx)
	//	ctxP2, ctxP2Cancel := context.WithCancel(ctx)
	//	ctxP3, ctxP2Cancel := context.WithCancel(ctx)
	//
	//	wg := &sync.WaitGroup{}
	//
	//	// Issue blocks on both partitions after merging the networks.
	//	node0.Validator.IssueActivity(ctxP1, wg, 21, node0)
	//	node1.Validator.IssueActivity(ctxP2, wg, 21, node1)
	//	node2.Validator.IssueActivity(ctxP3, wg, 21, node2)
	//
	//	// P1 finalized until slot 18. We do not expect any forks here because our CW is higher than the other partition's.
	//	ts.AssertForkDetectedCount(0, nodesP1...)
	//	// P1's chain is heavier, they should not consider switching the chain.
	//	ts.AssertCandidateEngineActivatedCount(0, nodesP1...)
	//	ctxP2Cancel() // we can stop issuing on P2.
	//
	//	// Nodes from P2 should switch the chain.
	//	ts.AssertForkDetectedCount(1, nodesP2...)
	//	ts.AssertCandidateEngineActivatedCount(1, nodesP2...)
	//
	//	// Here we need to let enough time pass for the nodes to sync up the candidate engines and switch them
	//	ts.AssertMainEngineSwitchedCount(1, nodesP2...)
	//
	//	ctxP1Cancel()
	//	wg.Wait()
	//}
	//
	//// Make sure that nodes that switched their engine still have blocks with prefix P0 from before the fork.
	//// Those nodes should also have all the blocks from the target fork P1 and should not have blocks from P2.
	//// This is to make sure that the storage was copied correctly during engine switching.
	//ts.AssertBlocksExist(ts.BlocksWithPrefix("P0"), true, ts.Nodes()...)
	//ts.AssertBlocksExist(ts.BlocksWithPrefix("P1"), true, ts.Nodes()...)
	//ts.AssertBlocksExist(ts.BlocksWithPrefix("P2"), false, ts.Nodes()...)
	//
	//ts.AssertEqualStoredCommitmentAtIndex(expectedCommittedSlotAfterPartitionMerge, ts.Nodes()...)
}

func slotPrefix(partition int, slot iotago.SlotIndex) string {
	if slot <= 13 {
		return "P0:"
	}

	return "P" + strconv.Itoa(partition) + ":"
}

type Blocks []*blocks.Block

func (a *Blocks) Add(ts *testsuite.TestSuite, node *mock.Node, partition int, slot iotago.SlotIndex) {
	*a = append(*a, ts.Block(fmt.Sprintf("%s%d.3-%s", slotPrefix(partition, slot), slot, node.Name)))
}
