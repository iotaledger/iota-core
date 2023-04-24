package test

import (
	"testing"
)

func TestName(t *testing.T) {
	f := NewFramework(t)
	defer f.Shutdown()

	f.AddValidatorNode("node1", 100)
	f.AddValidatorNode("node2", 100)

	f.Run()

	f.HookLogging()

	f.Node("node1").IssueBlock()
	f.Node("node1").Wait()
	f.Node("node2").Wait()
	f.Node("node2").IssueBlock()
}
