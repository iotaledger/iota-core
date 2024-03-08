package tests

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
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

func TestProtocol_EngineSwitching_No_Verified_Commitments(t *testing.T) {
	const maxCommittableAge = iotago.SlotIndex(4)
	const expectedCommittedSlotAfterPartitionMerge = 22

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
				maxCommittableAge,
				5,
			),
		),

		testsuite.WithWaitFor(5*time.Second),
	)
	defer ts.Shutdown()

	node0 := ts.AddValidatorNode("node0")
	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddNode("node2")
	node3 := ts.AddValidatorNode("node3")
	node4 := ts.AddValidatorNode("node4")
	node5 := ts.AddNode("node5")
	ts.AddDefaultWallet(node0)

	nodes := []*mock.Node{node0, node1, node2, node3, node4, node5}
	nodesP1 := []*mock.Node{node0, node1, node2}
	nodesP2 := []*mock.Node{node3, node4, node5}

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

						return poa
					})),
				),
			),
		}
	}

	ts.Run(false, nodeOptions)

	expectedCommittee := []iotago.AccountID{
		node0.Validator.AccountID,
		node1.Validator.AccountID,
		node3.Validator.AccountID,
		node4.Validator.AccountID,
	}
	expectedP1OnlineCommittee := []account.SeatIndex{
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node0.Validator.AccountID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node1.Validator.AccountID)),
	}
	expectedP2OnlineCommittee := []account.SeatIndex{
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node3.Validator.AccountID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node4.Validator.AccountID)),
	}
	expectedOnlineCommittee := append(expectedP1OnlineCommittee, expectedP2OnlineCommittee...)

	for _, node := range ts.Nodes() {
		node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().(*mock2.ManualPOA).SetOnline("node0", "node1", "node3", "node4")
	}

	// Verify that nodes have the expected states.
	{
		genesisCommitment := ts.CommitmentOfMainEngine(node0, 0)
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitment(genesisCommitment.Commitment()),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithMainChainID(genesisCommitment.ID()),

			testsuite.WithSybilProtectionCommittee(0, expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommittee...),
			testsuite.WithEvictedSlot(0),
			testsuite.WithActiveRootBlocks(ts.Blocks("Genesis")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis")),
		)
	}

	// Issue up to slot 8 in P0 (main partition with all nodes) and verify that the nodes have the expected states.
	{
		ts.IssueBlocksAtSlots("P0:", []iotago.SlotIndex{1, 2, 3, 4, 5, 6, 7, 8}, 4, "Genesis", ts.Nodes(), true, true)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(5),
			testsuite.WithLatestCommitmentSlotIndex(6),
			testsuite.WithEqualStoredCommitmentAtIndex(6),
			testsuite.WithLatestCommitmentCumulativeWeight(12), // 4 for each slot starting from 4
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommittee...),
			testsuite.WithEvictedSlot(6),
		)

		ts.AssertAttestationsForSlot(4, ts.Blocks("P0:4.3-node0", "P0:4.3-node1", "P0:4.3-node3", "P0:4.3-node4"), ts.Nodes()...)
		ts.AssertAttestationsForSlot(5, ts.Blocks("P0:5.3-node0", "P0:5.3-node1", "P0:5.3-node3", "P0:5.3-node4"), ts.Nodes()...)
		ts.AssertAttestationsForSlot(6, ts.Blocks("P0:6.3-node0", "P0:6.3-node1", "P0:6.3-node3", "P0:6.3-node4"), ts.Nodes()...)

		// Make sure the tips are properly set.
		ts.AssertStrongTips(ts.BlocksWithPrefix("P0:8.3"), ts.Nodes()...)

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
			manualPOA.SetOnline("node0", "node1")
			manualPOA.SetOffline("node3", "node4")
		} else {
			manualPOA.SetOnline("node3", "node4")
			manualPOA.SetOffline("node0", "node1")
		}
	}

	ts.AssertSybilProtectionOnlineCommittee(expectedP1OnlineCommittee, nodesP1...)
	ts.AssertSybilProtectionOnlineCommittee(expectedP2OnlineCommittee, nodesP2...)

	// Issue blocks in partition 1.
	{
		ts.IssueBlocksAtSlots("P1:", []iotago.SlotIndex{9, 10, 11}, 4, "P0:8.3", nodesP1[:len(nodesP1)-1], true, true)

		ts.AssertNodeState(nodesP1,
			testsuite.WithLatestFinalizedSlot(5),
			testsuite.WithLatestCommitmentSlotIndex(9),
			testsuite.WithEqualStoredCommitmentAtIndex(9),
			testsuite.WithLatestCommitmentCumulativeWeight(24),
		)

		ts.AssertAttestationsForSlot(7, ts.Blocks("P0:7.3-node0", "P0:7.3-node1", "P0:7.3-node3", "P0:7.3-node4"), nodesP1...)
		ts.AssertAttestationsForSlot(8, ts.Blocks("P0:8.3-node0", "P0:8.3-node1", "P0:8.3-node3", "P0:8.3-node4"), nodesP1...)
		ts.AssertAttestationsForSlot(9, ts.Blocks("P1:9.3-node0", "P1:9.3-node1", "P0:8.3-node3", "P0:8.3-node4"), nodesP1...)

		// Assert Protocol.Chains and Protocol.Commitments state.
		ts.AssertLatestEngineCommitmentOnMainChain(nodesP1...)
		ts.AssertUniqueCommitmentChain(nodesP1...)
		ts.AssertCommitmentsOnChainAndChainHasCommitments(ts.CommitmentsOfMainEngine(nodesP1[0], 1, 9), ts.CommitmentOfMainEngine(nodesP1[0], 1).ID(), nodesP1...)
		ts.AssertCommitmentsOnEvictedChain(ts.CommitmentsOfMainEngine(nodesP1[0], 1, 9), false, nodesP1...)
		ts.AssertCommitmentsAndChainsEvicted(0, nodesP1...)

		// Make sure the tips are properly set.
		ts.AssertStrongTips(ts.BlocksWithPrefix("P1:11.3"), nodesP1...)
	}

	// Issue blocks in partition 2.
	{
		ts.IssueBlocksAtSlots("P2:", []iotago.SlotIndex{9, 10, 11}, 4, "P0:8.3", nodesP2[:len(nodesP2)-1], true, true)

		ts.AssertNodeState(nodesP2,
			testsuite.WithLatestFinalizedSlot(5),
			testsuite.WithLatestCommitmentSlotIndex(9),
			testsuite.WithEqualStoredCommitmentAtIndex(9),
			testsuite.WithLatestCommitmentCumulativeWeight(24),
		)

		ts.AssertAttestationsForSlot(7, ts.Blocks("P0:7.3-node0", "P0:7.3-node1", "P0:7.3-node3", "P0:7.3-node4"), nodesP2...)
		ts.AssertAttestationsForSlot(8, ts.Blocks("P0:8.3-node0", "P0:8.3-node1", "P0:8.3-node3", "P0:8.3-node4"), nodesP2...)
		ts.AssertAttestationsForSlot(9, ts.Blocks("P0:8.3-node0", "P0:8.3-node1", "P2:9.3-node3", "P2:9.3-node4"), nodesP2...)

		// Assert Protocol.Chains and Protocol.Commitments state.
		ts.AssertLatestEngineCommitmentOnMainChain(nodesP2...)
		ts.AssertUniqueCommitmentChain(nodesP2...)
		ts.AssertCommitmentsOnChainAndChainHasCommitments(ts.CommitmentsOfMainEngine(nodesP2[0], 1, 9), ts.CommitmentOfMainEngine(nodesP2[0], 1).ID(), nodesP2...)
		ts.AssertCommitmentsOnEvictedChain(ts.CommitmentsOfMainEngine(nodesP2[0], 1, 9), false, nodesP2...)
		ts.AssertCommitmentsAndChainsEvicted(0, nodesP2...)

		// Make sure the tips are properly set.
		ts.AssertStrongTips(ts.BlocksWithPrefix("P2:11.3"), nodesP2...)
	}

	oldestNonEvictedCommitment := 5 - maxCommittableAge
	commitmentsMainChainP2 := ts.CommitmentsOfMainEngine(nodesP2[0], oldestNonEvictedCommitment, 9)

	// Merge the partitions
	{
		ts.MergePartitionsToMain()
		fmt.Println("\n=========================\nMerged network partitions\n=========================")

		for _, node := range ts.Nodes() {
			node.Protocol.Chains.WithInitializedEngines(func(chain *protocol.Chain, engine *engine.Engine) (shutdown func()) {
				manualPOA := engine.SybilProtection.SeatManager().(*mock2.ManualPOA)
				manualPOA.SetOnline("node0")
				manualPOA.SetOffline("node1", "node3", "node4")

				return nil
			})
		}

		require.NoError(t, node0.Protocol.Engines.Main.Get().Notarization.ForceCommitUntil(20))
		ts.IssueValidationBlockWithOptions("revive", node0,
			mock.WithValidationBlockHeaderOptions(
				mock.WithSlotCommitment(ts.CommitmentOfMainEngine(node0, 20).Commitment()),
				mock.WithIssuingTime(ts.API.TimeProvider().SlotStartTime(22)),
			),
		)

		ts.IssueBlocksAtSlots("P0-merged:", []iotago.SlotIndex{22, 23, 24}, 3, "revive", ts.Nodes("node0"), true, false)
	}

	{
		ts.AssertNodeState(nodes,
			testsuite.WithLatestFinalizedSlot(5),
			testsuite.WithLatestCommitmentSlotIndex(expectedCommittedSlotAfterPartitionMerge),
			testsuite.WithEqualStoredCommitmentAtIndex(expectedCommittedSlotAfterPartitionMerge),
			testsuite.WithLatestCommitmentCumulativeWeight(31),
		)
		ts.AssertAttestationsForSlot(10, ts.Blocks("P1:10.3-node0", "P1:10.3-node1"), nodes...)
		ts.AssertAttestationsForSlot(11, ts.Blocks("P1:11.1-node0", "P1:11.1-node1"), nodes...) // 11.1 are the last blocks that got accepted before reviving
		ts.AssertAttestationsForSlot(12, ts.Blocks("P1:11.1-node0", "P1:11.1-node1"), nodes...)
		for _, slot := range []iotago.SlotIndex{13, 14, 15, 16, 17, 18, 19, 20, 21} {
			ts.AssertAttestationsForSlot(slot, ts.Blocks(), nodes...)
		}
		ts.AssertAttestationsForSlot(22, ts.Blocks("P0-merged:22.2-node0"), nodes...)
	}

	commitmentsMainChain := ts.CommitmentsOfMainEngine(nodesP1[0], oldestNonEvictedCommitment, expectedCommittedSlotAfterPartitionMerge)

	ts.AssertMainChain(commitmentsMainChain[8].ID(), nodesP2...)
	ts.AssertMainChain(commitmentsMainChain[0].ID(), nodesP1...)

	ts.AssertCommitmentsOnChainAndChainHasCommitments(commitmentsMainChain, commitmentsMainChain[0].ID(), nodesP1...)
	ts.AssertCommitmentsOnChainAndChainHasCommitments(commitmentsMainChain[8:], commitmentsMainChain[8].ID(), nodesP2...)

	// Check that the losing chain has an expected state on P2.
	// P1 is not even aware of this chain because it hasn't received any blocks.
	ts.AssertCommitmentsOnChainAndChainHasCommitments(commitmentsMainChainP2, commitmentsMainChainP2[0].ID(), nodesP2...)

	ts.AssertUniqueCommitmentChain(ts.Nodes()...)
	ts.AssertLatestEngineCommitmentOnMainChain(ts.Nodes()...)
	ts.AssertCommitmentsOnEvictedChain(commitmentsMainChain, false, ts.Nodes()...)
	ts.AssertCommitmentsAndChainsEvicted(oldestNonEvictedCommitment-1, ts.Nodes()...)
}

