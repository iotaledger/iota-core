package testsuite

import (
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func (t *TestSuite) AssertStorageBlock(block *model.Block, node *mock.Node) {
	storage := node.Protocol.MainEngineInstance().Storage.Blocks(block.ID().Index())
	require.NotNilf(t.Testing, storage, "%s: storage for %s is nil", node.Name, block.ID().Index())

	loadedBlock, err := storage.Load(block.ID())
	require.NoError(t.Testing, err, "%s: failed to load block %s", node.Name, block)

	require.Equalf(t.Testing, block.ID(), loadedBlock.ID(), "%s: expected %s, got %s", node.Name, block.Block(), loadedBlock.ID())
	require.Equalf(t.Testing, block.Data(), loadedBlock.Data(), "%s: expected %s, got %s", node.Name, block.Data(), loadedBlock.Data())
}

func (t *TestSuite) AssertStorageBlockExist(block *model.Block, expectedExist bool, node *mock.Node) {
	if expectedExist {
		t.AssertStorageBlock(block, node)
	} else {
		storage := node.Protocol.MainEngineInstance().Storage.Blocks(block.ID().Index())
		if storage == nil {
			return
		}

		loadedBlock, _ := storage.Load(block.ID())
		require.Nil(t.Testing, loadedBlock, "%s: expected block %s to not exist", node.Name, block)
	}
}

func (t *TestSuite) AssertStorageBlocks(blocks []*model.Block, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, block := range blocks {
			storage := node.Protocol.MainEngineInstance().Storage.Blocks(block.ID().Index())
			require.NotNilf(t.Testing, storage, "%s: storage for %s is nil", node.Name, block.ID().Index())

			loadedBlock, err := storage.Load(block.ID())
			require.NoError(t.Testing, err, "%s: failed to load block %s", node.Name, block)

			require.Equalf(t.Testing, block.ID(), loadedBlock.ID(), "%s: expected %s, got %s", node.Name, block.Block(), loadedBlock.ID())
			require.Equalf(t.Testing, block.Data(), loadedBlock.Data(), "%s: expected %s, got %s", node.Name, block.Data(), loadedBlock.Data())
		}
	}
}
