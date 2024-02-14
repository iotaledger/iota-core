package tests

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/runtime/memanalyzer"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
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

func TestProtocol_Eviction(t *testing.T) {
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

	node := ts.AddValidatorNode("node0")

	ts.Run(false, map[string][]options.Option[protocol.Protocol]{
		"node0": []options.Option[protocol.Protocol]{
			protocol.WithSybilProtectionProvider(
				sybilprotectionv1.NewProvider(
					sybilprotectionv1.WithSeatManagerProvider(module.Provide(func(e *engine.Engine) seatmanager.SeatManager {
						poa := mock2.NewManualPOAProvider()(e).(*mock2.ManualPOA)
						poa.AddAccount(node.Validator.AccountID, node.Name)

						onlineValidators := ds.NewSet[string]()

						e.Constructed.OnTrigger(func() {
							e.Events.BlockDAG.BlockAttached.Hook(func(block *blocks.Block) {
								if block.ModelBlock().ProtocolBlock().Header.IssuerID == node.Validator.AccountID && onlineValidators.Add(node.Name) {
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
						return e.Notarization.IsBootstrapped()
					}),
				),
			),

			protocol.WithStorageOptions(
				storage.WithPruningDelay(20),
			),
		},
	})

	node.Protocol.Engines.Main.Get().SybilProtection.SeatManager().(*mock2.ManualPOA).SetOnline("node0")

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

			testsuite.WithSybilProtectionCommittee(0, []iotago.AccountID{node.Validator.AccountID}),
			testsuite.WithEvictedSlot(0),
			testsuite.WithActiveRootBlocks(ts.Blocks("Genesis")),
			testsuite.WithStorageRootBlocks(ts.Blocks("Genesis")),
		)
	}

	issueBlocks := func(slots []iotago.SlotIndex) {
		parentSlot := slots[0] - 1
		lastIssuedSlot := slots[len(slots)-1]
		lastCommittedSlot := lastIssuedSlot - minCommittableAge

		initialParentsPrefix := "P0:" + strconv.Itoa(int(parentSlot)) + ".3"
		if parentSlot == genesisSlot {
			initialParentsPrefix = "Genesis"
		}

		ts.IssueBlocksAtSlots("P0:", slots, 4, initialParentsPrefix, []*mock.Node{node}, true, true)

		cumulativeAttestations := uint64(0)
		for slot := genesisSlot + maxCommittableAge; slot <= lastCommittedSlot; slot++ {
			var attestationBlocks Blocks
			attestationBlocks.Add(ts, node, 0, slot)

			cumulativeAttestations++

			ts.AssertAttestationsForSlot(slot, attestationBlocks, node)
		}

		ts.AssertNodeState([]*mock.Node{node},
			testsuite.WithLatestFinalizedSlot(lastCommittedSlot-1),
			testsuite.WithLatestCommitmentSlotIndex(lastCommittedSlot),
			testsuite.WithEqualStoredCommitmentAtIndex(lastCommittedSlot),
			testsuite.WithLatestCommitmentCumulativeWeight(cumulativeAttestations),
			testsuite.WithSybilProtectionCommittee(ts.API.TimeProvider().EpochFromSlot(lastCommittedSlot), []iotago.AccountID{node.Validator.AccountID}),
			testsuite.WithEvictedSlot(lastCommittedSlot),
		)

		var tipBlocks Blocks
		tipBlocks.Add(ts, node, 0, lastIssuedSlot)

		ts.AssertStrongTips(tipBlocks, node)
	}

	// issue blocks until we evict the first slot
	issueBlocks([]iotago.SlotIndex{1, 2, 3, 4, 5, 6, 7, 8})

	memConsumptionStart := memConsumption(node)
	fmt.Println(memConsumptionStart)

	// issue more blocks
	issueBlocks([]iotago.SlotIndex{9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35})

	memConsumptionEnd := memConsumption(node)
	fmt.Println(memConsumptionEnd)

	// make sure the memory does not grow by more than 5%
	for key, memStart := range memConsumptionStart {
		require.Less(t, float64(memConsumptionEnd[key]), 1.05*float64(memStart), key+" should not grow by more than 5%")
	}
}

func memConsumption(node *mock.Node) map[string]uintptr {
	return map[string]uintptr{
		"Engine":   memanalyzer.MemSize(node.Protocol.Engines.Main.Get()),
		"Protocol": memanalyzer.MemSize(node.Protocol),
	}
}
