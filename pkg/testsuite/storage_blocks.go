package testsuite

import (
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func (t *TestSuite) AssertStorageBlock(block *model.Block, node *mock.Node) {
	var storage *prunable.Blocks
	t.Eventuallyf(func() bool {
		storage = node.Protocol.MainEngineInstance().Storage.Blocks(block.ID().Index())
		return storage != nil
	}, "AssertStorageBlock: %s: storage for %s is nil", node.Name, block.ID().Index())

	var loadedBlock *model.Block
	var err error
	t.Eventuallyf(func() bool {
		loadedBlock, err = storage.Load(block.ID())
		return err == nil && loadedBlock != nil
	}, "AssertStorageBlock: %s: failed to load block %s: %s", node.Name, block.ID(), err)

	require.Equalf(t.Testing, block.ID(), loadedBlock.ID(), "AssertStorageBlock: %s: expected %s, got %s", node.Name, block.ID(), loadedBlock.ID())
	require.Equalf(t.Testing, block.Data(), loadedBlock.Data(), "AssertStorageBlock: %s: expected %s, got %s", node.Name, block.Data(), loadedBlock.Data())
}

func (t *TestSuite) AssertStorageBlockExist(block *model.Block, expectedExist bool, node *mock.Node) {
	if expectedExist {
		t.AssertStorageBlock(block, node)
	} else {
		t.Eventuallyf(func() bool {
			storage := node.Protocol.MainEngineInstance().Storage.Blocks(block.ID().Index())
			if storage == nil {
				return true
			}

			loadedBlock, _ := storage.Load(block.ID())

			return loadedBlock == nil
		}, "AssertStorageBlockExist: %s: expected block %s to not exist", node.Name, block)
	}
}
