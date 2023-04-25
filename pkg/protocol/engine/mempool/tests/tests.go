package mempooltests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAll(t *testing.T, frameworkProvider func(*testing.T) *TestFramework) {
	for testName, testCase := range map[string]func(*testing.T, *TestFramework){
		"TestProcessTransaction":            TestProcessTransaction,
		"TestProcessTransactionsOutOfOrder": TestProcessTransactionsOutOfOrder,
	} {
		t.Run(testName, func(t *testing.T) { testCase(t, frameworkProvider(t)) })
	}
}

func TestProcessTransaction(t *testing.T, tf *TestFramework) {
	tf.CreateTransaction("tx1", []string{"genesis"}, 1)
	tf.CreateTransaction("tx2", []string{"tx1:0"}, 1)

	require.NoError(t, tf.ProcessTransactions("tx1", "tx2"))

	tf.RequireBooked("tx1", "tx2")
}

func TestProcessTransactionsOutOfOrder(t *testing.T, tf *TestFramework) {
	tf.CreateTransaction("tx1", []string{"genesis"}, 1)
	tf.CreateTransaction("tx2", []string{"tx1:0"}, 1)
	tf.CreateTransaction("tx3", []string{"tx2:0"}, 1)

	require.NoError(t, tf.ProcessTransactions("tx3"))
	require.NoError(t, tf.ProcessTransactions("tx2"))
	require.NoError(t, tf.ProcessTransactions("tx1"))

	tf.RequireBooked("tx1", "tx2", "tx3")
}
