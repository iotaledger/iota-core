package testsuite

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertStrongTips(expectedBlocks []*blocks.Block, nodes ...*mock.Node) {
	mustNodes(nodes)

	expectedBlockIDs := lo.Map(expectedBlocks, func(block *blocks.Block) iotago.BlockID {
		return block.ID()
	})

	for _, node := range nodes {
		t.Eventually(func() error {
			storedTipsBlocks := node.Protocol.MainEngineInstance().TipManager.StrongTipSet()
			storedTipsBlockIDs := lo.Map(storedTipsBlocks, func(block *blocks.Block) iotago.BlockID {
				return block.ID()
			})

			if !assert.ElementsMatch(t.fakeTesting, expectedBlockIDs, storedTipsBlockIDs) {
				return errors.Errorf("AssertTips: %s: expected %s, got %s", node.Name, expectedBlockIDs, storedTipsBlockIDs)
			}

			return nil
		})
	}
}
