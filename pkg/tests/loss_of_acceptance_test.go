package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestLossOfAcceptanceFromGenesis(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithWaitFor(15*time.Second),
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(100, testsuite.DefaultSlotDurationInSeconds),
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
	ts.AddDefaultWallet(node0)
	ts.AddValidatorNode("node1")
	ts.AddNode("node2")

	ts.Run(true, nil)

	// Create snapshot to use later.
	snapshotPath := ts.Directory.Path(fmt.Sprintf("%d_snapshot", time.Now().Unix()))
	require.NoError(t, ts.Node("node0").Protocol.Engines.Main.Get().WriteSnapshot(snapshotPath))

	// Revive chain on node0.
	{
		ts.SetCurrentSlot(50)
		block0 := ts.IssueValidationBlockWithHeaderOptions("block0", node0)
		require.EqualValues(t, 48, ts.Block("block0").SlotCommitmentID().Slot())
		// Reviving the chain should select one parent from the last committed slot.
		require.Len(t, block0.Parents(), 1)
		require.Equal(t, block0.Parents()[0].Alias(), "Genesis")
		ts.AssertBlocksExist(ts.Blocks("block0"), true, ts.Nodes("node0")...)
	}

	// Need to issue to slot 52 so that all other nodes can warp sync up to slot 49 and then commit slot 50 themselves.
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{51, 52}, 2, "block0", ts.Nodes("node0"), true, false)

		ts.AssertLatestCommitmentSlotIndex(50, ts.Nodes()...)
		ts.AssertEqualStoredCommitmentAtIndex(50, ts.Nodes()...)
		ts.AssertBlocksExist(ts.Blocks("block0"), true, ts.Nodes()...)
	}

	// Continue issuing on all nodes for a few slots.
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{53, 54, 55, 56, 57}, 3, "52.1", ts.Nodes(), true, false)

		ts.AssertBlocksInCacheAccepted(ts.BlocksWithPrefix("57.0"), true, ts.Nodes()...)
		ts.AssertLatestCommitmentSlotIndex(55, ts.Nodes()...)
		ts.AssertEqualStoredCommitmentAtIndex(55, ts.Nodes()...)
	}

	// Start node3 from genesis snapshot.
	{
		node3 := ts.AddNode("node3")
		node3.Initialize(true,
			protocol.WithSnapshotPath(snapshotPath),
			protocol.WithBaseDirectory(ts.Directory.PathWithCreate(node3.Name)),
		)
		ts.Wait()
	}

	// Continue issuing on all nodes for a few slots.
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{58, 59}, 3, "57.2", ts.Nodes("node0", "node1", "node2"), true, false)

		ts.AssertBlocksInCacheAccepted(ts.BlocksWithPrefix("59.0"), true, ts.Nodes()...)
		ts.AssertLatestCommitmentSlotIndex(57, ts.Nodes()...)
		ts.AssertEqualStoredCommitmentAtIndex(57, ts.Nodes()...)
	}

	// Check that commitments from 1-49 are empty.
	for slot := iotago.SlotIndex(1); slot <= 49; slot++ {
		ts.AssertStorageCommitmentBlocks(slot, nil, ts.Nodes()...)
	}
}

func TestLossOfAcceptanceFromSnapshot(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(100, testsuite.DefaultSlotDurationInSeconds),
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
	)
	defer ts.Shutdown()

	node0 := ts.AddValidatorNode("node0")
	ts.AddDefaultWallet(node0)
	ts.AddValidatorNode("node1")
	node2 := ts.AddNode("node2")

	ts.Run(true, nil)
	node2.Protocol.SetLogLevel(log.LevelTrace)

	// Issue up to slot 10, committing slot 8.
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 3, "Genesis", ts.Nodes(), true, false)

		ts.AssertBlocksInCacheAccepted(ts.BlocksWithPrefix("10.0"), true, ts.Nodes()...)
		ts.AssertEqualStoredCommitmentAtIndex(8, ts.Nodes()...)
		ts.AssertLatestCommitmentSlotIndex(8, ts.Nodes()...)
	}

	// Create snapshot and restart node0 from it.
	var node0restarted *mock.Node
	{
		snapshotPath := ts.Directory.Path(fmt.Sprintf("%d_snapshot", time.Now().Unix()))
		require.NoError(t, ts.Node("node0").Protocol.Engines.Main.Get().WriteSnapshot(snapshotPath))

		node0restarted = ts.AddNode("node0-restarted")
		node0restarted.Validator = node0.Validator
		node0restarted.Initialize(true,
			protocol.WithSnapshotPath(snapshotPath),
			protocol.WithBaseDirectory(ts.Directory.PathWithCreate(node0restarted.Name)),
		)
		ts.Wait()
	}

	for _, node := range ts.Nodes("node0", "node1") {
		ts.RemoveNode(node.Name)
		node.Shutdown()
	}

	// Revive chain on node0-restarted.
	{
		ts.SetCurrentSlot(20)
		block0 := ts.IssueValidationBlockWithHeaderOptions("block0", node0restarted)
		require.EqualValues(t, 18, block0.SlotCommitmentID().Slot())
		// Reviving the chain should select one parent from the last committed slot.
		require.Len(t, block0.Parents(), 1)
		require.EqualValues(t, block0.Parents()[0].Slot(), 8)
		ts.AssertBlocksExist(ts.Blocks("block0"), true, ts.Nodes("node0-restarted")...)
	}

	// Need to issue to slot 22 so that all other nodes can warp sync up to slot 19 and then commit slot 20 themselves.
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{21, 22}, 2, "block0", ts.Nodes("node0-restarted"), true, false)

		ts.AssertEqualStoredCommitmentAtIndex(20, ts.Nodes()...)
		ts.AssertLatestCommitmentSlotIndex(20, ts.Nodes()...)
	}

	// Continue issuing on all online nodes for a few slots.
	{
		// Since issued blocks in slot 9 and 10 are be orphaned, we need to make sure that the already issued transactions in the testsuite
		// are not used again.
		ts.SetAutomaticTransactionIssuingCounters(node2.Partition, 24)

		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{23, 24, 25}, 3, "22.1", ts.Nodes(), true, false)

		ts.AssertBlocksInCacheAccepted(ts.BlocksWithPrefix("25.0"), true, ts.Nodes()...)
		ts.AssertEqualStoredCommitmentAtIndex(23, ts.Nodes()...)
		ts.AssertLatestCommitmentSlotIndex(23, ts.Nodes()...)
	}

	// Check that commitments from 8-19 are empty -> all previously accepted blocks in 9,10 have been orphaned.
	for _, slot := range []iotago.SlotIndex{9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19} {
		ts.AssertStorageCommitmentBlocks(slot, nil, ts.Nodes()...)
	}
}