func TestProtocol_EngineSwitching(t *testing.T) {
	var (
		genesisSlot       iotago.SlotIndex = 0
		minCommittableAge iotago.SlotIndex = 2
		maxCommittableAge iotago.SlotIndex = 4
	)

	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				genesisSlot,
				testsuite.GenesisTimeWithOffsetBySlots(1000, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				3,
			),
			iotago.WithLivenessOptions(
				10,
				10,
				minCommittableAge,
				maxCommittableAge,
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
						return e.SyncManager.LatestCommitment().Slot() >= expectedCommittedSlotAfterPartitionMerge && e.Notarization.IsBootstrapped()
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
			testsuite.WithMainChainID(genesisCommitment.MustID()),
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

		// Assert Protocol.Chains and Protocol.Commitments state.
		ts.AssertLatestEngineCommitmentOnMainChain(nodesP1...)
		ts.AssertUniqueCommitmentChain(nodesP1...)
		ts.AssertCommitmentsOnChainAndChainHasCommitments(ts.CommitmentsOfMainEngine(nodesP1[0], 13, 18), ts.CommitmentOfMainEngine(nodesP1[0], 13).ID(), nodesP1...)
		ts.AssertCommitmentsOnEvictedChain(ts.CommitmentsOfMainEngine(nodesP1[0], 13, 18), false, nodesP1...)
		ts.AssertCommitmentsAndChainsEvicted(12, nodesP1...)

		// Make sure the tips are properly set.
		var tipBlocks []*blocks.Block
		for _, node := range nodesP1[:len(nodesP1)-1] {
			tipBlocks = append(tipBlocks, ts.Block(fmt.Sprintf("P1:20.3-%s", node.Name)))
		}
		ts.AssertStrongTips(tipBlocks, nodesP1...)
	}

	var engineCommitmentsP2 []*model.Commitment

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

		// Assert Protocol.Chains and Protocol.Commitments state.
		engineCommitmentsP2 = ts.CommitmentsOfMainEngine(nodesP2[0], 6, 18)
		ts.AssertLatestEngineCommitmentOnMainChain(nodesP2...)
		ts.AssertUniqueCommitmentChain(nodesP2...)
		ts.AssertCommitmentsOnChainAndChainHasCommitments(engineCommitmentsP2, ts.CommitmentOfMainEngine(nodesP1[0], 6).ID(), nodesP2...)
		ts.AssertCommitmentsOnEvictedChain(engineCommitmentsP2, false, nodesP2...)
		ts.AssertCommitmentsAndChainsEvicted(5, nodesP2...)

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

	// Merge the partitions
	{
		ts.MergePartitionsToMain()
		fmt.Println("\n=========================\nMerged network partitions\n=========================")

		// Issue blocks on both partitions after merging the networks.
		ts.IssueBlocksAtSlots("P2-merge:", []iotago.SlotIndex{20, 21}, 4, "P2:20.3", nodesP2[:len(nodesP2)-1], true, true)

		// P1 finalized until slot 18. We do not expect any forks here because our CW is higher than the other partition's.
		ts.AssertForkDetectedCount(0, nodesP1...)
		// P1's chain is heavier, they should not consider switching the chain.
		ts.AssertCandidateEngineActivatedCount(0, nodesP1...)

		for _, node := range ts.Nodes() {
			manualPOA := node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().(*mock2.ManualPOA)
			manualPOA.SetOnline("node0", "node1", "node2", "node3", "node4", "node6", "node7")
		}

		ts.IssueBlocksAtSlots("P1-merge:", []iotago.SlotIndex{20, 21, 22}, 4, "P1:20.3", nodesP1[:len(nodesP1)-1], true, true)

		// Nodes from P2 should switch the chain.
		ts.AssertForkDetectedCount(1, nodesP2...)
		ts.AssertCandidateEngineActivatedCount(1, nodesP2...)

		// Here we need to let enough time pass for the nodes to sync up the candidate engines and switch them
		ts.AssertMainEngineSwitchedCount(1, nodesP2...)
	}

	// Make sure that nodes that switched their engine still have blocks with prefix P0 from before the fork.
	// Those nodes should also have all the blocks from the target fork P1 and should not have blocks from P2.
	// This is to make sure that the storage was copied correctly during engine switching.
	ts.AssertBlocksExist(ts.BlocksWithPrefix("P0"), true, ts.Nodes()...)
	ts.AssertBlocksExist(ts.BlocksWithPrefix("P1"), true, ts.Nodes()...)
	ts.AssertBlocksExist(ts.BlocksWithPrefix("P2"), false, ts.Nodes()...)

	ts.AssertEqualStoredCommitmentAtIndex(expectedCommittedSlotAfterPartitionMerge, ts.Nodes()...)

	// Assert Protocol.Chains and Protocol.Commitments state.
	{
		oldestNonEvictedCommitment := iotago.SlotIndex(15)
		ultimateCommitmentsP2 := lo.Filter(engineCommitmentsP2, func(commitment *model.Commitment) bool {
			return commitment.Slot() >= oldestNonEvictedCommitment
		})

		ts.AssertLatestFinalizedSlot(19, ts.Nodes()...)

		commitmentsMainChain := ts.CommitmentsOfMainEngine(nodesP1[0], oldestNonEvictedCommitment, expectedCommittedSlotAfterPartitionMerge)

		ts.AssertMainChain(ts.CommitmentOfMainEngine(nodesP1[0], oldestNonEvictedCommitment).ID(), ts.Nodes()...)
		ts.AssertUniqueCommitmentChain(ts.Nodes()...)
		ts.AssertLatestEngineCommitmentOnMainChain(ts.Nodes()...)
		ts.AssertCommitmentsOnEvictedChain(ultimateCommitmentsP2, true, ts.Nodes()...)

		ts.AssertCommitmentsOnEvictedChain(commitmentsMainChain, false, ts.Nodes()...)

		ts.AssertCommitmentsOnChainAndChainHasCommitments(commitmentsMainChain, ts.CommitmentOfMainEngine(nodesP1[0], oldestNonEvictedCommitment).ID(), ts.Nodes()...)

		// Since we diverge in slot 14 the forking point of the chain is at slot 14.
		commitment14P2 := lo.First(lo.Filter(engineCommitmentsP2, func(commitment *model.Commitment) bool {
			return commitment.Slot() == 14
		}))
		ts.AssertCommitmentsOnChain(ultimateCommitmentsP2, commitment14P2.ID(), nodesP1...)

		// Before the merge we finalize until slot 10 (root commitment=6), so the forking point of the then main chain
		// is at slot 6.
		ts.AssertCommitmentsOnChain(ultimateCommitmentsP2, ts.CommitmentOfMainEngine(nodesP2[0], 6).ID(), nodesP2...)

		ts.AssertCommitmentsAndChainsEvicted(oldestNonEvictedCommitment-1, ts.Nodes()...)
	}
}

