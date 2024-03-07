package testsuite

import (
	"context"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota.go/v4/api"
)

func (t *TestSuite) AssertRetainerBlocksState(expectedBlocks []*blocks.Block, expectedState api.BlockState, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, block := range expectedBlocks {
			t.Eventually(func() error {
				blockFromRetainer, err := node.Client.BlockMetadataByBlockID(context.Background(), block.ID())
				if err != nil {
					return ierrors.Errorf("AssertRetainerBlocksState: %s: block %s: error when loading %s", node.Name, block.ID(), err.Error())
				}

				if expectedState != blockFromRetainer.BlockState {
					return ierrors.Errorf("AssertRetainerBlocksState: %s: block %s: expected %s, got %s", node.Name, block.ID(), expectedState, blockFromRetainer.BlockState)
				}

				return nil
			})

			t.AssertBlock(block, node.Client)
		}
	}
}
