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
				storage, err := node.Protocol.Engines.Main.Get().Storage.RootBlocks(block.ID().Slot())
				if err != nil {
					return ierrors.Errorf("AssertStorageRootBlocks: %s: error loading root blocks for %s: %v", node.Name, block.ID().Slot(), err)
				}

				loadedCommitmentID, exists, err := storage.Load(block.ID())
				if err != nil {
					return ierrors.Wrapf(err, "AssertStorageRootBlocks: %s: failed to load root block %s", node.Name, block.ID())
				}

				if !exists {
					return ierrors.Errorf("AssertStorageRootBlocks: %s: root block %s does not exist", node.Name, block.ID())
				}

				if block.SlotCommitmentID() != loadedCommitmentID {
					return ierrors.Errorf("AssertStorageRootBlocks: %s: expected slot commitment %s, got %s for block %s", node.Name, block.SlotCommitmentID(), loadedCommitmentID, block.ID())
				}

				return nil
			})
		}
	}
}