func TestProtocol_EngineSwitching_CommitteeRotation(t *testing.T) {
	var (
		genesisSlot       iotago.SlotIndex = 0
		minCommittableAge iotago.SlotIndex = 2
		maxCommittableAge iotago.SlotIndex = 5
	)

	ts := testsuite.NewTestSuite(t,
		testsuite.WithWaitFor(15*time.Second),

		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				genesisSlot,
				testsuite.GenesisTimeWithOffsetBySlots(1000, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				4, // 16 slots per epoch
			),
			iotago.WithLivenessOptions(
				10,
				10,
				minCommittableAge,
				maxCommittableAge,
				10,
			),
		),
	)
	defer ts.Shutdown()

	node0 := ts.AddValidatorNode("node0")
	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddValidatorNode("node2")
	node3 := ts.AddValidatorNode("node3")

	const expectedCommittedSlotAfterPartitionMerge = 19
	nodesP1 := []*mock.Node{node0, node1, node2}
	nodesP2 := []*mock.Node{node3}

	nodeOpts := []options.Option[protocol.Protocol]{
		protocol.WithNotarizationProvider(
			slotnotarization.NewProvider(),
		),
		protocol.WithSybilProtectionProvider(
			sybilprotectionv1.NewProvider(
				sybilprotectionv1.WithSeatManagerProvider(
					topstakers.NewProvider(),
				),
			),
		),
		protocol.WithSyncManagerProvider(
			trivialsyncmanager.NewProvider(
				trivialsyncmanager.WithBootstrappedFunc(func(e *engine.Engine) bool {
					return e.SyncManager.LatestCommitment().Slot() >= expectedCommittedSlotAfterPartitionMerge && e.Notarization.IsBootstrapped()
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
		genesisCommitment := ts.CommitmentOfMainEngine(node0, 0)
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithSnapshotImported(true),
			testsuite.WithProtocolParameters(ts.API.ProtocolParameters()),
			testsuite.WithLatestCommitment(genesisCommitment.Commitment()),
			testsuite.WithLatestFinalizedSlot(0),
			testsuite.WithMainChainID(genesisCommitment.ID()),
			testsuite.WithStorageCommitments([]*iotago.Commitment{genesisCommitment.Commitment()}),

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

		// Important note: We need to issue with `useMinCommittableAge` set to true, to make sure that the blocks do commit to a deterministic slot, so that we can assert the attestations and how they are carried forward later.
		ts.IssueBlocksAtSlots("P0:", []iotago.SlotIndex{3, 4, 5, 6, 7}, 4, "P0:node2-candidacy:1", ts.Nodes(), true, true)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(4),
			testsuite.WithLatestCommitmentSlotIndex(5),
			testsuite.WithEqualStoredCommitmentAtIndex(5),
			testsuite.WithLatestCommitmentCumulativeWeight(4), // 4 * slot 6
			testsuite.WithSybilProtectionOnlineCommittee(ts.SeatOfNodes(5, "node0", "node1", "node2", "node3")...),
			testsuite.WithSybilProtectionCandidates(0, ts.AccountsOfNodes("node1", "node2")),
			testsuite.WithEvictedSlot(5),
		)

		ts.AssertAttestationsForSlot(5, ts.Blocks("P0:5.3-node0", "P0:5.3-node1", "P0:5.3-node2", "P0:5.3-node3"), ts.Nodes()...)

		ts.AssertStrongTips(ts.Blocks("P0:7.3-node0", "P0:7.3-node1", "P0:7.3-node2", "P0:7.3-node3"), ts.Nodes()...)

		ts.AssertBlocksExist(ts.BlocksWithPrefix("P0"), true, ts.Nodes()...)
	}

	// Split into partitions P1 and P2.
	ts.SplitIntoPartitions(map[string][]*mock.Node{
		"P1": nodesP1,
		"P2": nodesP2,
	})

	// Issue blocks in partition 1.
	{
		ts.IssueBlocksAtSlots("P1:", []iotago.SlotIndex{8, 9, 10, 11, 12, 13, 14, 15}, 4, "P0:7.3", nodesP1, true, true)
		// Slot 16 is the new epoch, where node0 is not part of the (online) committee. Therefore, we can't issue blocks with commitments at minCommittableAge anymore.
		// This is fine, as we keep issuing with the committee members and therefore within each slot there is a newer attestation that replaces the previous one.
		// However, it is important that Block(P1:15.3-node0) commits to Slot12, as this decides how far we carry the attestation forward.
		ts.IssueBlocksAtSlots("P1:", []iotago.SlotIndex{16, 17, 18, 19, 20}, 4, "P1:15.3", nodesP1, true, false)

		ts.AssertNodeState(nodesP1,
			testsuite.WithLatestFinalizedSlot(17),
			testsuite.WithLatestCommitmentSlotIndex(18),
			testsuite.WithEqualStoredCommitmentAtIndex(18),
			testsuite.WithSybilProtectionOnlineCommittee(ts.SeatOfNodes(18, "node1", "node2")...),
			testsuite.WithEvictedSlot(18),
		)

		// Assert committee in epoch 1.
		ts.AssertSybilProtectionCandidates(0, ts.AccountsOfNodes("node1", "node2"), nodesP1...)
		ts.AssertSybilProtectionCommittee(1, ts.AccountsOfNodes("node1", "node2"), nodesP1...) // we selected a new committee for epoch 1

		// Assert committee in epoch 2.
		ts.AssertSybilProtectionCandidates(1, iotago.AccountIDs{}, nodesP1...)

		ts.AssertAttestationsForSlot(6, ts.Blocks("P0:6.3-node0", "P0:6.3-node1", "P0:6.3-node2", "P0:6.3-node3"), nodesP1...) // Committee in epoch 1 is all nodes
		ts.AssertAttestationsForSlot(7, ts.Blocks("P0:7.3-node0", "P0:7.3-node1", "P0:7.3-node2", "P0:7.3-node3"), nodesP1...) // Committee in epoch 1 is all nodes
		ts.AssertAttestationsForSlot(8, ts.Blocks("P1:8.3-node0", "P1:8.3-node1", "P1:8.3-node2", "P0:7.3-node3"), nodesP1...) // Committee in epoch 1 is all nodes; and we carry attestations of others because of window
		ts.AssertAttestationsForSlot(9, ts.Blocks("P1:9.3-node0", "P1:9.3-node1", "P1:9.3-node2", "P0:7.3-node3"), nodesP1...) // Committee in epoch 1 is all nodes; and we carry attestations of others because of window
		ts.AssertAttestationsForSlot(10, ts.Blocks("P1:10.3-node0", "P1:10.3-node1", "P1:10.3-node2"), nodesP1...)             // Committee in epoch 1 is all nodes; node3 is in P2
		ts.AssertAttestationsForSlot(11, ts.Blocks("P1:11.3-node0", "P1:11.3-node1", "P1:11.3-node2"), nodesP1...)             // Committee in epoch 1 is all nodes; node3 is in P2
		ts.AssertAttestationsForSlot(12, ts.Blocks("P1:12.3-node0", "P1:12.3-node1", "P1:12.3-node2"), nodesP1...)             // Committee in epoch 1 is all nodes; node3 is in P2
		ts.AssertAttestationsForSlot(13, ts.Blocks("P1:13.3-node0", "P1:13.3-node1", "P1:13.3-node2"), nodesP1...)             // Committee in epoch 1 is all nodes; node3 is in P2
		ts.AssertAttestationsForSlot(14, ts.Blocks("P1:14.3-node0", "P1:14.3-node1", "P1:14.3-node2"), nodesP1...)             // Committee in epoch 1 is all nodes; node3 is in P2
		ts.AssertAttestationsForSlot(15, ts.Blocks("P1:15.3-node0", "P1:15.3-node1", "P1:15.3-node2"), nodesP1...)             // Committee in epoch 1 is all nodes; node3 is in P2
		ts.AssertAttestationsForSlot(16, ts.Blocks("P1:15.3-node0", "P1:16.3-node1", "P1:16.3-node2"), nodesP1...)             // We're in Epoch 2 (only node1, node2) but we carry attestations of others because of window (=maxCommittableAge). Block(P1:15.3-node0) commits to Slot12.
		ts.AssertAttestationsForSlot(17, ts.Blocks("P1:15.3-node0", "P1:17.3-node1", "P1:17.3-node2"), nodesP1...)             // Committee in epoch 2 is only node1, node2. Block(P1:15.3-node0) commits to Slot12.
		ts.AssertAttestationsForSlot(18, ts.Blocks("P1:18.3-node1", "P1:18.3-node2"), nodesP1...)                              // Committee in epoch 2 is only node1, node2. Block(P1:15.3-node0) commits to Slot12, that's why it is not carried to 18.
		ts.AssertLatestCommitmentCumulativeWeight(46, nodesP1...)                                                              // 4 + see attestation assertions above for how to compute

		ts.AssertStrongTips(ts.Blocks("P1:20.3-node0", "P1:20.3-node1", "P1:20.3-node2"), nodesP1...)

		ts.AssertBlocksExist(ts.BlocksWithPrefix("P1"), true, nodesP1...)
		ts.AssertBlocksExist(ts.BlocksWithPrefix("P1"), false, nodesP2...)

		// Assert Protocol.Chains and Protocol.Commitments state.
		engineCommitmentsP1 := ts.CommitmentsOfMainEngine(nodesP1[0], 12, 18)
		ts.AssertLatestEngineCommitmentOnMainChain(nodesP1...)
		ts.AssertUniqueCommitmentChain(nodesP1...)
		ts.AssertCommitmentsOnChainAndChainHasCommitments(engineCommitmentsP1, ts.CommitmentOfMainEngine(node0, 12).ID(), nodesP1...)
		ts.AssertCommitmentsOnEvictedChain(engineCommitmentsP1, false, nodesP1...)
		ts.AssertCommitmentsAndChainsEvicted(11, nodesP1...)
	}

	var engineCommitmentsP2 []*model.Commitment

	// Issue blocks in partition 2.
	{
		ts.IssueBlocksAtSlots("P2:", []iotago.SlotIndex{8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}, 4, "P0:7.3", nodesP2, true, false)
		// Here we keep issuing with the only (online) validator in the partition. Since we issue in each slot, we don't need to set `useCommitmentAtMinCommittableAge` to true, as the newer blocks/attestations in each slot replace the previous ones.

		ts.AssertNodeState(nodesP2,
			testsuite.WithLatestFinalizedSlot(4),
			testsuite.WithLatestCommitmentSlotIndex(18),
			testsuite.WithEqualStoredCommitmentAtIndex(18),
			testsuite.WithSybilProtectionOnlineCommittee(ts.SeatOfNodes(18, "node3")...),
			testsuite.WithEvictedSlot(18),
		)

		// Assert committee in epoch 1.
		ts.AssertSybilProtectionCandidates(0, ts.AccountsOfNodes("node1", "node2"), nodesP2...)
		ts.AssertSybilProtectionCommittee(1, ts.AccountsOfNodes("node0", "node1", "node2", "node3"), nodesP2...) // committee was reused due to no finalization at epochNearingThreshold

		// Assert committee in epoch 2.
		ts.AssertSybilProtectionCandidates(1, iotago.AccountIDs{}, nodesP2...)

		ts.AssertAttestationsForSlot(6, ts.Blocks("P0:6.3-node0", "P0:6.3-node1", "P0:6.3-node2", "P0:6.3-node3"), nodesP2...) // Committee in epoch 1 is all nodes
		ts.AssertAttestationsForSlot(7, ts.Blocks("P0:7.3-node0", "P0:7.3-node1", "P0:7.3-node2", "P0:7.3-node3"), nodesP2...) // Committee in epoch 1 is all nodes
		ts.AssertAttestationsForSlot(8, ts.Blocks("P0:7.3-node0", "P0:7.3-node1", "P0:7.3-node2", "P2:8.3-node3"), nodesP2...) // Committee in epoch 1 is all nodes; and we carry attestations of others because of window
		ts.AssertAttestationsForSlot(9, ts.Blocks("P0:7.3-node0", "P0:7.3-node1", "P0:7.3-node2", "P2:9.3-node3"), nodesP2...) // Committee in epoch 1 is all nodes; and we carry attestations of others because of window
		ts.AssertAttestationsForSlot(10, ts.Blocks("P2:10.3-node3"), nodesP2...)                                               // Committee in epoch 1 is all nodes; only node3 is in P2
		ts.AssertAttestationsForSlot(11, ts.Blocks("P2:11.3-node3"), nodesP2...)                                               // Committee in epoch 1 is all nodes; only node3 is in P2
		ts.AssertAttestationsForSlot(12, ts.Blocks("P2:12.3-node3"), nodesP2...)                                               // Committee in epoch 1 is all nodes; only node3 is in P2
		ts.AssertAttestationsForSlot(13, ts.Blocks("P2:13.3-node3"), nodesP2...)                                               // Committee in epoch 1 is all nodes; only node3 is in P2
		ts.AssertAttestationsForSlot(14, ts.Blocks("P2:14.3-node3"), nodesP2...)                                               // Committee in epoch 1 is all nodes; only node3 is in P2
		ts.AssertAttestationsForSlot(15, ts.Blocks("P2:15.3-node3"), nodesP2...)                                               // Committee in epoch 1 is all nodes; only node3 is in P2
		ts.AssertAttestationsForSlot(16, ts.Blocks("P2:16.3-node3"), nodesP2...)                                               // Committee in epoch 2 (reused) is all nodes; only node3 is in P2
		ts.AssertAttestationsForSlot(17, ts.Blocks("P2:17.3-node3"), nodesP2...)                                               // Committee in epoch 2 (reused) is all nodes; only node3 is in P2
		ts.AssertAttestationsForSlot(18, ts.Blocks("P2:18.3-node3"), nodesP2...)                                               // Committee in epoch 2 (reused) is all nodes; only node3 is in P2
		ts.AssertLatestCommitmentCumulativeWeight(29, nodesP2...)                                                              // 4 + see attestation assertions above for how to compute

		ts.AssertStrongTips(ts.Blocks("P2:20.3-node3"), nodesP2...)

		ts.AssertBlocksExist(ts.BlocksWithPrefix("P2"), true, nodesP2...)
		ts.AssertBlocksExist(ts.BlocksWithPrefix("P2"), false, nodesP1...)

		// Assert Protocol.Chains and Protocol.Commitments state.
		engineCommitmentsP2 = ts.CommitmentsOfMainEngine(nodesP2[0], 0, 18)
		ts.AssertLatestEngineCommitmentOnMainChain(nodesP2...)
		ts.AssertUniqueCommitmentChain(nodesP2...)
		ts.AssertCommitmentsOnChainAndChainHasCommitments(engineCommitmentsP2, ts.CommitmentOfMainEngine(node0, 0).ID(), nodesP2...)
		ts.AssertCommitmentsOnEvictedChain(engineCommitmentsP2, false, nodesP2...)
		// We only finalized until slot 4, and maxCommittableAge=5. Thus, we don't expect any evictions on chains/commmitments yet.
	}

	// Merge the partitions
	{
		ts.MergePartitionsToMain()
		fmt.Println("\n=========================\nMerged network partitions\n=========================")

		ts.IssueBlocksAtSlots("P2-merge:", []iotago.SlotIndex{20, 21}, 4, "P2:20.3", nodesP2, true, true)

		// P1 finalized until slot 16. We do not expect any forks here because our CW is higher than the other partition's.
		ts.AssertForkDetectedCount(0, nodesP1...)
		// P1's chain is heavier, they should not consider switching the chain.
		ts.AssertCandidateEngineActivatedCount(0, nodesP1...)

		ts.IssueBlocksAtSlots("P1-merge:", []iotago.SlotIndex{20, 21, 22}, 4, "P1:20.3", nodesP1, true, true)

		// Nodes from P2 should switch the chain.
		ts.AssertForkDetectedCount(1, nodesP2...)
		ts.AssertCandidateEngineActivatedCount(1, nodesP2...)

		// Here we need to let enough time pass for the nodes to sync up the candidate engines and switch them
		ts.AssertMainEngineSwitchedCount(1, nodesP2...)
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
	ts.AssertAttestationsForSlot(8, ts.Blocks("P1:8.3-node0", "P1:8.3-node1", "P1:8.3-node2", "P0:7.3-node3"), ts.Nodes()...) // Committee in epoch 1 is all nodes; and we carry attestations of others because of window
	ts.AssertAttestationsForSlot(9, ts.Blocks("P1:9.3-node0", "P1:9.3-node1", "P1:9.3-node2", "P0:7.3-node3"), ts.Nodes()...) // Committee in epoch 1 is all nodes; and we carry attestations of others because of window
	ts.AssertAttestationsForSlot(10, ts.Blocks("P1:10.3-node0", "P1:10.3-node1", "P1:10.3-node2"), ts.Nodes()...)             // Committee in epoch 1 is all nodes; node3 is in P2
	ts.AssertAttestationsForSlot(11, ts.Blocks("P1:11.3-node0", "P1:11.3-node1", "P1:11.3-node2"), ts.Nodes()...)             // Committee in epoch 1 is all nodes; node3 is in P2
	ts.AssertAttestationsForSlot(12, ts.Blocks("P1:12.3-node0", "P1:12.3-node1", "P1:12.3-node2"), ts.Nodes()...)             // Committee in epoch 1 is all nodes; node3 is in P2
	ts.AssertAttestationsForSlot(13, ts.Blocks("P1:13.3-node0", "P1:13.3-node1", "P1:13.3-node2"), ts.Nodes()...)             // Committee in epoch 1 is all nodes; node3 is in P2
	ts.AssertAttestationsForSlot(14, ts.Blocks("P1:14.3-node0", "P1:14.3-node1", "P1:14.3-node2"), ts.Nodes()...)             // Committee in epoch 1 is all nodes; node3 is in P2
	ts.AssertAttestationsForSlot(15, ts.Blocks("P1:15.3-node0", "P1:15.3-node1", "P1:15.3-node2"), ts.Nodes()...)             // Committee in epoch 1 is all nodes; node3 is in P2
	ts.AssertAttestationsForSlot(16, ts.Blocks("P1:15.3-node0", "P1:16.3-node1", "P1:16.3-node2"), nodesP1...)                // We're in Epoch 2 (only node1, node2) but we carry attestations of others because of window (=maxCommittableAge). Block(P1:15.3-node0) commits to Slot12.
	ts.AssertAttestationsForSlot(17, ts.Blocks("P1:15.3-node0", "P1:17.3-node1", "P1:17.3-node2"), nodesP1...)                // Committee in epoch 2 is only node1, node2. Block(P1:15.3-node0) commits to Slot12.
	ts.AssertAttestationsForSlot(18, ts.Blocks("P1:18.3-node1", "P1:18.3-node2"), nodesP1...)                                 // Committee in epoch 2 is only node1, node2. Block(P1:15.3-node0) commits to Slot12, that's why it is not carried to 18.
	ts.AssertAttestationsForSlot(19, ts.Blocks("P1:19.3-node1", "P1:19.3-node2"), ts.Nodes()...)                              // Committee in epoch 2 is only node1, node2

	ts.AssertLatestFinalizedSlot(19, ts.Nodes()...)

	oldestNonEvictedCommitment := 19 - maxCommittableAge
	commitmentsMainChain := ts.CommitmentsOfMainEngine(node0, oldestNonEvictedCommitment, expectedCommittedSlotAfterPartitionMerge)
	ultimateCommitmentsP2 := lo.Filter(engineCommitmentsP2, func(commitment *model.Commitment) bool {
		return commitment.Slot() >= oldestNonEvictedCommitment
	})

	// Assert Protocol.Chains and Protocol.Commitments state.
	{
		ts.AssertUniqueCommitmentChain(ts.Nodes()...)
		ts.AssertLatestEngineCommitmentOnMainChain(ts.Nodes()...)
		ts.AssertCommitmentsOnEvictedChain(ultimateCommitmentsP2, true, ts.Nodes()...)
		ts.AssertCommitmentsOnEvictedChain(commitmentsMainChain, false, ts.Nodes()...)
		ts.AssertCommitmentsOnChainAndChainHasCommitments(commitmentsMainChain, ts.CommitmentOfMainEngine(nodesP1[0], oldestNonEvictedCommitment).ID(), ts.Nodes()...)

		// Since we diverge in slot 8 and P1 finalized until slot 17 (root commitment=12) before the merge,
		// the chain is not solidifiable and there will never be a chain created for nodes on P1.
		ts.AssertCommitmentsOnChain(ultimateCommitmentsP2, iotago.EmptyCommitmentID, nodesP1...)

		// After the merge we finalize until slot 19 (root commitment=14), so the chain is evicted (we check this above)
		// and therefore i
		ts.AssertCommitmentsOnChain(ultimateCommitmentsP2, ts.CommitmentOfMainEngine(node3, 0).ID(), nodesP2...)

		ts.AssertCommitmentsAndChainsEvicted(oldestNonEvictedCommitment-1, ts.Nodes()...)
	}
}

func TestProtocol_EngineSwitching_Tie(t *testing.T) {
	var (
		genesisSlot       iotago.SlotIndex = 0
		minCommittableAge iotago.SlotIndex = 2
		maxCommittableAge iotago.SlotIndex = 4
	)

	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				genesisSlot,
				testsuite.GenesisTimeWithOffsetBySlots(1000, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				3,
			),
			iotago.WithLivenessOptions(
				10,
				10,
				minCommittableAge,
				maxCommittableAge,
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

	validatorsByAccountID := map[iotago.AccountID]*mock.Node{
		nodes[0].Validator.AccountID: nodes[0],
		nodes[1].Validator.AccountID: nodes[1],
		nodes[2].Validator.AccountID: nodes[2],
	}

	ts.AddDefaultWallet(nodes[0])

	const expectedCommittedSlotAfterPartitionMerge = 18
	const forkingSlot = 14

	nodeOptions := []options.Option[protocol.Protocol]{
		protocol.WithSybilProtectionProvider(
			sybilprotectionv1.NewProvider(
				sybilprotectionv1.WithSeatManagerProvider(module.Provide(func(e *engine.Engine) seatmanager.SeatManager {
					poa := mock2.NewManualPOAProvider()(e).(*mock2.ManualPOA)
					for _, node := range lo.Filter(nodes, (*mock.Node).IsValidator) {
						poa.AddAccount(node.Validator.AccountID, node.Name)
					}

					onlineValidators := ds.NewSet[string]()

					e.Constructed.OnTrigger(func() {
						e.Events.PostSolidFilter.BlockAllowed.Hook(func(block *blocks.Block) {
							if node, exists := validatorsByAccountID[block.ModelBlock().ProtocolBlock().Header.IssuerID]; exists && onlineValidators.Add(node.Name) {
								e.LogError("node online", "name", node.Name)
								poa.SetOnline(onlineValidators.ToSlice()...)
							}
						})
					})

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

	nodesOptions := make(map[string][]options.Option[protocol.Protocol])
	for _, node := range ts.Nodes() {
		nodesOptions[node.Name] = nodeOptions
	}

	ts.Run(false, nodesOptions)

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
			testsuite.WithMainChainID(genesisCommitment.MustID()),
			testsuite.WithStorageCommitments([]*iotago.Commitment{genesisCommitment}),

			testsuite.WithSybilProtectionCommittee(0, expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(seatIndexes...),
			testsuite.WithEvictedSlot(0),
			testsuite.WithActiveRootBlocks(ts.Blocks("Genesis")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis")),
		)
	}

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

	lastCommonSlot := iotago.SlotIndex(13)

	issueBlocks := func(partition int, slots []iotago.SlotIndex) {
		parentSlot := slots[0] - 1
		lastIssuedSlot := slots[len(slots)-1]
		targetNodes := nodesInPartition(partition)
		otherNodes := nodesOutsidePartition(partition)
		lastCommittedSlot := lastIssuedSlot - minCommittableAge

		initialParentsPrefix := slotPrefix(partition, parentSlot) + strconv.Itoa(int(parentSlot)) + ".3"
		if parentSlot == genesisSlot {
			initialParentsPrefix = "Genesis"
		}

		ts.IssueBlocksAtSlots(slotPrefix(partition, slots[0]), slots, 4, initialParentsPrefix, targetNodes, true, true)

		cumulativeAttestations := uint64(0)
		for slot := genesisSlot + maxCommittableAge; slot <= lastCommittedSlot; slot++ {
			var attestationBlocks Blocks
			for _, node := range targetNodes {
				attestationBlocks.Add(ts, node, partition, slot)

				cumulativeAttestations++
			}

			for _, node := range otherNodes {
				// We force the commitments to be at minCommittableAge-1.
				if slot <= lastCommonSlot+minCommittableAge-1 {
					attestationBlocks.Add(ts, node, partition, min(slot, lastCommonSlot)) // carry forward last known attestations

					cumulativeAttestations++
				}
			}

			ts.AssertAttestationsForSlot(slot, attestationBlocks, targetNodes...)
		}

		ts.AssertNodeState(targetNodes,
			testsuite.WithLatestFinalizedSlot(10),
			testsuite.WithLatestCommitmentSlotIndex(lastCommittedSlot),
			testsuite.WithEqualStoredCommitmentAtIndex(lastCommittedSlot),
			testsuite.WithLatestCommitmentCumulativeWeight(cumulativeAttestations),
			testsuite.WithSybilProtectionCommittee(ts.API.TimeProvider().EpochFromSlot(lastCommittedSlot), expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(onlineCommittee(partition)...),
			testsuite.WithEvictedSlot(lastCommittedSlot),
		)

		var tipBlocks Blocks
		for _, node := range targetNodes {
			tipBlocks.Add(ts, node, partition, lastIssuedSlot)
		}

		ts.AssertStrongTips(tipBlocks, targetNodes...)
	}

	issueBlocks(0, []iotago.SlotIndex{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13})

	ts.AssertUniqueCommitmentChain(ts.Nodes()...)
	ts.AssertLatestEngineCommitmentOnMainChain(ts.Nodes()...)

	commitmentsMainChain := ts.CommitmentsOfMainEngine(nodes[0], 6, 11)
	ts.AssertCommitmentsOnEvictedChain(commitmentsMainChain, false, ts.Nodes()...)
	ts.AssertCommitmentsOnChainAndChainHasCommitments(commitmentsMainChain, commitmentsMainChain[0].ID(), ts.Nodes()...)
	ts.AssertCommitmentsAndChainsEvicted(5, ts.Nodes()...)

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

	issueBlocks(1, []iotago.SlotIndex{14, 15, 16, 17, 18, 19, 20})
	issueBlocks(2, []iotago.SlotIndex{14, 15, 16, 17, 18, 19, 20})
	issueBlocks(3, []iotago.SlotIndex{14, 15, 16, 17, 18, 19, 20})

	ts.AssertUniqueCommitmentChain(ts.Nodes()...)
	ts.AssertLatestEngineCommitmentOnMainChain(ts.Nodes()...)

	commitment140 := ts.CommitmentOfMainEngine(nodes[0], 14)
	commitment141 := ts.CommitmentOfMainEngine(nodes[1], 14)
	commitment142 := ts.CommitmentOfMainEngine(nodes[2], 14)

	var mainPartition []*mock.Node
	var otherPartitions []*mock.Node

	partitionsInOrder := []*types.Tuple[int, *model.Commitment]{types.NewTuple(1, commitment140), types.NewTuple(2, commitment141), types.NewTuple(3, commitment142)}
	slices.SortFunc(partitionsInOrder, func(a, b *types.Tuple[int, *model.Commitment]) int {
		return bytes.Compare(lo.PanicOnErr(a.B.ID().Bytes()), lo.PanicOnErr(b.B.ID().Bytes()))
	})

	mainPartition = nodes[partitionsInOrder[2].A-1 : partitionsInOrder[2].A]
	otherPartitions = []*mock.Node{nodes[partitionsInOrder[0].A-1], nodes[partitionsInOrder[1].A-1]}

	engineCommitmentsP2 := ts.CommitmentsOfMainEngine(otherPartitions[1], lastCommonSlot+1, 18)
	engineCommitmentsP3 := ts.CommitmentsOfMainEngine(otherPartitions[0], lastCommonSlot+1, 18)

	// Merge the partitions
	{
		fmt.Println("")
		fmt.Println("==========================")
		fmt.Println("Merging network partitions")
		fmt.Println("--------------------------")
		fmt.Println("Winner: ", mainPartition[0].Protocol.LogName())
		fmt.Println("Losers: ", otherPartitions[0].Protocol.LogName(), otherPartitions[1].Protocol.LogName())
		fmt.Println("==========================")
		fmt.Println("")

		ts.MergePartitionsToMain()
	}

	// Make sure the nodes switch their engines.
	{
		ts.IssueBlocksAtSlots(fmt.Sprintf("P%d-merge:", partitionsInOrder[0].A), []iotago.SlotIndex{20}, 1, slotPrefix(partitionsInOrder[0].A, 20)+strconv.Itoa(20)+".3", nodes[partitionsInOrder[0].A-1:partitionsInOrder[0].A], true, true)
		ts.AssertCommitmentsOnChainAndChainHasCommitments(engineCommitmentsP3, engineCommitmentsP3[0].ID(), mainPartition[0], otherPartitions[1])

		ts.IssueBlocksAtSlots(fmt.Sprintf("P%d-merge:", partitionsInOrder[1].A), []iotago.SlotIndex{20}, 1, slotPrefix(partitionsInOrder[1].A, 20)+strconv.Itoa(20)+".3", nodes[partitionsInOrder[1].A-1:partitionsInOrder[1].A], true, true)
		ts.AssertMainEngineSwitchedCount(1, otherPartitions[0])
		ts.AssertCommitmentsOnChainAndChainHasCommitments(engineCommitmentsP3, engineCommitmentsP3[0].ID(), mainPartition[0], otherPartitions[1])
		ts.AssertCommitmentsOnChainAndChainHasCommitments(engineCommitmentsP2, engineCommitmentsP2[0].ID(), mainPartition[0], otherPartitions[0])

		ts.IssueBlocksAtSlots(fmt.Sprintf("P%d-merge:", partitionsInOrder[2].A), []iotago.SlotIndex{20}, 1, slotPrefix(partitionsInOrder[2].A, 20)+strconv.Itoa(20)+".3", nodes[partitionsInOrder[2].A-1:partitionsInOrder[2].A], true, true)
		ts.AssertCommitmentsOnChainAndChainHasCommitments(engineCommitmentsP3, engineCommitmentsP3[0].ID(), mainPartition[0], otherPartitions[1])
		ts.AssertCommitmentsOnChainAndChainHasCommitments(engineCommitmentsP2, engineCommitmentsP2[0].ID(), mainPartition[0], otherPartitions[0])
		ts.AssertCommitmentsOnChainAndChainHasCommitments(commitmentsMainChain, commitmentsMainChain[0].ID(), otherPartitions...)

		ts.AssertMainEngineSwitchedCount(0, mainPartition...)
		ts.AssertMainEngineSwitchedCount(2, otherPartitions[0])
		ts.AssertMainEngineSwitchedCount(1, otherPartitions[1])

		ts.AssertEqualStoredCommitmentAtIndex(expectedCommittedSlotAfterPartitionMerge, ts.Nodes()...)
	}

	oldestNonEvictedCommitment := iotago.SlotIndex(6)

	commitmentsMainChain = ts.CommitmentsOfMainEngine(mainPartition[0], oldestNonEvictedCommitment, expectedCommittedSlotAfterPartitionMerge)
	ultimateCommitmentsP2 := lo.Filter(engineCommitmentsP2, func(commitment *model.Commitment) bool {
		return commitment.Slot() >= forkingSlot
	})
	ultimateCommitmentsP3 := lo.Filter(engineCommitmentsP3, func(commitment *model.Commitment) bool {
		return commitment.Slot() >= forkingSlot
	})

	{
		ts.AssertUniqueCommitmentChain(ts.Nodes()...)

		ts.AssertMainChain(ts.CommitmentOfMainEngine(mainPartition[0], 6).ID(), mainPartition...)
		ts.AssertMainChain(ts.CommitmentOfMainEngine(mainPartition[0], forkingSlot).ID(), otherPartitions...)

		ts.AssertLatestEngineCommitmentOnMainChain(ts.Nodes()...)

		// We have not evicted the slot below the forking point, so chains are not yet orphaned.
		ts.AssertCommitmentsOnEvictedChain(commitmentsMainChain, false, ts.Nodes()...)
		ts.AssertCommitmentsOnEvictedChain(ultimateCommitmentsP2, false, ts.Nodes()...)
		ts.AssertCommitmentsOnEvictedChain(ultimateCommitmentsP3, false, ts.Nodes()...)

		// The Main partition should have all commitments on the old chain, because it did not switch chains.
		ts.AssertCommitmentsOnChainAndChainHasCommitments(commitmentsMainChain, ts.CommitmentOfMainEngine(mainPartition[0], oldestNonEvictedCommitment).ID(), mainPartition...)
		// Pre-fork commitments should be on the old chains on other partitions.
		ts.AssertCommitmentsOnChainAndChainHasCommitments(commitmentsMainChain[:8], ts.CommitmentOfMainEngine(otherPartitions[0], oldestNonEvictedCommitment).ID(), otherPartitions...)
		// Post-fork winning commitments should be on the new chains on other partitions. This chain is the new main one.
		ts.AssertCommitmentsOnChainAndChainHasCommitments(commitmentsMainChain[8:], ts.CommitmentOfMainEngine(otherPartitions[0], forkingSlot).ID(), otherPartitions...)

		// P2 commitments on the main partition should be on its own chain.
		ts.AssertCommitmentsOnChainAndChainHasCommitments(ultimateCommitmentsP2, ultimateCommitmentsP2[0].ID(), mainPartition...)

		// P2 commitments on P2 node should be on the old chain, that is not the main chain anymore.
		ts.AssertCommitmentsOnChainAndChainHasCommitments(ultimateCommitmentsP2, ts.CommitmentOfMainEngine(otherPartitions[1], oldestNonEvictedCommitment).ID(), otherPartitions[1])
		// P2 commitments on P3 node should be on separate chain.
		ts.AssertCommitmentsOnChainAndChainHasCommitments(ultimateCommitmentsP2, ultimateCommitmentsP2[0].ID(), otherPartitions[0])

		// P3 commitments on the main partition should be on its own chain.
		ts.AssertCommitmentsOnChainAndChainHasCommitments(ultimateCommitmentsP3, ultimateCommitmentsP3[0].ID(), mainPartition...)
		// P3 commitments on P3 node should be on the old chain, that is not the main chain anymore.
		ts.AssertCommitmentsOnChainAndChainHasCommitments(ultimateCommitmentsP3, ts.CommitmentOfMainEngine(otherPartitions[0], oldestNonEvictedCommitment).ID(), otherPartitions[0])
		// P3 commitments on P2 node should be on separate chain.
		ts.AssertCommitmentsOnChainAndChainHasCommitments(ultimateCommitmentsP3, ultimateCommitmentsP3[0].ID(), otherPartitions[1])

		ts.AssertCommitmentsAndChainsEvicted(5, ts.Nodes()...)
	}

	// Finalize further slot and make sure the nodes have the same state of chains.
	{
		ts.IssueBlocksAtSlots("P0-merge:", []iotago.SlotIndex{20, 21, 22}, 3, slotPrefix(partitionsInOrder[len(partitionsInOrder)-1].A, 20)+strconv.Itoa(20)+".2", ts.Nodes(), true, true)

		oldestNonEvictedCommitment = 19 - maxCommittableAge
		commitmentsMainChain = ts.CommitmentsOfMainEngine(mainPartition[0], oldestNonEvictedCommitment, 20)

		ts.AssertLatestFinalizedSlot(19, ts.Nodes()...)

		ts.AssertMainChain(ts.CommitmentOfMainEngine(mainPartition[0], forkingSlot+1).ID(), ts.Nodes()...)

		ts.AssertUniqueCommitmentChain(ts.Nodes()...)
		ts.AssertLatestEngineCommitmentOnMainChain(ts.Nodes()...)
		ts.AssertCommitmentsAndChainsEvicted(forkingSlot, ts.Nodes()...)

		ts.AssertCommitmentsOnEvictedChain(commitmentsMainChain, false, ts.Nodes()...)
		ts.AssertCommitmentsOnChainAndChainHasCommitments(commitmentsMainChain, ts.CommitmentOfMainEngine(mainPartition[len(mainPartition)-1], oldestNonEvictedCommitment).ID(), mainPartition...)

		// The oldest commitment is in the slices are should already be evicted, so we only need to check the newer ones.
		ts.AssertCommitmentsOnEvictedChain(ultimateCommitmentsP2[2:], true, ts.Nodes()...)
		ts.AssertCommitmentsOnEvictedChain(ultimateCommitmentsP3[2:], true, ts.Nodes()...)
	}
}

type Blocks []*blocks.Block

func (a *Blocks) Add(ts *testsuite.TestSuite, node *mock.Node, partition int, slot iotago.SlotIndex) {
	*a = append(*a, ts.Block(fmt.Sprintf("%s%d.3-%s", slotPrefix(partition, slot), slot, node.Name)))
}

func slotPrefix(partition int, slot iotago.SlotIndex) string {
	if slot <= 13 {
		return "P0:"
	}

	return "P" + strconv.Itoa(partition) + ":"
}

func commitmentWithLargestID(commitments ...*model.Commitment) *model.Commitment {
	var largestCommitment *model.Commitment
	for _, commitment := range commitments {
		if largestCommitment == nil || bytes.Compare(lo.PanicOnErr(commitment.ID().Bytes()), lo.PanicOnErr(largestCommitment.ID().Bytes())) > 0 {
			largestCommitment = commitment
		}
	}

	return largestCommitment
}
