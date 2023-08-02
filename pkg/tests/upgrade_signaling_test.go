package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/blockfactory"
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
		testsuite.WithMinCommittableAge(1),
		testsuite.WithMaxCommittableAge(3),
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
			engine.WithRequesterOptions(
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

	ts.Run(map[string][]options.Option[protocol.Protocol]{
		"nodeA": nodeOptions,
		"nodeB": nodeOptions,
		"nodeC": nodeOptions,
		"nodeD": nodeOptions,
		"nodeE": nodeOptions,
		"nodeF": nodeOptions,
	})
	ts.HookLogging()

	ts.Wait()

	hash1 := iotago.Identifier{1}
	hash2 := iotago.Identifier{2}

	issuingOptions := map[string][]options.Option[blockfactory.BlockParams]{
		"nodeA": {blockfactory.WithHighestSupportedVersion(5), blockfactory.WithProtocolParametersHash(hash1)},
		"nodeB": {blockfactory.WithHighestSupportedVersion(5), blockfactory.WithProtocolParametersHash(hash1)},
		"nodeC": {blockfactory.WithHighestSupportedVersion(5), blockfactory.WithProtocolParametersHash(hash1)},
		"nodeD": {blockfactory.WithHighestSupportedVersion(5), blockfactory.WithProtocolParametersHash(hash2)},
	}

	ts.IssueBlocksAtEpoch("", 1, 4, "Genesis", ts.Nodes(), true, issuingOptions)
	ts.IssueBlocksAtEpoch("", 2, 4, "7.3", ts.Nodes(), true, issuingOptions)
	ts.IssueBlocksAtEpoch("", 3, 4, "15.3", ts.Nodes(), true, issuingOptions)
	ts.IssueBlocksAtEpoch("", 4, 4, "23.3", ts.Nodes(), true, issuingOptions)

	// Epoch 5: revoke vote of nodeA in last slot of epoch.
	ts.IssueBlocksAtSlots("", ts.SlotsForEpoch(5)[:ts.API.TimeProvider().EpochDurationSlots()-1], 4, "31.3", ts.Nodes(), true, issuingOptions)

	issuingOptionsRevoke := lo.MergeMaps(map[string][]options.Option[blockfactory.BlockParams]{}, issuingOptions)
	issuingOptionsRevoke["nodeA"] = []options.Option[blockfactory.BlockParams]{blockfactory.WithHighestSupportedVersion(5), blockfactory.WithProtocolParametersHash(iotago.Identifier{})}
	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{39}, 4, "38.3", ts.Nodes(), true, issuingOptionsRevoke)

	// Epoch 6: issue half before restarting and half after restarting.
	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{40, 41, 42, 43}, 4, "39.3", ts.Nodes(), true, issuingOptions)

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
			nodeE1.Initialize(
				append(nodeOptions,
					protocol.WithBaseDirectory(ts.Directory.Path(nodeE.Name)),
				)...,
			)
			nodeE1.HookLogging()
			ts.Wait()
		}

		// Create snapshot.
		snapshotPath := ts.Directory.Path(fmt.Sprintf("%d_snapshot", time.Now().Unix()))
		require.NoError(t, ts.Node("nodeA").Protocol.MainEngineInstance().WriteSnapshot(snapshotPath))

		{
			nodeG := ts.AddNode("nodeG")
			nodeG.Initialize(
				append(nodeOptions,
					protocol.WithSnapshotPath(snapshotPath),
					protocol.WithBaseDirectory(ts.Directory.PathWithCreate(nodeG.Name)),
				)...,
			)
			ts.Wait()
			nodeG.HookLogging()

			ts.AssertNodeState(ts.Nodes(),
				testsuite.WithLatestCommitmentSlotIndex(41),
				testsuite.WithActiveRootBlocks(expectedRootBlocks),
			)
		}
	}

	// Can only continue to issue on nodeA, nodeB, nodeC, nodeD, nodeF. nodeE and nodeG were just restarted and don't have the latest unaccepted state.
	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{44}, 4, "43.3", ts.Nodes("nodeA", "nodeB", "nodeC", "nodeD", "nodeF"), true, issuingOptions)

	// TODO: would be great to dynamically add accounts for later nodes.
	// Can't issue on nodeG as its account is not known.
	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{45, 46, 47}, 4, "44.3", ts.Nodes("nodeA", "nodeB", "nodeC", "nodeD", "nodeF", "nodeE.1"), true, issuingOptions)

	ts.IssueBlocksAtEpoch("", 7, 4, "47.3", ts.Nodes("nodeA", "nodeB", "nodeC", "nodeD", "nodeF", "nodeE.1"), true, issuingOptions)
	ts.IssueBlocksAtEpoch("", 8, 4, "55.3", ts.Nodes("nodeA", "nodeB", "nodeC", "nodeD", "nodeF", "nodeE.1"), true, issuingOptions)

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
