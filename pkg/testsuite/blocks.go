package testsuite

import (
	"context"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertBlock(block *blocks.Block, client mock.Client) *iotago.Block {
	var loadedBlock *iotago.Block
	t.Eventually(func() error {
		var err error
		loadedBlock, err = client.BlockByBlockID(context.Background(), block.ID())
		if err != nil {
			return ierrors.Wrapf(err, "AssertBlock: block %s does not exist", block.ID())
		}

		if block.ID() != loadedBlock.MustID() {
			return ierrors.Errorf("AssertBlock: expected %s, got %s", block.ID(), loadedBlock.MustID())
		}

		return nil
	})

	return loadedBlock
}

func (t *TestSuite) AssertBlocksExist(blocks []*blocks.Block, expectedExist bool, clients ...mock.Client) {
	for _, client := range clients {
		for _, block := range blocks {
			if block.ID() == iotago.EmptyBlockID {
				continue
			}

			if expectedExist {
				t.AssertBlock(block, client)
			} else {
				t.Eventually(func() error {
					_, err := client.BlockByBlockID(context.Background(), block.ID())
					if err == nil {
						return ierrors.Errorf("AssertBlocksExist: block %s exists but should not", block)
					}

					return nil
				})
			}
		}
	}
}

func (t *TestSuite) AssertBlockFiltered(blocks []*blocks.Block, reason error, node *mock.Node) {
	for _, block := range blocks {
		t.Eventually(func() error {
			for _, filteredBlockEvent := range node.FilteredBlocks() {
				if filteredBlockEvent.Block.ID() == block.ID() {
					if ierrors.Is(filteredBlockEvent.Reason, reason) {
						return nil
					}

					return ierrors.Errorf("AssertBlockFiltered: %s: block %s was filtered with reason %s but expected %s", node.Name, block.ID(), filteredBlockEvent.Reason, reason)
				}
			}

			return ierrors.Errorf("AssertBlockFiltered: %s: block %s was not filtered by the PostSolidFilter", node.Name, block.ID())
		})
	}
}

func (t *TestSuite) assertBlocksInCacheWithFunc(expectedBlocks []*blocks.Block, propertyName string, expectedPropertyState bool, propertyFunc func(*blocks.Block) bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, block := range expectedBlocks {
			t.Eventually(func() error {
				blockFromCache, exists := node.Protocol.Engines.Main.Get().BlockFromCache(block.ID())
				if !exists {
					return ierrors.Errorf("assertBlocksInCacheWithFunc[%s]: %s: block %s does not exist", propertyName, node.Name, block.ID())
				}

				if blockFromCache.IsRootBlock() {
					return ierrors.Errorf("assertBlocksInCacheWithFunc[%s]: %s: block %s is root block", propertyName, node.Name, blockFromCache.ID())
				}

				if expectedPropertyState != propertyFunc(blockFromCache) {
					return ierrors.Errorf("assertBlocksInCacheWithFunc[%s]: %s: block %s: expected %v, got %v", propertyName, node.Name, blockFromCache.ID(), expectedPropertyState, propertyFunc(blockFromCache))
				}

				return nil
			})

			t.AssertBlock(block, node.Client)
		}
	}
}

func (t *TestSuite) AssertBlocksInCachePreAccepted(expectedBlocks []*blocks.Block, expectedPreAccepted bool, nodes ...*mock.Node) {
	t.assertBlocksInCacheWithFunc(expectedBlocks, "pre-accepted", expectedPreAccepted, (*blocks.Block).IsPreAccepted, nodes...)
}

func (t *TestSuite) AssertBlocksInCacheAccepted(expectedBlocks []*blocks.Block, expectedAccepted bool, nodes ...*mock.Node) {
	t.assertBlocksInCacheWithFunc(expectedBlocks, "accepted", expectedAccepted, (*blocks.Block).IsAccepted, nodes...)
}

func (t *TestSuite) AssertBlocksInCachePreConfirmed(expectedBlocks []*blocks.Block, expectedPreConfirmed bool, nodes ...*mock.Node) {
	t.assertBlocksInCacheWithFunc(expectedBlocks, "pre-confirmed", expectedPreConfirmed, (*blocks.Block).IsPreConfirmed, nodes...)
}

func (t *TestSuite) AssertBlocksInCacheConfirmed(expectedBlocks []*blocks.Block, expectedConfirmed bool, nodes ...*mock.Node) {
	t.assertBlocksInCacheWithFunc(expectedBlocks, "confirmed", expectedConfirmed, (*blocks.Block).IsConfirmed, nodes...)
}

func (t *TestSuite) AssertBlocksInCacheScheduled(expectedBlocks []*blocks.Block, expectedScheduled bool, nodes ...*mock.Node) {
	t.assertBlocksInCacheWithFunc(expectedBlocks, "scheduled", expectedScheduled, (*blocks.Block).IsScheduled, nodes...)
}

func (t *TestSuite) AssertBlocksInCacheRootBlock(expectedBlocks []*blocks.Block, expectedRootBlock bool, nodes ...*mock.Node) {
	t.assertBlocksInCacheWithFunc(expectedBlocks, "root-block", expectedRootBlock, (*blocks.Block).IsRootBlock, nodes...)
}

func (t *TestSuite) AssertBlocksInCacheBooked(expectedBlocks []*blocks.Block, expectedBooked bool, nodes ...*mock.Node) {
	t.assertBlocksInCacheWithFunc(expectedBlocks, "booked", expectedBooked, (*blocks.Block).IsBooked, nodes...)
}

func (t *TestSuite) AssertBlocksInCacheInvalid(expectedBlocks []*blocks.Block, expectedInvalid bool, nodes ...*mock.Node) {
	t.assertBlocksInCacheWithFunc(expectedBlocks, "valid", expectedInvalid, (*blocks.Block).IsInvalid, nodes...)
}

func (t *TestSuite) AssertBlocksInCacheConflicts(blockConflicts map[*blocks.Block][]string, nodes ...*mock.Node) {
	for _, node := range nodes {
		for block, conflictAliases := range blockConflicts {
			t.Eventually(func() error {
				blockFromCache, exists := node.Protocol.Engines.Main.Get().BlockFromCache(block.ID())
				if !exists {
					return ierrors.Errorf("AssertBlocksInCacheConflicts: %s: block %s does not exist", node.Name, block.ID())
				}

				if blockFromCache.IsRootBlock() {
					return ierrors.Errorf("AssertBlocksInCacheConflicts: %s: block %s is root block", node.Name, blockFromCache.ID())
				}

				expectedSpenderIDs := ds.NewSet(lo.Map(conflictAliases, t.DefaultWallet().TransactionID)...)
				actualSpenderIDs := blockFromCache.SpenderIDs()

				if expectedSpenderIDs.Size() != actualSpenderIDs.Size() {
					return ierrors.Errorf("AssertBlocksInCacheConflicts: %s: block %s conflict count incorrect: expected conflicts %v, got %v", node.Name, blockFromCache.ID(), expectedSpenderIDs, actualSpenderIDs)
				}

				if !actualSpenderIDs.HasAll(expectedSpenderIDs) {
					return ierrors.Errorf("AssertBlocksInCacheConflicts: %s: block %s: expected conflicts %v, got %v", node.Name, blockFromCache.ID(), expectedSpenderIDs, actualSpenderIDs)
				}

				return nil
			})
		}
	}
}
