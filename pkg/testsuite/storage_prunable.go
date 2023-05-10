package testsuite

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertPrunedSlot(expectedIndex iotago.SlotIndex, expectedHasPruned bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			if prunedIndex, hasPruned := node.Protocol.MainEngineInstance().Storage.LastPrunedSlot(); expectedIndex != prunedIndex {
				return errors.Errorf("AssertPrunedSlot: %s: expected %d, got %d", node.Name, expectedIndex, prunedIndex)
			} else if expectedHasPruned != hasPruned {
				return errors.Errorf("AssertPrunedSlot: %s: expected to pruned %t, got %t", node.Name, expectedHasPruned, hasPruned)
			}

			return nil
		})
	}
}
