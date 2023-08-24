package testsuite

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func (t *TestSuite) AssertChainManagerIsSolid(nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			rootCommitment := node.Protocol.ChainManager.RootCommitment()
			chain := node.Protocol.ChainManager.Chain(rootCommitment.ID())
			if chain == nil {
				return ierrors.Errorf("AssertChainManagerIsSolid: %s: chain is nil", node.Name)
			}

			latestChainCommitment := chain.LatestCommitment()
			latestCommitment := node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment()

			if latestCommitment.ID() != latestChainCommitment.ID() {
				return ierrors.Errorf("AssertChainManagerIsSolid: %s: latest commitment is not equal, expected %d, got %d", node.Name, latestCommitment.ID(), latestChainCommitment.ID())
			}
			if !latestChainCommitment.SolidEvent().WasTriggered() {
				return ierrors.Errorf("AssertChainManagerIsSolid: %s: is not solid", node.Name)
			}

			return nil
		})
	}
}
