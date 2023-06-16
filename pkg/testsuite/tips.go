package testsuite

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func (t *TestSuite) AssertStrongTips(expectedBlocks []*blocks.Block, nodes ...*mock.Node) {
	mustNodes(nodes)

	expectedBlockIDs := lo.Map(expectedBlocks, (*blocks.Block).ID)

	for _, node := range nodes {
		t.Eventually(func() error {
			storedTipsBlocks := node.Protocol.MainEngineInstance().TipManager.StrongTips()
			storedTipsBlockIDs := lo.Map(storedTipsBlocks, tipmanager.TipMetadata.ID)

			if !assert.ElementsMatch(t.fakeTesting, expectedBlockIDs, storedTipsBlockIDs) {
				return errors.Errorf("AssertTips: %s: expected %s, got %s", node.Name, expectedBlockIDs, storedTipsBlockIDs)
			}

			return nil
		})
	}
}
