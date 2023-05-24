package testsuite

import (
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"

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
			activeRootBlocks := node.Protocol.MainEngineInstance().EvictionState.ActiveRootBlocks()

			if len(expectedBlocks) != len(activeRootBlocks) {
				return errors.Errorf("AssertActiveRootBlocks: %s: expected %d active root blocks, got %d", node.Name, len(expectedBlocks), len(activeRootBlocks))
			}

			if !cmp.Equal(expectedRootBlocks, activeRootBlocks) {
				return errors.Errorf("AssertActiveRootBlocks: %s: expected %v, got %v", node.Name, expectedRootBlocks, activeRootBlocks)
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertEvictedSlot(expectedIndex iotago.SlotIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			if expectedIndex != lo.Return1(node.Protocol.MainEngineInstance().EvictionState.LastEvictedSlot()) {
				return errors.Errorf("AssertEvictedSlot: %s: expected %d, got %d", node.Name, expectedIndex, lo.Return1(node.Protocol.MainEngineInstance().EvictionState.LastEvictedSlot()))
			}

			return nil
		})
	}
}
