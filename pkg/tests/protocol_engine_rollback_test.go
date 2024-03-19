package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager"
	mock2 "github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager/mock"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1"
	"github.com/iotaledger/iota-core/pkg/storage"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestProtocol_EngineRollbackFinalization(t *testing.T) {
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
				3,
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

	poaProvider := func() module.Provider[*engine.Engine, seatmanager.SeatManager] {
		return module.Provide(func(e *engine.Engine) seatmanager.SeatManager {
			poa := mock2.NewManualPOAProvider()(e).(*mock2.ManualPOA)

			for _, node := range []*mock.Node{node0, node1, node2, node3} {
				if node.IsValidator() {
					poa.AddAccount(node.Validator.AccountData.ID, node.Name)
				}
			}
			poa.SetOnline("node0", "node1", "node2", "node3")

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
			protocol.WithStorageOptions(
				storage.WithPruningDelay(20),
			),
		}
	}

	ts.Run(false, nodeOptions)

	// Verify that nodes have the expected states.

	expectedCommittee := []iotago.AccountID{
		node0.Validator.AccountData.ID,
		node1.Validator.AccountData.ID,
		node2.Validator.AccountData.ID,
		node3.Validator.AccountData.ID,
	}
	expectedOnlineCommitteeFull := []account.SeatIndex{
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node0.Validator.AccountData.ID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node1.Validator.AccountData.ID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node2.Validator.AccountData.ID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node3.Validator.AccountData.ID)),
	}

	for _, node := range ts.Nodes() {
		node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().(*mock2.ManualPOA).SetOnline("node0", "node1", "node2", "node3")
	}

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
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommitteeFull...),
			testsuite.WithEvictedSlot(0),
			testsuite.WithActiveRootBlocks(ts.Blocks("Genesis")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis")),
		)
	}

	// Issue up to slot 11 - just before committee selection for the next epoch.
	// Committee will be reused at slot 10 is finalized or slot 12 is committed, whichever happens first.
	{
		ts.IssueBlocksAtSlots("P0:", []iotago.SlotIndex{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}, 4, "Genesis", ts.Nodes(), true, false)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(8),
			testsuite.WithLatestCommitmentSlotIndex(9),
			testsuite.WithEqualStoredCommitmentAtIndex(9),
			testsuite.WithLatestCommitmentCumulativeWeight(28), // 7 for each slot starting from 4
			testsuite.WithSybilProtectionCommittee(ts.API.TimeProvider().EpochFromSlot(9), expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommitteeFull...),
			testsuite.WithEvictedSlot(9),
		)

		for _, slot := range []iotago.SlotIndex{4, 5, 6, 7, 8, 9} {
			var attestationBlocks []*blocks.Block
			for _, node := range ts.Nodes() {
				if node.IsValidator() {
					attestationBlocks = append(attestationBlocks, ts.Block(fmt.Sprintf("P0:%d.3-%s", slot, node.Name)))
				}
			}
			ts.AssertAttestationsForSlot(slot, attestationBlocks, ts.Nodes()...)
		}

		ts.AssertBlocksExist(ts.BlocksWithPrefix("P0"), true, ts.ClientsForNodes()...)
	}

	{
		ts.IssueBlocksAtSlots("P0:", []iotago.SlotIndex{12, 13, 14, 15, 16}, 4, "P0:11.3", ts.Nodes(), true, false)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(13),
			testsuite.WithLatestCommitmentSlotIndex(14),
			testsuite.WithEqualStoredCommitmentAtIndex(14),
			testsuite.WithLatestCommitmentCumulativeWeight(48), // 7 for each slot starting from 4
			testsuite.WithSybilProtectionCommittee(ts.API.TimeProvider().EpochFromSlot(14), expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommitteeFull...),
			testsuite.WithEvictedSlot(14),
		)

		ts.AssertBlocksExist(ts.BlocksWithPrefix("P0"), true, ts.ClientsForNodes()...)
	}

	newEngine, err := node3.Protocol.Engines.ForkAtSlot(13)
	require.NoError(t, err)

	// Assert state of the forked engine after rollback.
	{
		require.EqualValues(t, 13, newEngine.SyncManager.LatestCommitment().Slot())
		require.EqualValues(t, 13, newEngine.SyncManager.LatestFinalizedSlot())
		require.EqualValues(t, 13, newEngine.EvictionState.LastEvictedSlot())

		for epoch := 0; epoch <= 2; epoch++ {
			committeeEpoch, err := newEngine.Storage.Committee().Load(iotago.EpochIndex(epoch))
			require.NoError(t, err)
			require.Equal(t, 4, committeeEpoch.SeatCount())
		}

		// Commmittee for the future epoch does not exist.
		committeeEpoch3, err := newEngine.Storage.Committee().Load(3)
		require.NoError(t, err)
		require.Nil(t, committeeEpoch3)

		for slot := 1; slot <= 13; slot++ {
			copiedCommitment, err := newEngine.Storage.Commitments().Load(iotago.SlotIndex(slot))
			require.NoError(t, err)
			sourceCommitment, err := node1.Protocol.Engines.Main.Get().Storage.Commitments().Load(iotago.SlotIndex(slot))
			require.NoError(t, err)
			require.Equal(t, sourceCommitment.ID(), copiedCommitment.ID())
		}

		// Commitment for the first slot after the fork does not exist.
		_, err = newEngine.Storage.Commitments().Load(iotago.SlotIndex(14))
		require.Error(t, err)
	}
}

