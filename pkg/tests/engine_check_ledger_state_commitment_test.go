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

// Test function that checks the ledger state commitment from a non-genesis snapshot.
// It involves setting up a test suite, issuing blocks at specific slots, creating and using snapshots, and starting nodes based on those snapshots.
// The test verifies node states and commitments during the process.
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
	// Create snapshot from node0 and start node 2 from it.
	// During startup, it will correctly validate the ledger state for the last committed slot against the latest commitment.
	// Stop node 0.
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

	// Modify StateRoot and check whether the commitment check gets an error
	{
		curCommitment := ts.Node("node2").Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment()
		rootsStorage, _ := ts.Node("node2").Protocol.Engines.Main.Get().Storage.Roots(curCommitment.Slot())
		roots, _, _ := rootsStorage.Load(curCommitment.ID())

		// create a modified root and store it
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
		require.NoError(t, rootsStorage.Store(curCommitment.ID(), newRoots))

		// check that with a modified root, it does not pass the required check

		require.Error(t, ts.Node("node2").Protocol.Engines.Main.Get().Storage.CheckCorrectnessCommitmentLedgerState())

		ts.Wait()
	}
}
