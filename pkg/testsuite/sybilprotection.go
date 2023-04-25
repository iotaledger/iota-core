package testsuite

import (
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func (f *Framework) AssertSybilProtection(totalWeight int64, onlineWeight int64, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		require.Equalf(f.Testing, totalWeight, node.Protocol.MainEngineInstance().SybilProtection.Committee().TotalWeight(), "%s: expected %d, got %d", node.Name, totalWeight, node.Protocol.MainEngineInstance().SybilProtection.Committee().TotalWeight())
		require.Equalf(f.Testing, onlineWeight, node.Protocol.MainEngineInstance().SybilProtection.OnlineCommittee().TotalWeight(), "%s: expected %d, got %d", node.Name, onlineWeight, node.Protocol.MainEngineInstance().SybilProtection.OnlineCommittee().TotalWeight())
	}
}
