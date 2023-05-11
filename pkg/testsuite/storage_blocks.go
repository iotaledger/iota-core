package testsuite

import (
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"

	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func (t *TestSuite) AssertStorageBlock(block *model.Block, node *mock.Node) {
	t.Eventually(func() error {
		storage := node.Protocol.MainEngineInstance().Storage.Blocks(block.ID().Index())
		if storage == nil {
			return errors.Errorf("AssertStorageBlock: %s: storage for %s is nil", node.Name, block.ID().Index())
		}

		loadedBlock, err := storage.Load(block.ID())
		if err != nil {
			return errors.Wrapf(err, "AssertStorageBlock: %s: error loading block %s", node.Name, block.ID())
		}

		if block.ID() != loadedBlock.ID() {
			return errors.Errorf("AssertStorageBlock: %s: expected %s, got %s", node.Name, block.ID(), loadedBlock.ID())
		}

		if cmp.Equal(block.Data(), loadedBlock.Data()) {
			return errors.Errorf("AssertStorageBlock: %s: expected %s, got %s", node.Name, block.Data(), loadedBlock.Data())
		}

		return nil
	})
}

func (t *TestSuite) AssertStorageBlockExist(block *model.Block, expectedExist bool, node *mock.Node) {
	if expectedExist {
		t.AssertStorageBlock(block, node)
	} else {
		t.Eventually(func() error {
			storage := node.Protocol.MainEngineInstance().Storage.Blocks(block.ID().Index())
			if storage == nil {
				return nil
			}

			loadedBlock, _ := storage.Load(block.ID())
			if loadedBlock != nil {
				return errors.Errorf("AssertStorageBlockExist: %s: expected block %s to not exist", node.Name, block)
			}

			return nil
		})
	}
}
