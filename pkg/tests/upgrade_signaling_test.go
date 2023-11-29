package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/upgrade/signalingupgradeorchestrator"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager/topstakers"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1"
	"github.com/iotaledger/iota-core/pkg/storage"
	"github.com/iotaledger/iota-core/pkg/storage/permanent"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func Test_Upgrade_Signaling(t *testing.T) {
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
			iotago.WithVersionSignalingOptions(7, 5, 2),
		),
	)
	defer ts.Shutdown()

	// We "pretend" to have version 5 but reuse the same protocol parameters as for version 3.
	v5ProtocolParameters := iotago.NewV3ProtocolParameters(
		append(
			ts.ProtocolParameterOptions,
			iotago.WithVersion(5),
		)...,
	)

	nodeOptionsWithoutV5 := []options.Option[protocol.Protocol]{
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
					iotago.WithAPIForMissingVersionCallback(func(protocolParameters iotago.ProtocolParameters) (iotago.API, error) {
						switch protocolParameters.Version() {
						case 3:
							return ts.API, nil
						}

						return nil, ierrors.Errorf("can't create API due to unsupported protocol version: %d", protocolParameters.Version())
					}),
				),
			),
		),
		protocol.WithSybilProtectionProvider(
			sybilprotectionv1.NewProvider(
				sybilprotectionv1.WithSeatManagerProvider(
					topstakers.NewProvider(
						// We need to make sure that inactive nodes are evicted from the committee to continue acceptance.
						topstakers.WithActivityWindow(15 * time.Second),
					),
				),
			),
		),
	}

	nodeOptionsWithV5 := append(nodeOptionsWithoutV5,
		protocol.WithUpgradeOrchestratorProvider(
			signalingupgradeorchestrator.NewProvider(signalingupgradeorchestrator.WithProtocolParameters(v5ProtocolParameters)),
		),
		protocol.WithStorageOptions(
			storage.WithPruningDelay(20),
			storage.WithPermanentOptions(
				permanent.WithEpochBasedProviderOptions(
					iotago.WithAPIForMissingVersionCallback(func(protocolParameters iotago.ProtocolParameters) (iotago.API, error) {
						switch protocolParameters.Version() {
						case 3:
							return ts.API, nil
						case 5:
							return iotago.V3API(v5ProtocolParameters), nil
						}

						return nil, ierrors.Errorf("can't create API due to unsupported protocol version: %d", protocolParameters.Version())
					}),
				),
			),
		),
	)

	nodeA := ts.AddValidatorNode("nodeA")
	ts.AddValidatorNode("nodeB")
	ts.AddValidatorNode("nodeC")
	ts.AddValidatorNode("nodeD")
	ts.AddNode("nodeE")
	ts.AddNode("nodeF")
	wallet := ts.AddDefaultWallet(nodeA)

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{
		"nodeA": nodeOptionsWithoutV5,
		"nodeB": nodeOptionsWithV5,
		"nodeC": nodeOptionsWithV5,
		"nodeD": nodeOptionsWithoutV5,
		"nodeE": nodeOptionsWithoutV5,
		"nodeF": nodeOptionsWithoutV5,
	})

	ts.Wait()

	hash1 := lo.PanicOnErr(v5ProtocolParameters.Hash())
	hash2 := iotago.Identifier{2}

	ts.AssertAccountData(&accounts.AccountData{
		ID:                                    ts.Node("nodeA").Validator.AccountID,
		Credits:                               &accounts.BlockIssuanceCredits{Value: iotago.MaxBlockIssuanceCredits / 2, UpdateSlot: 0},
		ExpirySlot:                            iotago.MaxSlotIndex,
		OutputID:                              ts.AccountOutput("Genesis:1").OutputID(),
		BlockIssuerKeys:                       iotago.NewBlockIssuerKeys(iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(ts.Node("nodeA").Validator.PublicKey))),
		ValidatorStake:                        mock.MinValidatorAccountAmount(ts.API.ProtocolParameters()),
		DelegationStake:                       0,
		FixedCost:                             0,
		StakeEndEpoch:                         iotago.MaxEpochIndex,
		LatestSupportedProtocolVersionAndHash: model.VersionAndHash{},
	}, ts.Nodes()...)

	ts.AssertAccountData(&accounts.AccountData{
		ID:                                    wallet.BlockIssuer.AccountID,
		Credits:                               &accounts.BlockIssuanceCredits{Value: iotago.MaxBlockIssuanceCredits / 2, UpdateSlot: 0},
		ExpirySlot:                            iotago.MaxSlotIndex,
		OutputID:                              ts.AccountOutput("Genesis:5").OutputID(),
		BlockIssuerKeys:                       iotago.NewBlockIssuerKeys(iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(wallet.BlockIssuer.PublicKey))),
		ValidatorStake:                        0,
		DelegationStake:                       0,
		FixedCost:                             0,
		StakeEndEpoch:                         0,
		LatestSupportedProtocolVersionAndHash: model.VersionAndHash{},
	}, ts.Nodes()...)

	// We force the nodes to issue at a specific version/hash to test tracking of votes for the upgrade signaling.
	ts.Node("nodeA").SetHighestSupportedVersion(4)
	ts.Node("nodeA").SetProtocolParametersHash(hash2)
	ts.Node("nodeD").SetHighestSupportedVersion(3)
	ts.Node("nodeD").SetProtocolParametersHash(hash2)
	ts.IssueBlocksAtEpoch("", 0, 4, "Genesis", ts.Nodes(), true, false)

	// check account data before all nodes set the current version
	ts.AssertAccountData(&accounts.AccountData{
		ID:                                    ts.Node("nodeA").Validator.AccountID,
		Credits:                               &accounts.BlockIssuanceCredits{Value: iotago.MaxBlockIssuanceCredits / 2, UpdateSlot: 0},
		ExpirySlot:                            iotago.MaxSlotIndex,
		OutputID:                              ts.AccountOutput("Genesis:1").OutputID(),
		BlockIssuerKeys:                       iotago.NewBlockIssuerKeys(iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(ts.Node("nodeA").Validator.PublicKey))),
		ValidatorStake:                        mock.MinValidatorAccountAmount(ts.API.ProtocolParameters()),
		DelegationStake:                       0,
		FixedCost:                             0,
		StakeEndEpoch:                         iotago.MaxEpochIndex,
		LatestSupportedProtocolVersionAndHash: model.VersionAndHash{Version: 4, Hash: hash2},
	}, ts.Nodes()...)

	ts.AssertAccountData(&accounts.AccountData{
		ID:                                    ts.Node("nodeD").Validator.AccountID,
		Credits:                               &accounts.BlockIssuanceCredits{Value: iotago.MaxBlockIssuanceCredits / 2, UpdateSlot: 0},
		ExpirySlot:                            iotago.MaxSlotIndex,
		OutputID:                              ts.AccountOutput("Genesis:4").OutputID(),
		BlockIssuerKeys:                       iotago.NewBlockIssuerKeys(iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(ts.Node("nodeD").Validator.PublicKey))),
		ValidatorStake:                        mock.MinValidatorAccountAmount(ts.API.ProtocolParameters()),
		DelegationStake:                       0,
		FixedCost:                             0,
		StakeEndEpoch:                         iotago.MaxEpochIndex,
		LatestSupportedProtocolVersionAndHash: model.VersionAndHash{Version: 3, Hash: hash2},
	}, ts.Nodes()...)

	// update the latest supported version for the remaining nodes
	ts.Node("nodeA").SetHighestSupportedVersion(5)
	ts.Node("nodeA").SetProtocolParametersHash(hash1)
	ts.Node("nodeD").SetHighestSupportedVersion(5)
	ts.Node("nodeD").SetProtocolParametersHash(hash2)

	ts.IssueBlocksAtEpoch("", 1, 4, "7.3", ts.Nodes(), true, false)

	ts.AssertAccountData(&accounts.AccountData{
		ID:                                    ts.Node("nodeA").Validator.AccountID,
		Credits:                               &accounts.BlockIssuanceCredits{Value: iotago.MaxBlockIssuanceCredits / 2, UpdateSlot: 0},
		ExpirySlot:                            iotago.MaxSlotIndex,
		OutputID:                              ts.AccountOutput("Genesis:1").OutputID(),
		BlockIssuerKeys:                       iotago.NewBlockIssuerKeys(iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(ts.Node("nodeA").Validator.PublicKey))),
		ValidatorStake:                        mock.MinValidatorAccountAmount(ts.API.ProtocolParameters()),
		DelegationStake:                       0,
		FixedCost:                             0,
		StakeEndEpoch:                         iotago.MaxEpochIndex,
		LatestSupportedProtocolVersionAndHash: model.VersionAndHash{Version: 5, Hash: hash1},
	}, ts.Nodes()...)

	// check that rollback is correct
	pastAccounts, err := ts.Node("nodeA").Protocol.MainEngineInstance().Ledger.PastAccounts(iotago.AccountIDs{ts.Node("nodeA").Validator.AccountID}, 7)
	require.NoError(t, err)
	require.Contains(t, pastAccounts, ts.Node("nodeA").Validator.AccountID)
	require.Equal(t, model.VersionAndHash{Version: 4, Hash: hash2}, pastAccounts[ts.Node("nodeA").Validator.AccountID].LatestSupportedProtocolVersionAndHash)

	ts.IssueBlocksAtEpoch("", 2, 4, "15.3", ts.Nodes(), true, false)
	ts.IssueBlocksAtEpoch("", 3, 4, "23.3", ts.Nodes(), true, false)

	// Epoch 5: revoke vote of nodeA in last slot of epoch.
	ts.IssueBlocksAtSlots("", ts.SlotsForEpoch(4)[:ts.API.TimeProvider().EpochDurationSlots()-1], 4, "31.3", ts.Nodes(), true, false)

	ts.Node("nodeA").SetProtocolParametersHash(iotago.Identifier{})

	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{39}, 4, "38.3", ts.Nodes(), true, false)

	ts.Node("nodeA").SetProtocolParametersHash(hash1)

	// Epoch 6: issue half before restarting and half after restarting.
	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{40, 41, 42, 43}, 4, "39.3", ts.Nodes(), true, false)

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
			nodeE1.Initialize(true,
				append(nodeOptionsWithoutV5,
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
				append(nodeOptionsWithoutV5,
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
	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{44}, 4, "43.3", ts.Nodes("nodeA", "nodeB", "nodeC", "nodeD", "nodeF"), true, false)

	// TODO: would be great to dynamically add accounts for later nodes.
	// Can't issue on nodeG as its account is not known.
	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{45, 46, 47}, 4, "44.3", ts.Nodes("nodeA", "nodeB", "nodeC", "nodeD", "nodeF", "nodeE1"), true, false)

	ts.IssueBlocksAtEpoch("", 6, 4, "47.3", ts.Nodes("nodeA", "nodeB", "nodeC", "nodeD", "nodeF", "nodeE1"), true, false)
	ts.IssueBlocksAtEpoch("", 7, 4, "55.3", ts.Nodes("nodeA", "nodeB", "nodeC", "nodeD", "nodeF", "nodeE1"), true, false)

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
			nodeE2.Initialize(true,
				append(nodeOptionsWithV5,
					protocol.WithBaseDirectory(ts.Directory.Path("nodeE")),
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
				append(nodeOptionsWithV5,
					protocol.WithSnapshotPath(snapshotPath),
					protocol.WithBaseDirectory(ts.Directory.PathWithCreate(nodeG.Name)),
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
			5: 8,
		}, ts.Nodes()...)

		ts.AssertVersionAndProtocolParameters(map[iotago.Version]iotago.ProtocolParameters{
			3: ts.API.ProtocolParameters(),
			5: nil,
		}, ts.Nodes("nodeA", "nodeD", "nodeF", "nodeG")...)

		// We've started nodeH with the v5 protocol parameters set. Therefore, they should be available.
		ts.AssertVersionAndProtocolParameters(map[iotago.Version]iotago.ProtocolParameters{
			3: ts.API.ProtocolParameters(),
			5: v5ProtocolParameters,
		}, ts.Nodes("nodeB", "nodeC", "nodeE2", "nodeH")...)

		ts.AssertVersionAndProtocolParametersHashes(map[iotago.Version]iotago.Identifier{
			3: lo.PanicOnErr(ts.API.ProtocolParameters().Hash()),
			5: hash1,
		}, ts.Nodes()...)

		// check account data at the end of the test
		ts.AssertAccountData(&accounts.AccountData{
			ID:                                    ts.Node("nodeA").Validator.AccountID,
			Credits:                               &accounts.BlockIssuanceCredits{Value: iotago.MaxBlockIssuanceCredits / 2, UpdateSlot: 0},
			ExpirySlot:                            iotago.MaxSlotIndex,
			OutputID:                              ts.AccountOutput("Genesis:1").OutputID(),
			BlockIssuerKeys:                       iotago.NewBlockIssuerKeys(iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(ts.Node("nodeA").Validator.PublicKey))),
			ValidatorStake:                        mock.MinValidatorAccountAmount(ts.API.ProtocolParameters()),
			DelegationStake:                       0,
			FixedCost:                             0,
			StakeEndEpoch:                         iotago.MaxEpochIndex,
			LatestSupportedProtocolVersionAndHash: model.VersionAndHash{Version: 5, Hash: hash1},
		}, ts.Nodes()...)

		ts.AssertAccountData(&accounts.AccountData{
			ID:                                    ts.Node("nodeD").Validator.AccountID,
			Credits:                               &accounts.BlockIssuanceCredits{Value: iotago.MaxBlockIssuanceCredits / 2, UpdateSlot: 0},
			ExpirySlot:                            iotago.MaxSlotIndex,
			OutputID:                              ts.AccountOutput("Genesis:4").OutputID(),
			BlockIssuerKeys:                       iotago.NewBlockIssuerKeys(iotago.Ed25519PublicKeyBlockIssuerKeyFromPublicKey(ed25519.PublicKey(ts.Node("nodeD").Validator.PublicKey))),
			ValidatorStake:                        mock.MinValidatorAccountAmount(ts.API.ProtocolParameters()),
			DelegationStake:                       0,
			FixedCost:                             0,
			StakeEndEpoch:                         iotago.MaxEpochIndex,
			LatestSupportedProtocolVersionAndHash: model.VersionAndHash{Version: 5, Hash: hash2},
		}, ts.Nodes()...)
	}

	// TODO: these node start to warpsync and don't manage to catch up
	for _, node := range ts.Nodes("nodeE2", "nodeH") {
		node.Shutdown()
		ts.RemoveNode(node.Name)
	}

	// Check that issuing still produces the same commitments on the nodes that upgraded. The nodes that did not upgrade
	// should not be able to issue and process blocks with the new version.
	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{64, 65}, 4, "63.3", ts.Nodes("nodeB", "nodeC"), false, false)
	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{66, 67, 68, 69, 70, 71}, 4, "65.3", ts.Nodes("nodeB", "nodeC"), true, false)

	// Nodes that did not set up the new protocol parameters are not able to process blocks with the new version.
	ts.AssertNodeState(ts.Nodes("nodeA", "nodeD", "nodeF", "nodeG"),
		testsuite.WithLatestCommitmentSlotIndex(61),
		testsuite.WithEqualStoredCommitmentAtIndex(61),
	)

	ts.AssertNodeState(ts.Nodes("nodeB", "nodeC"),
		testsuite.WithLatestCommitmentSlotIndex(69),
		testsuite.WithEqualStoredCommitmentAtIndex(69),
	)
}
