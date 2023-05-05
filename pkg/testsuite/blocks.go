package testsuite

import (
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func (t *TestSuite) AssertBlock(block *model.Block, node *mock.Node) *model.Block {
	var loadedBlock *model.Block
	t.Eventuallyf(func() bool {
		var exists bool
		loadedBlock, exists = node.Protocol.MainEngineInstance().Block(block.ID())

		return exists
	}, "AssertBlock: %s: block %s does not exist", node.Name, block.ID())

	require.Equalf(t.Testing, block.ID(), loadedBlock.ID(), "%s: expected %s, got %s", node.Name, block.Block(), loadedBlock.ID())
	require.Equalf(t.Testing, block.Data(), loadedBlock.Data(), "%s: expected %s, got %s", node.Name, block.Data(), loadedBlock.Data())

	return loadedBlock
}

func (t *TestSuite) AssertBlocksExist(blocks []*model.Block, expectedExist bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, block := range blocks {
			if expectedExist {
				t.AssertBlock(block, node)
			} else {
				t.Eventuallyf(func() bool {
					return !lo.Return2(node.Protocol.MainEngineInstance().Block(block.ID()))
				}, "AssertBlocksExist: %s: block %s exists but should not", node.Name, block)
			}
		}
	}
}

func (t *TestSuite) AssertBlocksInCacheAccepted(expectedBlocks []*model.Block, expectedAccepted bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, block := range expectedBlocks {
			var blockFromCache *blocks.Block
			t.Eventuallyf(func() bool {
				var exists bool
				blockFromCache, exists = node.Protocol.MainEngineInstance().BlockFromCache(block.ID())

				return exists
			}, "AssertBlocksInCacheAccepted: %s: block %s does not exist", node.Name, block.ID())

			require.Equalf(t.Testing, expectedAccepted, blockFromCache.IsAccepted(), "AssertBlocksInCacheAccepted: %s: block %s expected %v, got %v", node.Name, blockFromCache.ID(), expectedAccepted, blockFromCache.IsAccepted())
			t.AssertBlock(block, node)
		}
	}
}
