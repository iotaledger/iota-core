package testsuite

import (
	"github.com/stretchr/testify/assert"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func (t *TestSuite) AssertStorageBlock(block *model.Block, node *mock.Node) {
	t.Eventually(func() error {
		storage, err := node.Protocol.Engines.Main.Get().Storage.Blocks(block.ID().Slot())
		if err != nil {
			return ierrors.Errorf("AssertStorageBlock: %s: storage for %s is nil", node.Name, block.ID().Slot())
		}

		loadedBlock, err := storage.Load(block.ID())
		if err != nil {
			return ierrors.Wrapf(err, "AssertStorageBlock: %s: error loading block %s", node.Name, block.ID())
		}

		if block.ID() != loadedBlock.ID() {
			return ierrors.Errorf("AssertStorageBlock: %s: expected %s, got %s", node.Name, block.ID(), loadedBlock.ID())
		}

		if assert.Equal(t.fakeTesting, block.Data(), loadedBlock.Data()) {
			return ierrors.Errorf("AssertStorageBlock: %s: expected %s, got %s", node.Name, block.Data(), loadedBlock.Data())
		}

		return nil
	})
}

func (t *TestSuite) AssertStorageBlockExist(block *model.Block, expectedExist bool, node *mock.Node) {
	if expectedExist {
		t.AssertStorageBlock(block, node)
	} else {
		t.Eventually(func() error {
			storage, err := node.Protocol.Engines.Main.Get().Storage.Blocks(block.ID().Slot())
			if err != nil {
				//nolint:nilerr // expected behavior
				return nil
			}

			loadedBlock, _ := storage.Load(block.ID())
			if loadedBlock != nil {
				return ierrors.Errorf("AssertStorageBlockExist: %s: expected block %s to not exist", node.Name, block)
			}

			return nil
		})
	}
}
