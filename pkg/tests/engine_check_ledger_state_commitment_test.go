package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestCheckLedgerStateCommitmentFromNonGenesisSnapshot(t *testing.T) {
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

	ts.Run(true, nil)

	// Issue up to slot 10, committing slot 8.
	{
		ts.IssueBlocksAtSlots("", []iotago.SlotIndex{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 3, "Genesis", ts.Nodes(), true, false)

		ts.AssertNodeState(ts.Nodes(),
			testsuite.WithLatestFinalizedSlot(7),
			testsuite.WithLatestCommitmentSlotIndex(8),
			testsuite.WithEqualStoredCommitmentAtIndex(8),
		)
	}

	// Create snapshot from node0 and start node 2 from it. Stop node 0
	var node2 *mock.Node
	{
		snapshotPath := ts.Directory.Path(fmt.Sprintf("%d_snapshot", time.Now().Unix()))
		require.NoError(t, ts.Node("node0").Protocol.Engines.Main.Get().WriteSnapshot(snapshotPath))

		node2 = ts.AddNode("node2")
		node2.Validator = node0.Validator
		node2.Initialize(true,
			protocol.WithSnapshotPath(snapshotPath),
			protocol.WithBaseDirectory(ts.Directory.PathWithCreate(node2.Name)),
		)
		ts.Wait()
	}

	ts.RemoveNode(node0.Name)
	node0.Shutdown()

	// Create snapshot for the latest slot (which has available roots) from node2 and start node 3 from it. Stop node 2
	var node3 *mock.Node

	{
		snapshotPath := ts.Directory.Path(fmt.Sprintf("%d_snapshot", time.Now().Unix()))

		curCommitment := ts.Node("node2").Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment()
		rootsStorage, _ := ts.Node("node2").Protocol.Engines.Main.Get().Storage.Roots(curCommitment.Slot())

		roots, _, _ := rootsStorage.Load(curCommitment.ID())

		newRoots := iotago.NewRoots(
			roots.TangleRoot,
			roots.StateMutationRoot,
			roots.AttestationsRoot,
			iotago.Identifier{0},
			roots.AccountRoot,
			roots.CommitteeRoot,
			roots.RewardsRoot,
			roots.ProtocolParametersHash,
		)

		rootsStorage.Store(curCommitment.ID(), newRoots)

		require.NoError(t, ts.Node("node2").Protocol.Engines.Main.Get().WriteSnapshot(snapshotPath))

		node3 = ts.AddNode("node3")
		node3.Validator = node2.Validator
		node3.Initialize(true,
			protocol.WithSnapshotPath(snapshotPath),
			protocol.WithBaseDirectory(ts.Directory.PathWithCreate(node3.Name)),
		)
		ts.Wait()
	}

	ts.RemoveNode(node2.Name)
	node2.Shutdown()

}
