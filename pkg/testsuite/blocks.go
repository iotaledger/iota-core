package testsuite

import (
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func (t *TestSuite) AssertBlock(block *model.Block, node *mock.Node) *model.Block {
	loadedBlock, exists := node.Protocol.MainEngineInstance().Block(block.ID())
	require.True(t.Testing, exists, "%s: block %s does not exist", node.Name, block.ID())
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
				require.False(t.Testing, lo.Return2(node.Protocol.MainEngineInstance().Block(block.ID())), "%s: block %s exists", node.Name, block)
			}
		}
	}
}

func (t *TestSuite) AssertBlocksAccepted(blocks []*model.Block, expectedAccepted bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, block := range blocks {
			blockFromCache, exists := node.Protocol.MainEngineInstance().BlockFromCache(block.ID())
			if exists {
				require.Equalf(t.Testing, expectedAccepted, blockFromCache.IsAccepted(), "AssertBlocksAccepted: %s: block %s expected %v, got %v", node.Name, blockFromCache.ID(), expectedAccepted, blockFromCache.IsAccepted())
				t.AssertBlock(block, node)
			} else {
				// A block that doesn't exist in the cache and is expected to be accepted should be found in the storage.
				t.AssertStorageBlockExist(block, expectedAccepted, node)
			}

		}
	}
}
