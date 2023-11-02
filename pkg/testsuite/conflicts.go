package testsuite

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/core/acceptance"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func (t *TestSuite) AssertConflictsInCacheAcceptanceState(expectedConflictAliases []string, expectedState acceptance.State, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, conflictAlias := range expectedConflictAliases {
			t.Eventually(func() error {
				acceptanceState := node.Protocol.Engines.Main.Get().Ledger.ConflictDAG().AcceptanceState(ds.NewSet(t.TransactionFramework.TransactionID(conflictAlias)))

				if acceptanceState != expectedState {
					return ierrors.Errorf("assertTransactionsInCacheWithFunc: %s: conflict %s is %s, but expected %s", node.Name, conflictAlias, acceptanceState, expectedState)
				}

				return nil
			})
		}
	}
}
