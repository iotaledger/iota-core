package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	iotago "github.com/iotaledger/iota.go/v4"
)

func Test_Upgrade_Signaling(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithLivenessThreshold(1),
		testsuite.WithMinCommittableAge(2),
		testsuite.WithMaxCommittableAge(4),
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

	hash1 := iotago.Identifier{1}
	hash2 := iotago.Identifier{2}

	ts.Node("nodeA").SetHighestSupportedVersion(5)
	ts.Node("nodeA").SetProtocolParametersHash(hash1)
	ts.Node("nodeB").SetHighestSupportedVersion(5)
	ts.Node("nodeB").SetProtocolParametersHash(hash1)
	ts.Node("nodeC").SetHighestSupportedVersion(5)
	ts.Node("nodeC").SetProtocolParametersHash(hash1)
	ts.Node("nodeD").SetHighestSupportedVersion(5)
	ts.Node("nodeD").SetProtocolParametersHash(hash2)

	ts.IssueBlocksAtEpoch("", 1, 4, "Genesis", ts.Nodes(), true, nil)
	ts.IssueBlocksAtEpoch("", 2, 4, "7.3", ts.Nodes(), true, nil)
	ts.IssueBlocksAtEpoch("", 3, 4, "15.3", ts.Nodes(), true, nil)
	ts.IssueBlocksAtEpoch("", 4, 4, "23.3", ts.Nodes(), true, nil)

	// Epoch 5: revoke vote of nodeA in last slot of epoch.
	ts.IssueBlocksAtSlots("", ts.SlotsForEpoch(5)[:ts.API.TimeProvider().EpochDurationSlots()-1], 4, "31.3", ts.Nodes(), true, nil)

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

			nodeE1 := ts.AddNode("nodeE.1")
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
		require.NoError(t, ts.Node("nodeA").Protocol.MainEngine().WriteSnapshot(snapshotPath))

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
	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{45, 46, 47}, 4, "44.3", ts.Nodes("nodeA", "nodeB", "nodeC", "nodeD", "nodeF", "nodeE.1"), true, nil)

	ts.IssueBlocksAtEpoch("", 7, 4, "47.3", ts.Nodes("nodeA", "nodeB", "nodeC", "nodeD", "nodeF", "nodeE.1"), true, nil)
	ts.IssueBlocksAtEpoch("", 8, 4, "55.3", ts.Nodes("nodeA", "nodeB", "nodeC", "nodeD", "nodeF", "nodeE.1"), true, nil)

	ts.AssertEpochVersions(map[iotago.Version]iotago.EpochIndex{
		3: 0,
		5: 13,
	}, ts.Nodes()...)

	ts.AssertVersionAndProtocolParameters(map[iotago.Version]iotago.ProtocolParameters{
		3: ts.API.ProtocolParameters(),
		5: nil,
	}, ts.Nodes()...)

	ts.AssertVersionAndProtocolParametersHashes(map[iotago.Version]iotago.Identifier{
		3: lo.PanicOnErr(ts.API.ProtocolParameters().Hash()),
		5: hash1,
	}, ts.Nodes()...)
}
