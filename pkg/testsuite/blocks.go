package testsuite

import (
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func (t *TestSuite) AssertBlock(block *blocks.Block, node *mock.Node) *model.Block {
	var loadedBlock *model.Block
	t.Eventually(func() error {
		var exists bool
		loadedBlock, exists = node.Protocol.MainEngineInstance().Block(block.ID())
		if !exists {
			return errors.Errorf("AssertBlock: %s: block %s does not exist", node.Name, block.ID())
		}

		if block.ID() != loadedBlock.ID() {
			return errors.Errorf("AssertBlock: %s: expected %s, got %s", node.Name, block.ID(), loadedBlock.ID())
		}
		if !cmp.Equal(block.ModelBlock().Data(), loadedBlock.Data()) {
			return errors.Errorf("AssertBlock: %s: expected %s, got %s", node.Name, block.ModelBlock().Data(), loadedBlock.Data())
		}

		return nil
	})

	return loadedBlock
}

func (t *TestSuite) AssertBlocksExist(blocks []*blocks.Block, expectedExist bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, block := range blocks {
			if expectedExist {
				t.AssertBlock(block, node)
			} else {
				t.Eventually(func() error {
					if lo.Return2(node.Protocol.MainEngineInstance().Block(block.ID())) {
						return errors.Errorf("AssertBlocksExist: %s: block %s exists but should not", node.Name, block)
					}

					return nil
				})
			}
		}
	}
}

func (t *TestSuite) assertBlocksInCacheWithFunc(expectedBlocks []*blocks.Block, expectedPropertyState bool, propertyFunc func(*blocks.Block) bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, block := range expectedBlocks {
			t.Eventually(func() error {
				blockFromCache, exists := node.Protocol.MainEngineInstance().BlockFromCache(block.ID())
				if !exists {
					return errors.Errorf("assertBlocksInCacheWithFunc: %s: block %s does not exist", node.Name, block.ID())
				}

				if expectedPropertyState != propertyFunc(blockFromCache) {
					return errors.Errorf("assertBlocksInCacheWithFunc: %s: block %s: expected %v, got %v", node.Name, blockFromCache.ID(), expectedPropertyState, propertyFunc(blockFromCache))
				}

				return nil
			})

			t.AssertBlock(block, node)
		}
	}
}

func (t *TestSuite) AssertBlocksInCacheAccepted(expectedBlocks []*blocks.Block, expectedAccepted bool, nodes ...*mock.Node) {
	t.assertBlocksInCacheWithFunc(expectedBlocks, expectedAccepted, (*blocks.Block).IsAccepted, nodes...)
}

func (t *TestSuite) AssertBlocksInCacheRatifiedAccepted(expectedBlocks []*blocks.Block, expectedRatifiedAccepted bool, nodes ...*mock.Node) {
	t.assertBlocksInCacheWithFunc(expectedBlocks, expectedRatifiedAccepted, (*blocks.Block).IsRatifiedAccepted, nodes...)
}

func (t *TestSuite) AssertBlocksInCacheConfirmed(expectedBlocks []*blocks.Block, expectedConfirmed bool, nodes ...*mock.Node) {
	t.assertBlocksInCacheWithFunc(expectedBlocks, expectedConfirmed, (*blocks.Block).IsConfirmed, nodes...)
}

func (t *TestSuite) AssertBlocksInCacheRootBlock(expectedBlocks []*blocks.Block, expectedRootBlock bool, nodes ...*mock.Node) {
	t.assertBlocksInCacheWithFunc(expectedBlocks, expectedRootBlock, (*blocks.Block).IsRootBlock, nodes...)
}
