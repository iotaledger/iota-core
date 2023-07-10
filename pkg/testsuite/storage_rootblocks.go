package testsuite

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func (t *TestSuite) AssertStorageRootBlocks(blocks []*blocks.Block, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, block := range blocks {
			t.Eventually(func() error {
				storage := node.Protocol.MainEngineInstance().Storage.RootBlocks(block.ID().Index())
				if storage == nil {
					return ierrors.Errorf("AssertStorageRootBlocks: %s: storage for %s is nil", node.Name, block.ID().Index())
				}

				loadedBlockID, loadedCommitmentID, err := storage.Load(block.ID())
				if err != nil {
					return ierrors.Wrapf(err, "AssertStorageRootBlocks: %s: failed to load root block %s", node.Name, block.ID())
				}

				if block.ID() != loadedBlockID {
					return ierrors.Errorf("AssertStorageRootBlocks: %s: expected block %s, got %s", node.Name, block.ID(), loadedBlockID)
				}

				if block.SlotCommitmentID() != loadedCommitmentID {
					return ierrors.Errorf("AssertStorageRootBlocks: %s: expected slot commitment %s, got %s for block %s", node.Name, block.SlotCommitmentID(), loadedCommitmentID, block.ID())
				}

				return nil
			})
		}
	}
}
