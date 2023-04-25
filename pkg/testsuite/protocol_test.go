package testsuite

import (
	"fmt"
	"testing"

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

	f.AssertSybilProtection(200, 200, node1, node2, node3, node4)
	// TODO: manually set nodes as active/inactive? then verify again

}
