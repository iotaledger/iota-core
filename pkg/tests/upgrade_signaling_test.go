package tests

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/upgrade/signalingupgradeorchestrator"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/storage"
	"github.com/iotaledger/iota-core/pkg/storage/permanent"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func Test_Upgrade_Signaling(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithLivenessThreshold(1),
		testsuite.WithMinCommittableAge(2),
		testsuite.WithMaxCommittableAge(6),
		testsuite.WithEpochNearingThreshold(2),
		testsuite.WithSlotsPerEpochExponent(3),
		testsuite.WithGenesisTimestampOffset(1000*10),
	)
	defer ts.Shutdown()

	nodeOptions := []options.Option[protocol.Protocol]{
		protocol.WithChainManagerOptions(
			chainmanager.WithCommitmentRequesterOptions(
				eventticker.RetryInterval[iotago.SlotIndex, iotago.CommitmentID](1*time.Second),
				eventticker.RetryJitter[iotago.SlotIndex, iotago.CommitmentID](500*time.Millisecond),
			),
		),
		protocol.WithEngineOptions(
			engine.WithBlockRequesterOptions(
				eventticker.RetryInterval[iotago.SlotIndex, iotago.BlockID](1*time.Second),
				eventticker.RetryJitter[iotago.SlotIndex, iotago.BlockID](100*time.Millisecond),
			),
		),
		protocol.WithStorageOptions(
			storage.WithPruningDelay(20),
			storage.WithPermanentOptions(
				permanent.WithEpochBasedProviderOptions(
					api.WithAPIForMissingVersionCallback(func(version iotago.Version) (iotago.API, error) {
						return ts.API, nil
					}),
				),
			),
		),
	}

	ts.AddValidatorNode("nodeA")
	ts.AddValidatorNode("nodeB")
	ts.AddValidatorNode("nodeC")
	ts.AddValidatorNode("nodeD")
	ts.AddNode("nodeE")
	ts.AddNode("nodeF")

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{
		"nodeA": nodeOptions,
		"nodeB": nodeOptions,
		"nodeC": nodeOptions,
		"nodeD": nodeOptions,
		"nodeE": nodeOptions,
		"nodeF": nodeOptions,
	})

	ts.Wait()

	v5ProtocolParameters := iotago.NewV3ProtocolParameters(
		iotago.WithVersion(5),
		iotago.WithTimeProviderOptions(
			ts.API.TimeProvider().GenesisUnixTime(),
			uint8(ts.API.TimeProvider().SlotDurationSeconds()),
			uint8(math.Log2(float64(ts.API.TimeProvider().EpochDurationSlots()))),
		),
	)

	hash1 := lo.PanicOnErr(v5ProtocolParameters.Hash())
	hash2 := iotago.Identifier{2}

	ts.AssertAccountData(&accounts.AccountData{
		ID:                                    ts.Node("nodeA").AccountID,
		Credits:                               &accounts.BlockIssuanceCredits{Value: math.MaxInt64, UpdateTime: 0},
		ExpirySlot:                            math.MaxUint64,
		OutputID:                              iotago.OutputIDFromTransactionIDAndIndex(snapshotcreator.GenesisTransactionID, 1),
		BlockIssuerKeys:                       ds.NewSet[iotago.BlockIssuerKey](iotago.BlockIssuerKeyEd25519FromPublicKey(ed25519.PublicKey(ts.Node("nodeA").PubKey))),
		ValidatorStake:                        testsuite.MinValidatorAccountAmount,
		DelegationStake:                       0,
		FixedCost:                             0,
		StakeEndEpoch:                         math.MaxUint64,
		LatestSupportedProtocolVersionAndHash: model.VersionAndHash{},
	}, ts.Nodes()...)

	ts.AssertAccountData(&accounts.AccountData{
		ID:                                    ts.Node("nodeF").AccountID,
		Credits:                               &accounts.BlockIssuanceCredits{Value: math.MaxInt64, UpdateTime: 0},
		ExpirySlot:                            math.MaxUint64,
		OutputID:                              iotago.OutputIDFromTransactionIDAndIndex(snapshotcreator.GenesisTransactionID, 6),
		BlockIssuerKeys:                       ds.NewSet[iotago.BlockIssuerKey](iotago.BlockIssuerKeyEd25519FromPublicKey(ed25519.PublicKey(ts.Node("nodeF").PubKey))),
		ValidatorStake:                        0,
		DelegationStake:                       0,
		FixedCost:                             0,
		StakeEndEpoch:                         0,
		LatestSupportedProtocolVersionAndHash: model.VersionAndHash{},
	}, ts.Nodes()...)

	ts.Node("nodeA").SetHighestSupportedVersion(4)
	ts.Node("nodeA").SetProtocolParametersHash(hash2)
	ts.Node("nodeB").SetHighestSupportedVersion(5)
	ts.Node("nodeB").SetProtocolParametersHash(hash1)
	ts.Node("nodeC").SetHighestSupportedVersion(5)
	ts.Node("nodeC").SetProtocolParametersHash(hash1)
	ts.Node("nodeD").SetHighestSupportedVersion(3)
	ts.Node("nodeD").SetProtocolParametersHash(hash2)
	ts.IssueBlocksAtEpoch("", 0, 4, "Genesis", ts.Nodes(), true, nil)

	// check account data before all nodes set the current version
	ts.AssertAccountData(&accounts.AccountData{
		ID:                                    ts.Node("nodeA").AccountID,
		Credits:                               &accounts.BlockIssuanceCredits{Value: math.MaxInt64, UpdateTime: 0},
		ExpirySlot:                            math.MaxUint64,
		OutputID:                              iotago.OutputIDFromTransactionIDAndIndex(snapshotcreator.GenesisTransactionID, 1),
		BlockIssuerKeys:                       ds.NewSet[iotago.BlockIssuerKey](iotago.BlockIssuerKeyEd25519FromPublicKey(ed25519.PublicKey(ts.Node("nodeA").PubKey))),
		ValidatorStake:                        testsuite.MinValidatorAccountAmount,
		DelegationStake:                       0,
		FixedCost:                             0,
		StakeEndEpoch:                         math.MaxUint64,
		LatestSupportedProtocolVersionAndHash: model.VersionAndHash{Version: 4, Hash: hash2},
	}, ts.Nodes()...)

	ts.AssertAccountData(&accounts.AccountData{
		ID:                                    ts.Node("nodeD").AccountID,
		Credits:                               &accounts.BlockIssuanceCredits{Value: math.MaxInt64, UpdateTime: 0},
		ExpirySlot:                            math.MaxUint64,
		OutputID:                              iotago.OutputIDFromTransactionIDAndIndex(snapshotcreator.GenesisTransactionID, 4),
		BlockIssuerKeys:                       ds.NewSet[iotago.BlockIssuerKey](iotago.BlockIssuerKeyEd25519FromPublicKey(ed25519.PublicKey(ts.Node("nodeD").PubKey))),
		ValidatorStake:                        testsuite.MinValidatorAccountAmount,
		DelegationStake:                       0,
		FixedCost:                             0,
		StakeEndEpoch:                         math.MaxUint64,
		LatestSupportedProtocolVersionAndHash: model.VersionAndHash{Version: 3, Hash: hash2},
	}, ts.Nodes()...)

	// update the latest supported version for the remaining nodes
	ts.Node("nodeA").SetHighestSupportedVersion(5)
	ts.Node("nodeA").SetProtocolParametersHash(hash1)
	ts.Node("nodeD").SetHighestSupportedVersion(5)
	ts.Node("nodeD").SetProtocolParametersHash(hash2)

	ts.IssueBlocksAtEpoch("", 1, 4, "7.3", ts.Nodes(), true, nil)

	ts.AssertAccountData(&accounts.AccountData{
		ID:                                    ts.Node("nodeA").AccountID,
		Credits:                               &accounts.BlockIssuanceCredits{Value: math.MaxInt64, UpdateTime: 0},
		ExpirySlot:                            math.MaxUint64,
		OutputID:                              iotago.OutputIDFromTransactionIDAndIndex(snapshotcreator.GenesisTransactionID, 1),
		BlockIssuerKeys:                       ds.NewSet[iotago.BlockIssuerKey](iotago.BlockIssuerKeyEd25519FromPublicKey(ed25519.PublicKey(ts.Node("nodeA").PubKey))),
		ValidatorStake:                        testsuite.MinValidatorAccountAmount,
		DelegationStake:                       0,
		FixedCost:                             0,
		StakeEndEpoch:                         math.MaxUint64,
		LatestSupportedProtocolVersionAndHash: model.VersionAndHash{Version: 5, Hash: hash1},
	}, ts.Nodes()...)

	// check that rollback is correct
	account, exists, err := ts.Node("nodeA").Protocol.MainEngineInstance().Ledger.Account(ts.Node("nodeA").AccountID, 7)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, model.VersionAndHash{Version: 4, Hash: hash2}, account.LatestSupportedProtocolVersionAndHash)

	ts.IssueBlocksAtEpoch("", 2, 4, "15.3", ts.Nodes(), true, nil)
	ts.IssueBlocksAtEpoch("", 3, 4, "23.3", ts.Nodes(), true, nil)

	// Epoch 5: revoke vote of nodeA in last slot of epoch.
	ts.IssueBlocksAtSlots("", ts.SlotsForEpoch(4)[:ts.API.TimeProvider().EpochDurationSlots()-1], 4, "31.3", ts.Nodes(), true, nil)

	ts.Node("nodeA").SetProtocolParametersHash(iotago.Identifier{})

	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{39}, 4, "38.3", ts.Nodes(), true, nil)

	ts.Node("nodeA").SetProtocolParametersHash(hash1)

	// Epoch 6: issue half before restarting and half after restarting.
	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{40, 41, 42, 43}, 4, "39.3", ts.Nodes(), true, nil)

	{
		var expectedRootBlocks []*blocks.Block
		for _, slot := range []iotago.SlotIndex{39, 40, 41} {
			expectedRootBlocks = append(expectedRootBlocks, ts.BlocksWithPrefix(fmt.Sprintf("%d.3-", slot))...)
		}

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestCommitmentSlotIndex(41),
			testsuite.WithActiveRootBlocks(expectedRootBlocks),
		)

		// Shutdown nodeE and restart it from disk. Verify state.
		{
			nodeE := ts.Node("nodeE")
			nodeE.Shutdown()
			ts.RemoveNode("nodeE")

			nodeE1 := ts.AddNode("nodeE1")
			nodeE1.CopyIdentityFromNode(nodeE)
			nodeE1.Initialize(true,
				append(nodeOptions,
					protocol.WithBaseDirectory(ts.Directory.Path(nodeE.Name)),
				)...,
			)
			ts.Wait()
		}

		// Create snapshot.
		snapshotPath := ts.Directory.Path(fmt.Sprintf("%d_snapshot", time.Now().Unix()))
		require.NoError(t, ts.Node("nodeA").Protocol.MainEngineInstance().WriteSnapshot(snapshotPath))

		{
			nodeG := ts.AddNode("nodeG")
			nodeG.Initialize(true,
				append(nodeOptions,
					protocol.WithSnapshotPath(snapshotPath),
					protocol.WithBaseDirectory(ts.Directory.PathWithCreate(nodeG.Name)),
				)...,
			)
			ts.Wait()

			ts.AssertNodeState(ts.Nodes(),
				testsuite.WithLatestCommitmentSlotIndex(41),
				testsuite.WithActiveRootBlocks(expectedRootBlocks),
			)
		}
	}

	// Can only continue to issue on nodeA, nodeB, nodeC, nodeD, nodeF. nodeE and nodeG were just restarted and don't have the latest unaccepted state.
	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{44}, 4, "43.3", ts.Nodes("nodeA", "nodeB", "nodeC", "nodeD", "nodeF"), true, nil)

	// TODO: would be great to dynamically add accounts for later nodes.
	// Can't issue on nodeG as its account is not known.
	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{45, 46, 47}, 4, "44.3", ts.Nodes("nodeA", "nodeB", "nodeC", "nodeD", "nodeF", "nodeE1"), true, nil)

	ts.IssueBlocksAtEpoch("", 6, 4, "47.3", ts.Nodes("nodeA", "nodeB", "nodeC", "nodeD", "nodeF", "nodeE1"), true, nil)
	ts.IssueBlocksAtEpoch("", 7, 4, "55.3", ts.Nodes("nodeA", "nodeB", "nodeC", "nodeD", "nodeF", "nodeE1"), true, nil)

	// Restart node (and add protocol parameters) and add another node from snapshot (also with protocol parameters already set).
	{
		var expectedRootBlocks []*blocks.Block
		for _, slot := range []iotago.SlotIndex{59, 60, 61} {
			expectedRootBlocks = append(expectedRootBlocks, ts.BlocksWithPrefix(fmt.Sprintf("%d.3-", slot))...)
		}

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestCommitmentSlotIndex(61),
			testsuite.WithActiveRootBlocks(expectedRootBlocks),
		)

		// Shutdown nodeE1 and restart it from disk as nodeE2. Verify state.
		{
			nodeE1 := ts.Node("nodeE1")
			nodeE1.Shutdown()
			ts.RemoveNode("nodeE1")

			nodeE2 := ts.AddNode("nodeE2")
			nodeE2.CopyIdentityFromNode(nodeE1)
			nodeE2.Initialize(true,
				append(nodeOptions,
					protocol.WithBaseDirectory(ts.Directory.Path("nodeE")),
					protocol.WithUpgradeOrchestratorProvider(
						signalingupgradeorchestrator.NewProvider(signalingupgradeorchestrator.WithProtocolParameters(v5ProtocolParameters)),
					),
				)...,
			)
			ts.Wait()
		}

		// Create snapshot and start new nodeH from it.
		{
			snapshotPath := ts.Directory.Path(fmt.Sprintf("%d_snapshot", time.Now().Unix()))
			require.NoError(t, ts.Node("nodeE2").Protocol.MainEngineInstance().WriteSnapshot(snapshotPath))

			nodeG := ts.AddNode("nodeH")
			nodeG.Initialize(true,
				append(nodeOptions,
					protocol.WithSnapshotPath(snapshotPath),
					protocol.WithBaseDirectory(ts.Directory.PathWithCreate(nodeG.Name)),
					protocol.WithUpgradeOrchestratorProvider(
						signalingupgradeorchestrator.NewProvider(signalingupgradeorchestrator.WithProtocolParameters(v5ProtocolParameters)),
					),
				)...,
			)
			ts.Wait()
		}

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestCommitmentSlotIndex(61),
			testsuite.WithEqualStoredCommitmentAtIndex(61),

			testsuite.WithActiveRootBlocks(expectedRootBlocks),
		)
	}

	// Verify final state of all nodes.
	{
		ts.AssertEpochVersions(map[iotago.Version]iotago.EpochIndex{
			3: 0,
			5: 13,
		}, ts.Nodes()...)

		ts.AssertVersionAndProtocolParameters(map[iotago.Version]iotago.ProtocolParameters{
			3: ts.API.ProtocolParameters(),
			5: nil,
		}, ts.Nodes("nodeA", "nodeB", "nodeC", "nodeD", "nodeF", "nodeG")...)

		// We've started nodeH with the v5 protocol parameters set. Therefore, they should be available.
		ts.AssertVersionAndProtocolParameters(map[iotago.Version]iotago.ProtocolParameters{
			3: ts.API.ProtocolParameters(),
			5: v5ProtocolParameters,
		}, ts.Nodes("nodeE2", "nodeH")...)

		ts.AssertVersionAndProtocolParametersHashes(map[iotago.Version]iotago.Identifier{
			3: lo.PanicOnErr(ts.API.ProtocolParameters().Hash()),
			5: hash1,
		}, ts.Nodes()...)

		// check account data at the end of the test
		ts.AssertAccountData(&accounts.AccountData{
			ID:                                    ts.Node("nodeA").AccountID,
			Credits:                               &accounts.BlockIssuanceCredits{Value: math.MaxInt64, UpdateTime: 0},
			ExpirySlot:                            math.MaxUint64,
			OutputID:                              iotago.OutputIDFromTransactionIDAndIndex(snapshotcreator.GenesisTransactionID, 1),
			BlockIssuerKeys:                       ds.NewSet[iotago.BlockIssuerKey](iotago.BlockIssuerKeyEd25519FromPublicKey(ed25519.PublicKey(ts.Node("nodeA").PubKey))),
			ValidatorStake:                        testsuite.MinValidatorAccountAmount,
			DelegationStake:                       0,
			FixedCost:                             0,
			StakeEndEpoch:                         math.MaxUint64,
			LatestSupportedProtocolVersionAndHash: model.VersionAndHash{Version: 5, Hash: hash1},
		}, ts.Nodes()...)

		ts.AssertAccountData(&accounts.AccountData{
			ID:                                    ts.Node("nodeD").AccountID,
			Credits:                               &accounts.BlockIssuanceCredits{Value: math.MaxInt64, UpdateTime: 0},
			ExpirySlot:                            math.MaxUint64,
			OutputID:                              iotago.OutputIDFromTransactionIDAndIndex(snapshotcreator.GenesisTransactionID, 4),
			BlockIssuerKeys:                       ds.NewSet[iotago.BlockIssuerKey](iotago.BlockIssuerKeyEd25519FromPublicKey(ed25519.PublicKey(ts.Node("nodeD").PubKey))),
			ValidatorStake:                        testsuite.MinValidatorAccountAmount,
			DelegationStake:                       0,
			FixedCost:                             0,
			StakeEndEpoch:                         math.MaxUint64,
			LatestSupportedProtocolVersionAndHash: model.VersionAndHash{Version: 5, Hash: hash2},
		}, ts.Nodes()...)
	}

	// Check that issuing still produces the same commitments
	ts.IssueBlocksAtEpoch("", 8, 4, "63.3", ts.Nodes("nodeA", "nodeB", "nodeC", "nodeD", "nodeF"), true, nil)
	ts.AssertNodeState(ts.Nodes(),
		testsuite.WithLatestCommitmentSlotIndex(69),
		testsuite.WithEqualStoredCommitmentAtIndex(69),
	)
}
