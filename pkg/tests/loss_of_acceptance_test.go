package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/blockfactory"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestLossOfAcceptanceFromGenesis(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithLivenessThresholdLowerBound(10),
		testsuite.WithLivenessThresholdUpperBound(10),
		testsuite.WithMinCommittableAge(2),
		testsuite.WithMaxCommittableAge(4),
		testsuite.WithEpochNearingThreshold(2),
		testsuite.WithSlotsPerEpochExponent(3),
		testsuite.WithGenesisTimestampOffset(100*10),
	)
	defer ts.Shutdown()

	node0 := ts.AddValidatorNode("node0")
	ts.AddValidatorNode("node1")
	ts.AddNode("node2")

	ts.Run(true, nil)

	// Create snapshot to use later.
	snapshotPath := ts.Directory.Path(fmt.Sprintf("%d_snapshot", time.Now().Unix()))
	require.NoError(t, ts.Node("node0").Protocol.MainEngineInstance().WriteSnapshot(snapshotPath))

	ts.IssueValidationBlock("block0", node0,
		blockfactory.WithIssuingTime(ts.API.TimeProvider().SlotStartTime(50)),
		blockfactory.WithStrongParents(ts.BlockIDs("Genesis")...),
	)
	require.EqualValues(t, 48, ts.Block("block0").SlotCommitmentID().Slot())
	ts.AssertBlocksExist(ts.Blocks("block0"), true, ts.Nodes("node0")...)

	// Need to issue to slot 52 so that all other nodes can warp sync up to slot 49 and then commit slot 50 themselves.
	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{51, 52}, 2, "block0", ts.Nodes("node0"), true, nil)

	ts.AssertEqualStoredCommitmentAtIndex(50, ts.Nodes()...)
	ts.AssertLatestCommitmentSlotIndex(50, ts.Nodes()...)
	ts.AssertBlocksExist(ts.Blocks("block0"), true, ts.Nodes()...)

	// Continue issuing on all nodes for a few slots.
	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{53, 54, 55, 56, 57}, 3, "52.1", ts.Nodes(), true, nil)

	ts.AssertEqualStoredCommitmentAtIndex(55, ts.Nodes()...)
	ts.AssertLatestCommitmentSlotIndex(55, ts.Nodes()...)
	ts.AssertBlocksInCacheAccepted(ts.BlocksWithPrefix("57.0"), true, ts.Nodes()...)

	{
		node3 := ts.AddNode("node3")
		node3.Initialize(true,
			protocol.WithSnapshotPath(snapshotPath),
			protocol.WithBaseDirectory(ts.Directory.PathWithCreate(node3.Name)),
		)
		ts.Wait()
	}

	// Continue issuing on all nodes for a few slots.
	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{58, 59}, 3, "57.2", ts.Nodes("node0", "node1", "node2"), true, nil)

	ts.AssertEqualStoredCommitmentAtIndex(57, ts.Nodes()...)
	ts.AssertLatestCommitmentSlotIndex(57, ts.Nodes()...)
	ts.AssertBlocksInCacheAccepted(ts.BlocksWithPrefix("59.0"), true, ts.Nodes()...)
}
