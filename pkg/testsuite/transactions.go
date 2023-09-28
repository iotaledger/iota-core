package testsuite

import (
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertTransaction(signedTransaction *iotago.SignedTransaction, node *mock.Node) mempool.Transaction {
	var loadedTransactionMetadata mempool.TransactionMetadata
	signedTransactionID, err := signedTransaction.ID()
	require.NoError(t.Testing, err)

	t.Eventually(func() error {
		var exists bool
		loadedTransactionMetadata, exists = node.Protocol.MainEngineInstance().Ledger.TransactionMetadata(signedTransactionID)
		if !exists {
			return ierrors.Errorf("AssertTransaction: %s: signedTransaction %s does not exist", node.Name, signedTransactionID)
		}

		if signedTransactionID != loadedTransactionMetadata.ID() {
			return ierrors.Errorf("AssertTransaction: %s: expected ID %s, got %s", node.Name, signedTransactionID, loadedTransactionMetadata.ID())
		}

		//nolint: forcetypeassert // we are in a test and want to assert it anyway
		if !cmp.Equal(signedTransaction.Transaction, loadedTransactionMetadata.Transaction().(*iotago.SignedTransaction).Transaction) {
			return ierrors.Errorf("AssertTransaction: %s: expected %s, got %s", node.Name, signedTransaction, loadedTransactionMetadata.Transaction())
		}

		return nil
	})

	return loadedTransactionMetadata.Transaction()
}

func (t *TestSuite) AssertTransactionsExist(transactions []*iotago.SignedTransaction, expectedExist bool, nodes ...*mock.Node) {
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
							return ierrors.Errorf("AssertTransactionsExist: %s: transaction %s exists but should not", node.Name, transactionID)
						}

						return nil
					})
				}

				return nil
			})
		}
	}
}

func (t *TestSuite) assertTransactionsInCacheWithFunc(expectedTransactions []*iotago.SignedTransaction, expectedPropertyState bool, propertyFunc func(mempool.TransactionMetadata) bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, transaction := range expectedTransactions {
			transactionID, err := transaction.ID()
			require.NoError(t.Testing, err)

			t.Eventually(func() error {
				blockFromCache, exists := node.Protocol.MainEngineInstance().Ledger.TransactionMetadata(transactionID)
				if !exists {
					return ierrors.Errorf("assertTransactionsInCacheWithFunc: %s: transaction %s does not exist", node.Name, transactionID)
				}

				if expectedPropertyState != propertyFunc(blockFromCache) {
					return ierrors.Errorf("assertTransactionsInCacheWithFunc: %s: transaction %s: expected %v, got %v", node.Name, blockFromCache.ID(), expectedPropertyState, propertyFunc(blockFromCache))
				}

				return nil
			})

			t.AssertTransaction(transaction, node)
		}
	}
}

func (t *TestSuite) AssertTransactionsInCacheAccepted(expectedTransactions []*iotago.SignedTransaction, expectedFlag bool, nodes ...*mock.Node) {
	t.assertTransactionsInCacheWithFunc(expectedTransactions, expectedFlag, mempool.TransactionMetadata.IsAccepted, nodes...)
}

func (t *TestSuite) AssertTransactionsInCacheRejected(expectedTransactions []*iotago.SignedTransaction, expectedFlag bool, nodes ...*mock.Node) {
	t.assertTransactionsInCacheWithFunc(expectedTransactions, expectedFlag, mempool.TransactionMetadata.IsRejected, nodes...)
}

func (t *TestSuite) AssertTransactionsInCacheBooked(expectedTransactions []*iotago.SignedTransaction, expectedFlag bool, nodes ...*mock.Node) {
	t.assertTransactionsInCacheWithFunc(expectedTransactions, expectedFlag, mempool.TransactionMetadata.IsBooked, nodes...)
}

func (t *TestSuite) AssertTransactionsInCacheConflicting(expectedTransactions []*iotago.SignedTransaction, expectedFlag bool, nodes ...*mock.Node) {
	t.assertTransactionsInCacheWithFunc(expectedTransactions, expectedFlag, mempool.TransactionMetadata.IsConflicting, nodes...)
}

func (t *TestSuite) AssertTransactionsInCacheInvalid(expectedTransactions []*iotago.SignedTransaction, expectedFlag bool, nodes ...*mock.Node) {
	t.assertTransactionsInCacheWithFunc(expectedTransactions, expectedFlag, mempool.TransactionMetadata.IsInvalid, nodes...)
}

func (t *TestSuite) AssertTransactionsInCachePending(expectedTransactions []*iotago.SignedTransaction, expectedFlag bool, nodes ...*mock.Node) {
	t.assertTransactionsInCacheWithFunc(expectedTransactions, expectedFlag, mempool.TransactionMetadata.IsPending, nodes...)
}

func (t *TestSuite) AssertTransactionInCacheConflicts(transactionConflicts map[*iotago.SignedTransaction][]string, nodes ...*mock.Node) {
	for _, node := range nodes {
		for transaction, conflictAliases := range transactionConflicts {
			transactionID, err := transaction.ID()
			require.NoError(t.Testing, err)

			t.Eventually(func() error {
				transactionFromCache, exists := node.Protocol.MainEngineInstance().Ledger.TransactionMetadata(transactionID)
				if !exists {
					return ierrors.Errorf("AssertTransactionInCacheConflicts: %s: block %s does not exist", node.Name, transactionID)
				}

				expectedConflictIDs := ds.NewSet(lo.Map(conflictAliases, t.TransactionFramework.SignedTransactionID)...)
				actualConflictIDs := transactionFromCache.ConflictIDs()

				if expectedConflictIDs.Size() != actualConflictIDs.Size() {
					return ierrors.Errorf("AssertTransactionInCacheConflicts: %s: transaction %s conflict count incorrect: expected conflicts %v, got %v", node.Name, transactionFromCache.ID(), expectedConflictIDs, actualConflictIDs)
				}

				if !actualConflictIDs.HasAll(expectedConflictIDs) {
					return ierrors.Errorf("AssertTransactionInCacheConflicts: %s: transaction %s: expected conflicts %v, got %v", node.Name, transactionFromCache.ID(), expectedConflictIDs, actualConflictIDs)
				}

				return nil
			})

			t.AssertTransaction(transaction, node)
		}
	}
}
