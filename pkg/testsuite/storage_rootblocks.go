package testsuite

import (
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func (t *TestSuite) AssertStorageRootBlocks(blocks []*model.Block, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, block := range blocks {
			storage := node.Protocol.MainEngineInstance().Storage.RootBlocks(block.ID().Index())
			require.NotNilf(t.Testing, storage, "%s: storage for %s is nil", node.Name, block.ID().Index())

			loadedBlockID, commitmentID, err := storage.Load(block.ID())
			require.NoError(t.Testing, err, "%s: failed to load root block %s", node.Name, block)

			require.Equalf(t.Testing, block.ID(), loadedBlockID, "%s: expected %s, got %s", node.Name, block.Block(), loadedBlockID)
			// TODO: replace with block.Commitment()
			require.Equalf(t.Testing, block.Block().SlotCommitment.MustID(), commitmentID, "%s: expected %s, got %s", node.Name, block.Block().SlotCommitment.MustID(), commitmentID)
		}
	}
}