func TestLossOfAcceptanceWithRestartFromDisk(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(100, testsuite.DefaultSlotDurationInSeconds),
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
	)
	defer ts.Shutdown()

	node0 := ts.AddValidatorNode("node0")
	ts.AddDefaultWallet(node0)
	ts.AddValidatorNode("node1")
	node2 := ts.AddNode("node2")

	ts.Run(true, nil)
	node2.Protocol.SetLogLevel(log.LevelTrace)

	// Issue up to slot 10, committing slot 8.
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 3, "Genesis", ts.Nodes(), true, false)

		ts.AssertBlocksInCacheAccepted(ts.BlocksWithPrefix("10.0"), true, ts.Nodes()...)
		ts.AssertEqualStoredCommitmentAtIndex(8, ts.Nodes()...)
		ts.AssertLatestCommitmentSlotIndex(8, ts.Nodes()...)
	}

	for _, node := range ts.Nodes("node0", "node1") {
		ts.RemoveNode(node.Name)
		node.Shutdown()
	}

	// Create snapshot and restart node0 from it.
	var node0restarted *mock.Node
	{
		node0restarted = ts.AddNode("node0-restarted")
		node0restarted.Validator = node0.Validator
		node0restarted.Initialize(true,
			protocol.WithBaseDirectory(ts.Directory.PathWithCreate(node0.Name)),
		)
		ts.Wait()
	}

	// Revive chain on node0-restarted.
	{
		ts.SetCurrentSlot(20)
		block0 := ts.IssueValidationBlockWithHeaderOptions("block0", node0restarted)
		require.EqualValues(t, 18, block0.SlotCommitmentID().Slot())
		// Reviving the chain should select one parent from the last committed slot.
		require.Len(t, block0.Parents(), 1)
		require.EqualValues(t, block0.Parents()[0].Slot(), 8)
		ts.AssertBlocksExist(ts.Blocks("block0"), true, ts.Nodes("node0-restarted")...)
	}

	// Need to issue to slot 22 so that all other nodes can warp sync up to slot 19 and then commit slot 20 themselves.
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{21, 22}, 2, "block0", ts.Nodes("node0-restarted"), true, false)

		ts.AssertEqualStoredCommitmentAtIndex(20, ts.Nodes()...)
		ts.AssertLatestCommitmentSlotIndex(20, ts.Nodes()...)
	}

	// Continue issuing on all online nodes for a few slots.
	{
		// Since issued blocks in slot 9 and 10 are be orphaned, we need to make sure that the already issued transactions in the testsuite
		// are not used again.
		ts.SetAutomaticTransactionIssuingCounters(node2.Partition, 24)

		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{23, 24, 25}, 3, "22.1", ts.Nodes(), true, false)

		ts.AssertBlocksInCacheAccepted(ts.BlocksWithPrefix("25.0"), true, ts.Nodes()...)
		ts.AssertEqualStoredCommitmentAtIndex(23, ts.Nodes()...)
		ts.AssertLatestCommitmentSlotIndex(23, ts.Nodes()...)
	}

	// Check that commitments from 8-19 are empty -> all previously accepted blocks in 9,10 have been orphaned.
	for _, slot := range []iotago.SlotIndex{9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19} {
		ts.AssertStorageCommitmentBlocks(slot, nil, ts.Nodes()...)
	}
}
