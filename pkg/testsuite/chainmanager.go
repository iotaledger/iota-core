package testsuite

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func (t *TestSuite) AssertChainManagerIsSolid(nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			rootCommitment := node.Protocol.ChainManager.RootCommitment()
			chain := node.Protocol.ChainManager.Chain(rootCommitment.ID())
			if chain == nil {
				return errors.Errorf("AssertChainManagerIsSolid: %s: chain is nil", node.Name)
			}

			latestChainCommitment := chain.LatestCommitment()
			latestCommitment := node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment()

			if latestCommitment.ID() != latestChainCommitment.ID() {
				return errors.Errorf("AssertChainManagerIsSolid: %s: latest commitment is not equal, expected %d, got %d", node.Name, latestCommitment.ID(), latestChainCommitment.ID())
			}
			if !latestChainCommitment.IsSolid() {
				return errors.Errorf("AssertChainManagerIsSolid: %s: is not solid", node.Name)
			}

			return nil
		})
	}
}
