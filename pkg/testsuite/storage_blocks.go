package testsuite

import (
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func (f *Framework) AssertStorageBlock(block *model.Block, node *mock.Node) {
	storage := node.Protocol.MainEngineInstance().Storage.Blocks(block.ID().Index())
	require.NotNilf(f.Testing, storage, "%s: storage for %s is nil", node.Name, block.ID().Index())

	loadedBlock, err := storage.Load(block.ID())
	require.NoError(f.Testing, err, "%s: failed to load block %s", node.Name, block)

	require.Equalf(f.Testing, block.ID(), loadedBlock.ID(), "%s: expected %s, got %s", node.Name, block.Block(), loadedBlock.ID())
	require.Equalf(f.Testing, block.Data(), loadedBlock.Data(), "%s: expected %s, got %s", node.Name, block.Data(), loadedBlock.Data())
}

func (f *Framework) AssertStorageBlockExist(block *model.Block, expectedExist bool, node *mock.Node) {
	if expectedExist {
		f.AssertStorageBlock(block, node)
	} else {
		storage := node.Protocol.MainEngineInstance().Storage.Blocks(block.ID().Index())
		if storage == nil {
			return
		}

		loadedBlock, _ := storage.Load(block.ID())
		require.Nil(f.Testing, loadedBlock, "%s: expected block %s to not exist", node.Name, block)
	}
}

func (f *Framework) AssertStorageBlocks(blocks []*model.Block, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, block := range blocks {
			storage := node.Protocol.MainEngineInstance().Storage.Blocks(block.ID().Index())
			require.NotNilf(f.Testing, storage, "%s: storage for %s is nil", node.Name, block.ID().Index())

			loadedBlock, err := storage.Load(block.ID())
			require.NoError(f.Testing, err, "%s: failed to load block %s", node.Name, block)

			require.Equalf(f.Testing, block.ID(), loadedBlock.ID(), "%s: expected %s, got %s", node.Name, block.Block(), loadedBlock.ID())
			require.Equalf(f.Testing, block.Data(), loadedBlock.Data(), "%s: expected %s, got %s", node.Name, block.Data(), loadedBlock.Data())
		}
	}
}
