package testsuite

import (
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func (f *Framework) AssertBlock(block *model.Block, node *mock.Node) *model.Block {
	loadedBlock, exists := node.Protocol.MainEngineInstance().Block(block.ID())
	require.True(f.Testing, exists, "%s: block %s does not exist", node.Name, block)
	require.Equalf(f.Testing, block.ID(), loadedBlock.ID(), "%s: expected %s, got %s", node.Name, block.Block(), loadedBlock.ID())
	require.Equalf(f.Testing, block.Data(), loadedBlock.Data(), "%s: expected %s, got %s", node.Name, block.Data(), loadedBlock.Data())

	return loadedBlock
}

func (f *Framework) AssertBlocksExist(blocks []*model.Block, expectedExist bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, block := range blocks {
			if expectedExist {
				f.AssertBlock(block, node)
			} else {
				_, exists := node.Protocol.MainEngineInstance().Block(block.ID())
				require.False(f.Testing, exists, "%s: block %s exists", node.Name, block)
			}
		}
	}
}

func (f *Framework) AssertBlocksAccepted(blocks []*model.Block, expectedAccepted bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, block := range blocks {
			blockFromCache, exists := node.Protocol.MainEngineInstance().BlockFromCache(block.ID())
			if exists {
				require.Equalf(f.Testing, expectedAccepted, blockFromCache.IsAccepted(), "%s: expected %s, got %s", node.Name, expectedAccepted, blockFromCache.IsAccepted())
				f.AssertBlock(block, node)
			} else {
				// A block that doesn't exist in the cache and is expected to be accepted should be found in the storage.
				f.AssertStorageBlockExist(block, expectedAccepted, node)
			}

		}
	}
}
