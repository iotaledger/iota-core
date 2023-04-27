package testsuite

import (
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertSybilProtectionCommittee(weightVector map[iotago.AccountID]int64, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.assertSybilProtectionCustomCommittee(weightVector, node.Protocol.MainEngineInstance().SybilProtection.Committee, node)
	}
}

func (t *TestSuite) AssertSybilProtectionOnlineCommittee(weightVector map[iotago.AccountID]int64, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.assertSybilProtectionCustomCommittee(weightVector, node.Protocol.MainEngineInstance().SybilProtection.OnlineCommittee, node)
	}
}

func (t *TestSuite) assertSybilProtectionCustomCommittee(weightVector map[iotago.AccountID]int64, committeeFunc func() *account.SelectedAccounts[iotago.AccountID, *iotago.AccountID], node *mock.Node) {
	require.ElementsMatchf(t.Testing, lo.Keys(weightVector), committeeFunc().Members().Slice(), "AssertSybilProtectionCommittee: %s: expected %d, got %d", node.Name, lo.Keys(weightVector), committeeFunc().Members().Slice())
	require.Equalf(t.Testing, lo.Sum(lo.Values(weightVector)...), committeeFunc().TotalWeight(), "AssertSybilProtectionCommittee: %s: expected %d, got %d", node.Name, lo.Sum(lo.Values(weightVector)...), committeeFunc().TotalWeight())

	require.NoError(t.Testing, committeeFunc().ForEach(func(accountID iotago.AccountID, weight int64) error {
		require.Equalf(t.Testing, weightVector[accountID], weight, "AssertSybilProtectionCommittee: %s: expected %d, got %d", node.Name, weightVector[accountID], weight)
		return nil
	}))
}
