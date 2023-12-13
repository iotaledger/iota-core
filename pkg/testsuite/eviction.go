package testsuite

import (
	"github.com/stretchr/testify/assert"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertActiveRootBlocks(expectedBlocks []*blocks.Block, nodes ...*mock.Node) {
	mustNodes(nodes)

	expectedRootBlocks := make(map[iotago.BlockID]iotago.CommitmentID)
	for _, expectedBlock := range expectedBlocks {
		expectedRootBlocks[expectedBlock.ID()] = expectedBlock.SlotCommitmentID()
	}

	for _, node := range nodes {
		t.Eventually(func() error {
			activeRootBlocks := node.Protocol.Engines.Main.Get().EvictionState.AllActiveRootBlocks()

			if !assert.Equal(t.fakeTesting, expectedRootBlocks, activeRootBlocks) {
				return ierrors.Errorf("AssertActiveRootBlocks: %s: expected %v, got %v", node.Name, expectedRootBlocks, activeRootBlocks)
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertEvictedSlot(expectedIndex iotago.SlotIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			if expectedIndex != lo.Return1(node.Protocol.Engines.Main.Get().EvictionState.LastEvictedSlot()) {
				return ierrors.Errorf("AssertEvictedSlot: %s: expected %d, got %d", node.Name, expectedIndex, lo.Return1(node.Protocol.Engines.Main.Get().EvictionState.LastEvictedSlot()))
			}

			return nil
		})
	}
}
