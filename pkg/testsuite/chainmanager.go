package testsuite

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func (t *TestSuite) AssertChainManagerIsSolid(nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			chain := node.Protocol.Chains.Main.Get()
			if chain == nil {
				return ierrors.Errorf("AssertChainManagerIsSolid: %s: chain is nil", node.Name)
			}

			latestChainCommitment := chain.LatestCommitment.Get()
			latestCommitment := node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment()

			if latestCommitment.ID() != latestChainCommitment.ID() {
				return ierrors.Errorf("AssertChainManagerIsSolid: %s: latest commitment is not equal, expected %s, got %s", node.Name, latestCommitment.ID(), latestChainCommitment.ID())
			}

			return nil
		})
	}
}
