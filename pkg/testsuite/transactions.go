package testsuite

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertTransactionsExist(transactionAliases []string, expectedExist bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, transactionAlias := range transactionAliases {
			t.Eventually(func() error {
				actuallyExists := lo.Return2(node.Protocol.MainEngineInstance().Ledger.TransactionMetadata(t.TransactionFramework.TransactionID(transactionAlias)))
				if actuallyExists && !expectedExist {
					return errors.Errorf("AssertTransactionsExist: %s: transaction %s exists but should not", node.Name, transactionAlias)
				}
				if !actuallyExists && expectedExist {
					return errors.Errorf("AssertTransactionsExist: %s: transaction %s does not exists but should", node.Name, transactionAlias)
				}

				return nil
			})
		}
	}
}

func (t *TestSuite) assertTransactionsInCacheWithFunc(expectedTransactions []iotago.TransactionID, expectedPropertyState bool, propertyFunc func(mempool.TransactionMetadata) bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, transactionID := range expectedTransactions {
			t.Eventually(func() error {
				blockFromCache, exists := node.Protocol.MainEngineInstance().Ledger.TransactionMetadata(transactionID)
				if !exists {
					return errors.Errorf("assertTransactionsInCacheWithFunc: %s: transaction %s does not exist", node.Name, transactionID)
				}

				if expectedPropertyState != propertyFunc(blockFromCache) {
					return errors.Errorf("assertTransactionsInCacheWithFunc: %s: transaction %s: expected %v, got %v", node.Name, blockFromCache.ID(), expectedPropertyState, propertyFunc(blockFromCache))
				}

				return nil
			})
		}
	}
}

func (t *TestSuite) AssertTransactionsInCacheAccepted(expectedTransactions []string, expectedFlag bool, nodes ...*mock.Node) {
	t.assertTransactionsInCacheWithFunc(lo.Map(expectedTransactions, t.TransactionFramework.TransactionID), expectedFlag, mempool.TransactionMetadata.IsAccepted, nodes...)
}

func (t *TestSuite) AssertTransactionsInCacheRejected(expectedTransactions []string, expectedFlag bool, nodes ...*mock.Node) {
	t.assertTransactionsInCacheWithFunc(lo.Map(expectedTransactions, t.TransactionFramework.TransactionID), expectedFlag, mempool.TransactionMetadata.IsRejected, nodes...)
}

func (t *TestSuite) AssertTransactionsInCacheBooked(expectedTransactions []string, expectedFlag bool, nodes ...*mock.Node) {
	t.assertTransactionsInCacheWithFunc(lo.Map(expectedTransactions, t.TransactionFramework.TransactionID), expectedFlag, mempool.TransactionMetadata.IsBooked, nodes...)
}

func (t *TestSuite) AssertTransactionsInCacheConflicting(expectedTransactions []string, expectedFlag bool, nodes ...*mock.Node) {
	t.assertTransactionsInCacheWithFunc(lo.Map(expectedTransactions, t.TransactionFramework.TransactionID), expectedFlag, mempool.TransactionMetadata.IsConflicting, nodes...)
}

func (t *TestSuite) AssertTransactionsInCacheInvalid(expectedTransactions []string, expectedFlag bool, nodes ...*mock.Node) {
	t.assertTransactionsInCacheWithFunc(lo.Map(expectedTransactions, t.TransactionFramework.TransactionID), expectedFlag, mempool.TransactionMetadata.IsInvalid, nodes...)
}

func (t *TestSuite) AssertTransactionsInCachePending(expectedTransactions []string, expectedFlag bool, nodes ...*mock.Node) {
	t.assertTransactionsInCacheWithFunc(lo.Map(expectedTransactions, t.TransactionFramework.TransactionID), expectedFlag, mempool.TransactionMetadata.IsPending, nodes...)
}

func (t *TestSuite) AssertTransactionInCacheConflicts(transactionConflicts map[string][]string, nodes ...*mock.Node) {
	for _, node := range nodes {
		for transactionAlias, conflictAliases := range transactionConflicts {
			t.Eventually(func() error {
				transactionFromCache, exists := node.Protocol.MainEngineInstance().Ledger.TransactionMetadata(t.TransactionFramework.TransactionID(transactionAlias))
				if !exists {
					return errors.Errorf("AssertTransactionInCacheConflicts: %s: block %s does not exist", node.Name, transactionAlias)
				}

				expectedConflictIDs := advancedset.New(lo.Map(conflictAliases, t.TransactionFramework.TransactionID)...)
				actualConflictIDs := transactionFromCache.ConflictIDs().Get()

				if expectedConflictIDs.Size() != actualConflictIDs.Size() {
					return errors.Errorf("AssertTransactionInCacheConflicts: %s: transaction %s conflict count incorrect: expected conflicts %v, got %v", node.Name, transactionFromCache.ID(), expectedConflictIDs, actualConflictIDs)
				}

				if !actualConflictIDs.HasAll(expectedConflictIDs) {
					return errors.Errorf("AssertTransactionInCacheConflicts: %s: transaction %s: expected conflicts %v, got %v", node.Name, transactionFromCache.ID(), expectedConflictIDs, actualConflictIDs)
				}

				return nil
			})

		}
	}
}