func TestProtocol_EngineRollbackNoFinalization(t *testing.T) {
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
				3,
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

	poaProvider := func() module.Provider[*engine.Engine, seatmanager.SeatManager] {
		return module.Provide(func(e *engine.Engine) seatmanager.SeatManager {
			poa := mock2.NewManualPOAProvider()(e).(*mock2.ManualPOA)

			for _, node := range []*mock.Node{node0, node1, node2, node3} {
				if node.IsValidator() {
					poa.AddAccount(node.Validator.AccountData.ID, node.Name)
				}
			}
			poa.SetOnline("node0", "node1", "node2", "node3")

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
			protocol.WithStorageOptions(
				storage.WithPruningDelay(20),
			),
		}
	}

	ts.Run(false, nodeOptions)

	// Verify that nodes have the expected states.

	expectedCommittee := []iotago.AccountID{
		node0.Validator.AccountData.ID,
		node1.Validator.AccountData.ID,
		node2.Validator.AccountData.ID,
		node3.Validator.AccountData.ID,
	}
	expectedOnlineCommitteeFull := []account.SeatIndex{
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node0.Validator.AccountData.ID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node1.Validator.AccountData.ID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node2.Validator.AccountData.ID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node3.Validator.AccountData.ID)),
	}

	expectedOnlineCommitteeHalf := []account.SeatIndex{
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node0.Validator.AccountData.ID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node1.Validator.AccountData.ID)),
	}

	for _, node := range ts.Nodes() {
		node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().(*mock2.ManualPOA).SetOnline("node0", "node1", "node2", "node3")
	}

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
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommitteeFull...),
			testsuite.WithEvictedSlot(0),
			testsuite.WithActiveRootBlocks(ts.Blocks("Genesis")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis")),
		)
	}

	// Issue up to slot 11 - just before committee selection for the next epoch.
	// Committee will be reused when slot 10 is finalized or slot 12 is committed, whichever happens first.
	{
		ts.IssueBlocksAtSlots("P0:", []iotago.SlotIndex{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}, 4, "Genesis", ts.Nodes(), true, false)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(8),
			testsuite.WithLatestCommitmentSlotIndex(9),
			testsuite.WithEqualStoredCommitmentAtIndex(9),
			testsuite.WithLatestCommitmentCumulativeWeight(28), // 7 for each slot starting from 4
			testsuite.WithSybilProtectionCommittee(ts.API.TimeProvider().EpochFromSlot(9), expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommitteeFull...),
			testsuite.WithEvictedSlot(9),
		)

		for _, slot := range []iotago.SlotIndex{4, 5, 6, 7, 8, 9} {
			var attestationBlocks []*blocks.Block
			for _, node := range ts.Nodes() {
				if node.IsValidator() {
					attestationBlocks = append(attestationBlocks, ts.Block(fmt.Sprintf("P0:%d.3-%s", slot, node.Name)))
				}
			}
			ts.AssertAttestationsForSlot(slot, attestationBlocks, ts.Nodes()...)
		}

		ts.AssertBlocksExist(ts.BlocksWithPrefix("P0"), true, ts.ClientsForNodes()...)
	}

	// Update online committee.
	for _, node := range ts.Nodes() {
		manualPOA := node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().(*mock2.ManualPOA)
		manualPOA.SetOnline("node0", "node1")
		manualPOA.SetOffline("node2", "node3")
	}

	{
		ts.IssueBlocksAtSlots("P0:", []iotago.SlotIndex{12, 13, 14, 15, 16}, 4, "P0:11.3", []*mock.Node{node0, node1}, true, false)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(8),
			testsuite.WithLatestCommitmentSlotIndex(14),
			testsuite.WithEqualStoredCommitmentAtIndex(14),
			testsuite.WithLatestCommitmentCumulativeWeight(44), // 7 for each slot starting from 4
			testsuite.WithSybilProtectionCommittee(ts.API.TimeProvider().EpochFromSlot(14), expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommitteeHalf...),
			testsuite.WithEvictedSlot(14),
		)

		ts.AssertBlocksExist(ts.BlocksWithPrefix("P0"), true, ts.ClientsForNodes()...)
	}

	newEngine, err := node3.Protocol.Engines.ForkAtSlot(13)
	require.NoError(t, err)

	// Assert state of the forked engine after rollback.
	{
		require.EqualValues(t, 13, newEngine.SyncManager.LatestCommitment().Slot())
		require.EqualValues(t, 8, newEngine.SyncManager.LatestFinalizedSlot())
		require.EqualValues(t, 13, newEngine.EvictionState.LastEvictedSlot())

		for epoch := 0; epoch <= 2; epoch++ {
			committeeEpoch, err := newEngine.Storage.Committee().Load(iotago.EpochIndex(epoch))
			require.NoError(t, err)
			require.Equal(t, 4, committeeEpoch.SeatCount())
		}

		// Commmittee for the future epoch does not exist.
		committeeEpoch3, err := newEngine.Storage.Committee().Load(3)
		require.NoError(t, err)
		require.Nil(t, committeeEpoch3)

		for slot := 1; slot <= 13; slot++ {
			copiedCommitment, err := newEngine.Storage.Commitments().Load(iotago.SlotIndex(slot))
			require.NoError(t, err)
			sourceCommitment, err := node1.Protocol.Engines.Main.Get().Storage.Commitments().Load(iotago.SlotIndex(slot))
			require.NoError(t, err)
			require.Equal(t, sourceCommitment.ID(), copiedCommitment.ID())
		}

		// Commitment for the first slot after the fork does not exist.
		_, err = newEngine.Storage.Commitments().Load(iotago.SlotIndex(14))
		require.Error(t, err)
	}
}

