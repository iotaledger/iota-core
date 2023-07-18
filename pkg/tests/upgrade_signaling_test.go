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
		testsuite.WithEvictionAge(1),
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

	issueBlocksAtSlotsInEpoch(ts, 1, []iotago.SlotIndex{1, 2, 3, 4, 5, 6, 7}, issuingOptions)
	issueBlocksAtSlotsInEpoch(ts, 2, []iotago.SlotIndex{0, 1, 2, 3, 4, 5, 6, 7}, issuingOptions)
	issueBlocksAtSlotsInEpoch(ts, 3, []iotago.SlotIndex{0, 1, 2, 3, 4, 5, 6, 7}, issuingOptions)
	issueBlocksAtSlotsInEpoch(ts, 4, []iotago.SlotIndex{0, 1, 2, 3, 4, 5, 6, 7}, issuingOptions)
	issueBlocksAtSlotsInEpoch(ts, 5, []iotago.SlotIndex{0, 1, 2, 3, 4, 5, 6}, issuingOptions)

	issuingOptionsRevoke := lo.MergeMaps(map[string][]options.Option[blockfactory.BlockParams]{}, issuingOptions)
	issuingOptionsRevoke["nodeA"] = []options.Option[blockfactory.BlockParams]{blockfactory.WithHighestSupportedVersion(5), blockfactory.WithProtocolParametersHash(iotago.Identifier{})}
	issueBlocksAtSlotsInEpoch(ts, 5, []iotago.SlotIndex{7}, issuingOptionsRevoke)

	issueBlocksAtSlotsInEpoch(ts, 6, []iotago.SlotIndex{0, 1, 2, 3, 4}, issuingOptions)

	{
		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestCommitmentSlotIndex(40),
			testsuite.WithActiveRootBlocks(ts.Blocks(
				"5.6-nodeA", "5.6-nodeB", "5.6-nodeC", "5.6-nodeD",
				"5.7-nodeA", "5.7-nodeB", "5.7-nodeC", "5.7-nodeD",
				"6.0-nodeA", "6.0-nodeB", "6.0-nodeC", "6.0-nodeD",
			)),
		)

		// Shutdown nodeA and restart it from disk. Verify state.
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

			ts.AssertNodeState(ts.Nodes(),
				testsuite.WithLatestCommitmentSlotIndex(40),
				testsuite.WithActiveRootBlocks(ts.Blocks(
					"5.6-nodeA", "5.6-nodeB", "5.6-nodeC", "5.6-nodeD",
					"5.7-nodeA", "5.7-nodeB", "5.7-nodeC", "5.7-nodeD",
					"6.0-nodeA", "6.0-nodeB", "6.0-nodeC", "6.0-nodeD",
				)),
			)
		}
	}

	issueBlocksAtSlotsInEpoch(ts, 6, []iotago.SlotIndex{5, 6, 7}, issuingOptions)
	issueBlocksAtSlotsInEpoch(ts, 7, []iotago.SlotIndex{0, 1, 2, 3, 4, 5, 6, 7}, issuingOptions)
	issueBlocksAtSlotsInEpoch(ts, 8, []iotago.SlotIndex{0, 1, 2, 3, 4, 5, 6, 7}, issuingOptions)

	ts.AssertEpochVersions(map[iotago.Version]iotago.EpochIndex{
		3: 0,
		5: 13,
	}, ts.Nodes()...)

	ts.AssertVersionAndProtocolParameters(map[iotago.Version]iotago.ProtocolParameters{
		3: ts.API.ProtocolParameters(),
		5: nil,
	}, ts.Nodes()...)

	// TODO: add this after handling future protocol parameters hashes in settings
	// ts.AssertVersionsAndProtocolParameterHashes(map[iotago.Version]iotago.Identifier{
	// 	3: ts.API.ProtocolParameters().Hash(),
	// 	5: hash1,
	// }, ts.Nodes()...)
}

func issueBlocksAtSlotsInEpoch(ts *testsuite.TestSuite, epoch iotago.EpochIndex, slotsWithinEpoch []iotago.SlotIndex, issuingOptions map[string][]options.Option[blockfactory.BlockParams]) {
	for _, slotWithinEpoch := range slotsWithinEpoch {
		parentsPrefix := fmt.Sprintf("%d.%d", epoch, slotWithinEpoch-1)
		if epoch == 1 && slotWithinEpoch == 1 {
			parentsPrefix = fmt.Sprintf("Genesis")
		} else if slotWithinEpoch == 0 {
			parentsPrefix = fmt.Sprintf("%d.%d", epoch-1, 7)
		}

		issuedBlocks := issueBlockAllNodes(ts, epoch, slotWithinEpoch, ts.BlockIDsWithPrefix(parentsPrefix), issuingOptions)

		actualSlot := ts.API.TimeProvider().EpochStart(epoch) + slotWithinEpoch
		// We can assert only after slot 5:
		// MinCommittableAge=1 -> when we accept in slot 3 we can commit slot 1.
		// Since we issue per slot only once, we accept in slot 3 when issuing in slot 5.
		if actualSlot >= 5 {
			ts.AssertLatestCommitmentSlotIndex(actualSlot-4, ts.Nodes()...)
		} else {
			ts.AssertBlocksExist(issuedBlocks, true, ts.Nodes()...)
		}
	}
}

func issueBlockAllNodes(ts *testsuite.TestSuite, epoch iotago.EpochIndex, slotWithinEpoch iotago.SlotIndex, strongParents iotago.BlockIDs, issuingOptions map[string][]options.Option[blockfactory.BlockParams]) []*blocks.Block {
	issuedBlocks := make([]*blocks.Block, 0)

	if issuingOptions == nil {
		issuingOptions = make(map[string][]options.Option[blockfactory.BlockParams])
	}

	for _, node := range ts.Validators() {
		blockAlias := fmt.Sprintf("%d.%d-%s", epoch, slotWithinEpoch, node.Name)
		if strongParents != nil {
			issuingOptions[node.Name] = append(issuingOptions[node.Name], blockfactory.WithStrongParents(strongParents...))
		}

		b := ts.IssueValidationBlockAtSlotWithinEpochWithOptions(blockAlias, epoch, slotWithinEpoch, node, issuingOptions[node.Name]...)
		issuedBlocks = append(issuedBlocks, b)
	}

	return issuedBlocks
}
