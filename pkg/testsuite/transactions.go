package testsuite

import (
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertTransaction(transaction *iotago.Transaction, node *mock.Node) mempool.Transaction {
	var loadedTransaction mempool.TransactionMetadata
	transactionID, err := transaction.ID()
	require.NoError(t.Testing, err)

	t.Eventually(func() error {
		var exists bool
		loadedTransaction, exists = node.Protocol.MainEngineInstance().Ledger.TransactionMetadata(transactionID)
		if !exists {
			return errors.Errorf("AssertTransaction: %s: transaction %s does not exist", node.Name, transactionID)
		}

		if transactionID != loadedTransaction.ID() {
			return errors.Errorf("AssertTransaction: %s: expected %s, got %s", node.Name, transactionID, loadedTransaction.ID())
		}
		if !cmp.Equal(transaction, loadedTransaction.Transaction()) {
			return errors.Errorf("AssertTransaction: %s: expected %s, got %s", node.Name, transaction, loadedTransaction)
		}

		return nil
	})

	return loadedTransaction.Transaction()
}

func (t *TestSuite) AssertTransactionsExist(transactions []*iotago.Transaction, expectedExist bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, transaction := range transactions {
			transactionID, err := transaction.ID()
			require.NoError(t.Testing, err)

			t.Eventually(func() error {
				if expectedExist {
					t.AssertTransaction(transaction, node)
				} else {
					t.Eventually(func() error {
						if lo.Return2(node.Protocol.MainEngineInstance().Ledger.TransactionMetadata(transactionID)) {
							return errors.Errorf("AssertTransactionsExist: %s: transaction %s exists but should not", node.Name, transactionID)
						}

						return nil
					})
				}

				return nil
			})
		}
	}
}

func (t *TestSuite) assertTransactionsInCacheWithFunc(expectedTransactions []*iotago.Transaction, expectedPropertyState bool, propertyFunc func(mempool.TransactionMetadata) bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, transaction := range expectedTransactions {
			transactionID, err := transaction.ID()
			require.NoError(t.Testing, err)

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

			t.AssertTransaction(transaction, node)
		}
	}
}

func (t *TestSuite) AssertTransactionsInCacheAccepted(expectedTransactions []*iotago.Transaction, expectedFlag bool, nodes ...*mock.Node) {
	t.assertTransactionsInCacheWithFunc(expectedTransactions, expectedFlag, mempool.TransactionMetadata.IsAccepted, nodes...)
}

func (t *TestSuite) AssertTransactionsInCacheRejected(expectedTransactions []*iotago.Transaction, expectedFlag bool, nodes ...*mock.Node) {
	t.assertTransactionsInCacheWithFunc(expectedTransactions, expectedFlag, mempool.TransactionMetadata.IsRejected, nodes...)
}

func (t *TestSuite) AssertTransactionsInCacheBooked(expectedTransactions []*iotago.Transaction, expectedFlag bool, nodes ...*mock.Node) {
	t.assertTransactionsInCacheWithFunc(expectedTransactions, expectedFlag, mempool.TransactionMetadata.IsBooked, nodes...)
}

func (t *TestSuite) AssertTransactionsInCacheConflicting(expectedTransactions []*iotago.Transaction, expectedFlag bool, nodes ...*mock.Node) {
	t.assertTransactionsInCacheWithFunc(expectedTransactions, expectedFlag, mempool.TransactionMetadata.IsConflicting, nodes...)
}

func (t *TestSuite) AssertTransactionsInCacheInvalid(expectedTransactions []*iotago.Transaction, expectedFlag bool, nodes ...*mock.Node) {
	t.assertTransactionsInCacheWithFunc(expectedTransactions, expectedFlag, mempool.TransactionMetadata.IsInvalid, nodes...)
}

func (t *TestSuite) AssertTransactionsInCachePending(expectedTransactions []*iotago.Transaction, expectedFlag bool, nodes ...*mock.Node) {
	t.assertTransactionsInCacheWithFunc(expectedTransactions, expectedFlag, mempool.TransactionMetadata.IsPending, nodes...)
}

func (t *TestSuite) AssertTransactionInCacheConflicts(transactionConflicts map[*iotago.Transaction][]string, nodes ...*mock.Node) {
	for _, node := range nodes {
		for transaction, conflictAliases := range transactionConflicts {
			transactionID, err := transaction.ID()
			require.NoError(t.Testing, err)

			t.Eventually(func() error {
				transactionFromCache, exists := node.Protocol.MainEngineInstance().Ledger.TransactionMetadata(transactionID)
				if !exists {
					return errors.Errorf("AssertTransactionInCacheConflicts: %s: block %s does not exist", node.Name, transactionID)
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

			t.AssertTransaction(transaction, node)
		}
	}
}
