package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/iotaledger/iota-core/pkg/testsuite"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestProtocol_StartNodeFromSnapshotAndDisk(t *testing.T) {
	ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1", 50)
	node2 := ts.AddValidatorNode("node2", 50)

	ts.Run()
	ts.HookLogging()

	ts.Wait()

	// Verify that node1 has the expected states.
	{
		ts.AssertSnapshotImported(true, ts.Nodes()...)
		ts.AssertProtocolParameters(ts.ProtocolParameters, ts.Nodes()...)
		ts.AssertLatestCommitment(iotago.NewEmptyCommitment(), ts.Nodes()...)
		ts.AssertLatestStateMutationSlot(0, ts.Nodes()...)
		ts.AssertLatestFinalizedSlot(0, ts.Nodes()...)
		ts.AssertChainID(iotago.NewEmptyCommitment().MustID(), ts.Nodes()...)

		ts.AssertStorageCommitments([]*iotago.Commitment{iotago.NewEmptyCommitment()}, ts.Nodes()...)

		ts.AssertSybilProtectionCommittee(map[iotago.AccountID]int64{
			node1.AccountID: 50,
			node2.AccountID: 50,
		}, ts.Nodes()...)
		ts.AssertSybilProtectionOnlineCommittee(map[iotago.AccountID]int64{
			node1.AccountID: 50,
			node2.AccountID: 50,
		}, ts.Nodes()...)
	}

	// Issue blocks in subsequent slots and make sure that commitments, root blocks are correct.
	{
		// Slot 1
		ts.IssueBlockAtSlot("1.1", 1, node1, iotago.EmptyBlockID())
		ts.IssueBlockAtSlot("1.2", 1, node2, iotago.EmptyBlockID())
		ts.IssueBlockAtSlot("1.1*", 1, node1, ts.BlockID("1.2"))
		ts.Wait()

		// Slot 2
		ts.IssueBlockAtSlot("2.2", 2, node2, ts.BlockID("1.1"))
		ts.IssueBlockAtSlot("2.2*", 2, node2, ts.BlockID("1.1*"))

		ts.WaitWithDelay(2 * time.Second)

		ts.AssertBlocksExist(ts.Blocks("1.1", "1.2", "1.1*", "2.2", "2.2*"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("1.1", "1.2", "1.1*"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("2.2", "2.2*"), false, ts.Nodes()...)

		// Slot 3
		ts.IssueBlockAtSlot("3.1", 3, node1, ts.BlockIDs("2.2", "2.2*")...)
		// Slot 4
		ts.IssueBlockAtSlot("4.2", 4, node2, ts.BlockID("3.1"))

		ts.WaitWithDelay(2 * time.Second)

		ts.AssertBlocksExist(ts.Blocks("3.1", "4.2"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("2.2", "2.2*", "3.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("4.2"), false, ts.Nodes()...)

		// Slot 1 should be committed.
		ts.AssertLatestCommitmentSlotIndex(1, ts.Nodes()...)
		ts.AssertStorageRootBlocks(ts.Blocks("1.1", "1.1*"), ts.Nodes()...)
		// TODO: assert root blocks in eviction state
		assert.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().ID(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().ID())

		// Slot 5
		ts.IssueBlockAtSlot("5.1", 5, node1, ts.BlockID("4.2"))
		// Slot 6
		ts.IssueBlockAtSlot("6.2", 6, node2, ts.BlockID("5.1"))
		// Slot 7
		ts.IssueBlockAtSlot("7.1", 7, node1, ts.BlockID("6.2"))
		// Slot 8
		ts.IssueBlockAtSlot("8.2", 8, node2, ts.BlockID("7.1"))

		ts.WaitWithDelay(2 * time.Second)

		ts.AssertBlocksExist(ts.Blocks("5.1", "6.2", "7.1", "8.2"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("4.2", "5.1", "6.2", "7.1"), true, ts.Nodes()...)
		ts.AssertBlocksInCacheAccepted(ts.Blocks("8.2"), false, ts.Nodes()...)

		// We evicted rootblocks of epoch 1, as the rootBlock delay is 4, and we are committing 5.
		ts.AssertLatestCommitmentSlotIndex(5, ts.Nodes()...)
		assert.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().ID(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().ID())
		ts.AssertStorageRootBlocks(ts.Blocks("1.1*", "2.2", "2.2*", "3.1", "4.2"), ts.Nodes()...)
		// TODO: assert root blocks in eviction state

		// TODO: port rest from GoShimmer
	}

	// Create snapshot.

	// Load node3 from created snapshot and verify state.

	// Shutdown node3 and restart it from disk. Verify state.

}