func TestProtocol_EngineRollbackNoFinalizationLastSlot(t *testing.T) {
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
				3,
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

	poaProvider := func() module.Provider[*engine.Engine, seatmanager.SeatManager] {
		return module.Provide(func(e *engine.Engine) seatmanager.SeatManager {
			poa := mock2.NewManualPOAProvider()(e).(*mock2.ManualPOA)

			for _, node := range []*mock.Node{node0, node1, node2, node3} {
				if node.IsValidator() {
					poa.AddAccount(node.Validator.AccountData.ID, node.Name)
				}
			}
			poa.SetOnline("node0", "node1", "node2", "node3")

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
			protocol.WithStorageOptions(
				storage.WithPruningDelay(20),
			),
		}
	}

	ts.Run(true, nodeOptions)

	// Verify that nodes have the expected states.

	expectedCommittee := []iotago.AccountID{
		node0.Validator.AccountData.ID,
		node1.Validator.AccountData.ID,
		node2.Validator.AccountData.ID,
		node3.Validator.AccountData.ID,
	}
	expectedOnlineCommitteeFull := []account.SeatIndex{
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node0.Validator.AccountData.ID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node1.Validator.AccountData.ID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node2.Validator.AccountData.ID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node3.Validator.AccountData.ID)),
	}

	expectedOnlineCommitteeHalf := []account.SeatIndex{
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node0.Validator.AccountData.ID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node1.Validator.AccountData.ID)),
	}

	for _, node := range ts.Nodes() {
		node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().(*mock2.ManualPOA).SetOnline("node0", "node1", "node2", "node3")
	}

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
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommitteeFull...),
			testsuite.WithEvictedSlot(0),
			testsuite.WithActiveRootBlocks(ts.Blocks("Genesis")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis")),
		)
	}

	// Issue up to slot 11 - just before committee selection for the next epoch.
	// Committee will be reused at slot 10 is finalized or slot 12 is committed, whichever happens first.
	{
		ts.IssueBlocksAtSlots("P0:", []iotago.SlotIndex{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}, 4, "Genesis", ts.Nodes(), true, true)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(8),
			testsuite.WithLatestCommitmentSlotIndex(9),
			testsuite.WithEqualStoredCommitmentAtIndex(9),
			testsuite.WithLatestCommitmentCumulativeWeight(28), // 7 for each slot starting from 4
			testsuite.WithSybilProtectionCommittee(ts.API.TimeProvider().EpochFromSlot(9), expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommitteeFull...),
			testsuite.WithEvictedSlot(9),
		)

		for _, slot := range []iotago.SlotIndex{4, 5, 6, 7, 8, 9} {
			var attestationBlocks []*blocks.Block
			for _, node := range ts.Nodes() {
				if node.IsValidator() {
					attestationBlocks = append(attestationBlocks, ts.Block(fmt.Sprintf("P0:%d.3-%s", slot, node.Name)))
				}
			}
			ts.AssertAttestationsForSlot(slot, attestationBlocks, ts.Nodes()...)
		}

		ts.AssertBlocksExist(ts.BlocksWithPrefix("P0"), true, ts.ClientsForNodes()...)
	}

	// Update online committee.
	for _, node := range ts.Nodes() {
		manualPOA := node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().(*mock2.ManualPOA)
		manualPOA.SetOnline("node0", "node1")
		manualPOA.SetOffline("node2", "node3")
	}

	{
		ts.IssueBlocksAtSlots("P0:", []iotago.SlotIndex{12, 13, 14, 15, 16, 17, 18, 19}, 4, "P0:11.3", []*mock.Node{node0, node1}, true, true)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(8),
			testsuite.WithLatestCommitmentSlotIndex(17),
			testsuite.WithEqualStoredCommitmentAtIndex(17),
			testsuite.WithLatestCommitmentCumulativeWeight(48), // 7 for each slot starting from 4
			testsuite.WithSybilProtectionCommittee(ts.API.TimeProvider().EpochFromSlot(17), expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommitteeHalf...),
			testsuite.WithEvictedSlot(17),
		)

		ts.AssertBlocksExist(ts.BlocksWithPrefix("P0"), true, ts.ClientsForNodes()...)
	}

	newEngine, err := node3.Protocol.Engines.ForkAtSlot(15)
	require.NoError(t, err)

	// Assert state of the forked engine after rollback.
	{
		require.EqualValues(t, 15, newEngine.SyncManager.LatestCommitment().Slot())
		require.EqualValues(t, 8, newEngine.SyncManager.LatestFinalizedSlot())
		require.EqualValues(t, 15, newEngine.EvictionState.LastEvictedSlot())

		for epoch := 0; epoch <= 2; epoch++ {
			committeeEpoch, err := newEngine.Storage.Committee().Load(iotago.EpochIndex(epoch))
			require.NoError(t, err)
			require.Equal(t, 4, committeeEpoch.SeatCount())
		}

		// Commmittee for the future epoch does not exist.
		committeeEpoch3, err := newEngine.Storage.Committee().Load(3)
		require.NoError(t, err)
		require.Nil(t, committeeEpoch3)

		for slot := 1; slot <= 15; slot++ {
			copiedCommitment, err := newEngine.Storage.Commitments().Load(iotago.SlotIndex(slot))
			require.NoError(t, err)
			sourceCommitment, err := node1.Protocol.Engines.Main.Get().Storage.Commitments().Load(iotago.SlotIndex(slot))
			require.NoError(t, err)
			require.Equal(t, sourceCommitment.ID(), copiedCommitment.ID())
		}

		// Commitment for the first slot after the fork does not exist.
		_, err = newEngine.Storage.Commitments().Load(iotago.SlotIndex(16))
		require.Error(t, err)
	}
}

