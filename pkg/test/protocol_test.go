package test

import (
	"fmt"
	"testing"
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
