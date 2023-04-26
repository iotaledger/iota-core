package testsuite

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iotago "github.com/iotaledger/iota.go/v4"
)

func TestName(t *testing.T) {
	f := NewFramework(t)
	defer f.Shutdown()

	f.AddValidatorNode("node1", 100)
	f.AddValidatorNode("node2", 100)

	f.Run()

	f.HookLogging()

	blockID1 := f.Node("node1").IssueBlock()
	f.Node("node1").Wait()
	f.Node("node2").Wait()
	f.Node("node2").IssueBlock()

	node1 := f.Node("node1")
	node1.Wait()
	f.Node("node2").Wait()

	block, exist := node1.Protocol.MainEngineInstance().Block(blockID1)
	fmt.Println(block.String(), exist)
}

func TestProtocol_EngineSwitching(t *testing.T) {
	f := NewFramework(t)
	defer f.Shutdown()

	node1 := f.AddValidatorNodeToPartition("node1", 75, "P1")
	node2 := f.AddValidatorNodeToPartition("node2", 75, "P1")
	node3 := f.AddValidatorNodeToPartition("node3", 25, "P2")
	node4 := f.AddValidatorNodeToPartition("node4", 25, "P2")

	f.Run()
	f.HookLogging()

	f.Wait()

	// Verify all nodes have the expected states.
	f.AssertSnapshotImported(true, f.Nodes()...)
	f.AssertProtocolParameters(f.ProtocolParameters, f.Nodes()...)
	f.AssertLatestCommitment(iotago.NewEmptyCommitment(), f.Nodes()...)
	f.AssertLatestStateMutationSlot(0, f.Nodes()...)
	f.AssertLatestFinalizedSlot(0, f.Nodes()...)
	f.AssertChainID(iotago.NewEmptyCommitment().MustID(), f.Nodes()...)

	f.AssertStorageCommitments([]*iotago.Commitment{iotago.NewEmptyCommitment()}, f.Nodes()...)

	f.AssertSybilProtection(200, 200, node1, node2, node3, node4)
	// TODO: manually set nodes as active/inactive? then verify again

	// TODO: wait for online weight
	assert.Eventually(t, func() bool {
		for _, node := range f.Nodes("node1", "node2") {
			if node.Protocol.MainEngineInstance().SybilProtection.OnlineCommittee().TotalWeight() <= 150 {
				return true
			}
		}
		return false
	}, 100*time.Second, 100*time.Millisecond)
	assert.Eventually(t, func() bool {
		for _, node := range f.Nodes("node3", "node4") {
			if node.Protocol.MainEngineInstance().SybilProtection.OnlineCommittee().TotalWeight() <= 50 {
				return true
			}
		}
		return false
	}, 100*time.Second, 100*time.Millisecond)

	// Issue blocks on partition 1.
	{
		f.IssueBlockAtSlot("P1.A", 5, node1, iotago.EmptyBlockID())
		f.Wait()
		f.IssueBlockAtSlot("P1.B", 6, node2, f.Block("P1.A").ID())
		f.Wait()
		f.IssueBlockAtSlot("P1.C", 7, node1, f.Block("P1.B").ID())
		f.Wait()
		f.IssueBlockAtSlot("P1.D", 8, node2, f.Block("P1.C").ID())
		f.Wait()
		f.IssueBlockAtSlot("P1.E", 9, node1, f.Block("P1.D").ID())
		f.Wait()
		f.IssueBlockAtSlot("P1.F", 10, node2, f.Block("P1.E").ID())
		f.Wait()
		f.IssueBlockAtSlot("P1.G", 11, node1, f.Block("P1.F").ID())

		f.WaitWithDelay(1 * time.Second) // Give some time for the blocks to arrive over the network

		f.AssertBlocksExist(f.BlocksByGroup("P1"), true, node1, node2)
		f.AssertBlocksExist(f.BlocksByGroup("P1"), false, node3, node4)

		f.AssertBlocksAccepted(f.Blocks("P1.A", "P1.B", "P1.C", "P1.D", "P1.E", "P1.F"), true, node1, node2)
		f.AssertBlocksAccepted(f.Blocks("P1.G"), false, node1, node2) // block not referenced yet
	}

	// Issue blocks on partition 2.
	{
		f.IssueBlockAtSlot("P2.A", 5, node3, iotago.EmptyBlockID())
		f.Wait()
		f.IssueBlockAtSlot("P2.B", 6, node4, f.Block("P2.A").ID())
		f.Wait()
		f.IssueBlockAtSlot("P2.C", 7, node3, f.Block("P2.B").ID())
		f.Wait()
		f.IssueBlockAtSlot("P2.D", 8, node4, f.Block("P2.C").ID())
		f.Wait()
		f.IssueBlockAtSlot("P2.E", 9, node3, f.Block("P2.D").ID())
		f.Wait()
		f.IssueBlockAtSlot("P2.F", 10, node4, f.Block("P2.E").ID())
		f.Wait()
		f.IssueBlockAtSlot("P2.G", 11, node3, f.Block("P2.F").ID())

		f.WaitWithDelay(1 * time.Second) // Give some time for the blocks to arrive over the network

		f.AssertBlocksExist(f.BlocksByGroup("P2"), true, node3, node4)
		f.AssertBlocksExist(f.BlocksByGroup("P2"), false, node1, node2)

		f.AssertBlocksAccepted(f.Blocks("P2.A", "P2.B", "P2.C", "P2.D", "P2.E", "P2.F"), true, node3, node4)
		f.AssertBlocksAccepted(f.Blocks("P2.G"), false, node3, node4) // block not referenced yet
	}

	// Both partitions should have committed slot 8 and have different commitments
	{
		f.Wait()
		require.EqualValues(t, 8, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Index)
		require.EqualValues(t, 8, node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Index)
		require.EqualValues(t, 8, node3.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Index)
		require.EqualValues(t, 8, node4.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Index)

		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment())
		require.Equal(t, node3.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment(), node4.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment())
		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment(), node3.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment())
	}

}
