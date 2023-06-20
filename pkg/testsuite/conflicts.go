package testsuite

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/iota-core/pkg/core/acceptance"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
)

func (t *TestSuite) AssertConflictsInCacheAcceptanceState(expectedConflictAliases []string, expectedState acceptance.State, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, conflictAlias := range expectedConflictAliases {
			t.Eventually(func() error {
				acceptanceState := node.Protocol.MainEngineInstance().Ledger.ConflictDAG().AcceptanceState(advancedset.New(t.TransactionFramework.TransactionID(conflictAlias)))

				if acceptanceState != expectedState {
					return errors.Errorf("assertTransactionsInCacheWithFunc: %s: conflict %s is %s, but expected %s", node.Name, conflictAlias, acceptanceState, expectedState)
				}

				return nil
			})
		}
	}
}