func TestProtocol_EngineRollbackNoFinalizationBeforePointOfNoReturn(t *testing.T) {
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
				3,
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

	poaProvider := func() module.Provider[*engine.Engine, seatmanager.SeatManager] {
		return module.Provide(func(e *engine.Engine) seatmanager.SeatManager {
			poa := mock2.NewManualPOAProvider()(e).(*mock2.ManualPOA)

			for _, node := range []*mock.Node{node0, node1, node2, node3} {
				if node.IsValidator() {
					poa.AddAccount(node.Validator.AccountData.ID, node.Name)
				}
			}
			poa.SetOnline("node0", "node1", "node2", "node3")

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
			protocol.WithStorageOptions(
				storage.WithPruningDelay(20),
			),
		}
	}

	ts.Run(false, nodeOptions)

	// Verify that nodes have the expected states.

	expectedCommittee := []iotago.AccountID{
		node0.Validator.AccountData.ID,
		node1.Validator.AccountData.ID,
		node2.Validator.AccountData.ID,
		node3.Validator.AccountData.ID,
	}
	expectedOnlineCommitteeFull := []account.SeatIndex{
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node0.Validator.AccountData.ID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node1.Validator.AccountData.ID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node2.Validator.AccountData.ID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node3.Validator.AccountData.ID)),
	}

	expectedOnlineCommitteeHalf := []account.SeatIndex{
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node0.Validator.AccountData.ID)),
		lo.Return1(lo.Return1(node0.Protocol.Engines.Main.Get().SybilProtection.SeatManager().CommitteeInSlot(1)).GetSeat(node1.Validator.AccountData.ID)),
	}

	for _, node := range ts.Nodes() {
		node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().(*mock2.ManualPOA).SetOnline("node0", "node1", "node2", "node3")
	}

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
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommitteeFull...),
			testsuite.WithEvictedSlot(0),
			testsuite.WithActiveRootBlocks(ts.Blocks("Genesis")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis")),
		)
	}

	// Issue up to slot 11 - just before committee selection for the next epoch.
	// Committee will be reused at slot 10 is finalized or slot 12 is committed, whichever happens first.
	{
		ts.IssueBlocksAtSlots("P0:", []iotago.SlotIndex{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}, 4, "Genesis", ts.Nodes(), true, false)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(8),
			testsuite.WithLatestCommitmentSlotIndex(9),
			testsuite.WithEqualStoredCommitmentAtIndex(9),
			testsuite.WithLatestCommitmentCumulativeWeight(28), // 7 for each slot starting from 4
			testsuite.WithSybilProtectionCommittee(ts.API.TimeProvider().EpochFromSlot(9), expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommitteeFull...),
			testsuite.WithEvictedSlot(9),
		)

		for _, slot := range []iotago.SlotIndex{4, 5, 6, 7, 8, 9} {
			var attestationBlocks []*blocks.Block
			for _, node := range ts.Nodes() {
				if node.IsValidator() {
					attestationBlocks = append(attestationBlocks, ts.Block(fmt.Sprintf("P0:%d.3-%s", slot, node.Name)))
				}
			}
			ts.AssertAttestationsForSlot(slot, attestationBlocks, ts.Nodes()...)
		}

		ts.AssertBlocksExist(ts.BlocksWithPrefix("P0"), true, ts.ClientsForNodes()...)
	}

	// Update online committee.
	for _, node := range ts.Nodes() {
		manualPOA := node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().(*mock2.ManualPOA)
		manualPOA.SetOnline("node0", "node1")
		manualPOA.SetOffline("node2", "node3")
	}

	{
		ts.IssueBlocksAtSlots("P0:", []iotago.SlotIndex{12, 13, 14, 15}, 4, "P0:11.3", []*mock.Node{node0, node1}, true, false)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(8),
			testsuite.WithLatestCommitmentSlotIndex(13),
			testsuite.WithEqualStoredCommitmentAtIndex(13),
			testsuite.WithLatestCommitmentCumulativeWeight(42), // 7 for each slot starting from 4
			testsuite.WithSybilProtectionCommittee(ts.API.TimeProvider().EpochFromSlot(13), expectedCommittee),
			testsuite.WithSybilProtectionOnlineCommittee(expectedOnlineCommitteeHalf...),
			testsuite.WithEvictedSlot(13),
		)

		ts.AssertBlocksExist(ts.BlocksWithPrefix("P0"), true, ts.ClientsForNodes()...)
	}

	newEngine, err := node3.Protocol.Engines.ForkAtSlot(9)
	require.NoError(t, err)

	// Assert state of the forked engine after rollback.
	{
		require.EqualValues(t, 9, newEngine.SyncManager.LatestCommitment().Slot())
		require.EqualValues(t, 8, newEngine.SyncManager.LatestFinalizedSlot())
		require.EqualValues(t, 9, newEngine.EvictionState.LastEvictedSlot())

		for epoch := 0; epoch <= 1; epoch++ {
			committeeEpoch, err := newEngine.Storage.Committee().Load(iotago.EpochIndex(epoch))
			require.NoError(t, err)
			require.Equal(t, 4, committeeEpoch.SeatCount())
		}

		// Committee for the future epoch does not exist.
		committeeEpoch2, err := newEngine.Storage.Committee().Load(2)
		require.NoError(t, err)
		require.Nil(t, committeeEpoch2)

		for slot := 1; slot <= 9; slot++ {
			copiedCommitment, err := newEngine.Storage.Commitments().Load(iotago.SlotIndex(slot))
			require.NoError(t, err)
			sourceCommitment, err := node1.Protocol.Engines.Main.Get().Storage.Commitments().Load(iotago.SlotIndex(slot))
			require.NoError(t, err)
			require.Equal(t, sourceCommitment.ID(), copiedCommitment.ID())
		}

		// Commitment for the first slot after the fork does not exist.
		_, err = newEngine.Storage.Commitments().Load(iotago.SlotIndex(10))
		require.Error(t, err)
	}
}

// TODO: test fork before point of no return (slot 12)
// TODO: test fork on last slot of an epoch (slot 15)
