package testsuite

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertPrunedSlot(expectedIndex iotago.SlotIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			if expectedIndex != lo.Return1(node.Protocol.MainEngineInstance().Storage.LastPrunedSlot()) {
				return errors.Errorf("AssertPrunedSlot: %s: expected %d, got %d", node.Name, expectedIndex, lo.Return1(node.Protocol.MainEngineInstance().Storage.LastPrunedSlot()))
			}

			return nil
		})
	}
}
