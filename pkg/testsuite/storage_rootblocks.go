package testsuite

import (
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertStorageRootBlocks(blocks []*model.Block, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, block := range blocks {
			var storage *prunable.RootBlocks
			t.Eventuallyf(func() bool {
				storage = node.Protocol.MainEngineInstance().Storage.RootBlocks(block.ID().Index())
				return storage != nil
			}, "AssertStorageRootBlocks: %s: storage for %s is nil", node.Name, block.ID().Index())

			var loadedBlockID iotago.BlockID
			var commitmentID iotago.CommitmentID
			var err error
			t.Eventuallyf(func() bool {
				loadedBlockID, commitmentID, err = storage.Load(block.ID())
				return err == nil
			}, "AssertStorageRootBlocks: %s: failed to load root block %s: %s", node.Name, block.ID(), err)

			t.Eventuallyf(func() bool {
				return block.ID() == loadedBlockID && block.SlotCommitment().ID() == commitmentID
			}, "AssertStorageRootBlocks: %s: block %s expected %s, got %s; slot commitment expected %s, got %s", node.Name, block.ID(), block.ID(), loadedBlockID, block.SlotCommitment().ID(), commitmentID)
		}
	}
}
