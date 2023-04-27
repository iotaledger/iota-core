package testsuite

import (
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (f *Framework) AssertSybilProtectionCommittee(weightVector map[iotago.AccountID]int64, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		f.assertSybilProtectionCustomCommittee(weightVector, node.Protocol.MainEngineInstance().SybilProtection.Committee, node)
	}
}

func (f *Framework) AssertSybilProtectionOnlineCommittee(weightVector map[iotago.AccountID]int64, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		f.assertSybilProtectionCustomCommittee(weightVector, node.Protocol.MainEngineInstance().SybilProtection.OnlineCommittee, node)
	}
}

func (f *Framework) assertSybilProtectionCustomCommittee(weightVector map[iotago.AccountID]int64, committeeFunc func() *account.SelectedAccounts[iotago.AccountID, *iotago.AccountID], node *mock.Node) {
	require.ElementsMatchf(f.Testing, lo.Keys(weightVector), committeeFunc().Members().Slice(), "AssertSybilProtectionCommittee: %s: expected %d, got %d", node.Name, lo.Keys(weightVector), committeeFunc().Members().Slice())
	require.Equalf(f.Testing, lo.Sum(lo.Values(weightVector)...), committeeFunc().TotalWeight(), "AssertSybilProtectionCommittee: %s: expected %d, got %d", node.Name, lo.Sum(lo.Values(weightVector)...), committeeFunc().TotalWeight())

	require.NoError(f.Testing, committeeFunc().ForEach(func(accountID iotago.AccountID, weight int64) error {
		require.Equalf(f.Testing, weightVector[accountID], weight, "AssertSybilProtectionCommittee: %s: expected %d, got %d", node.Name, weightVector[accountID], weight)
		return nil
	}))
}
