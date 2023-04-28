package mempooltests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/runtime/debug"
)

func TestAll(t *testing.T, frameworkProvider func(*testing.T) *TestFramework) {
	for testName, testCase := range map[string]func(*testing.T, *TestFramework){
		"TestProcessTransaction":            TestProcessTransaction,
		"TestProcessTransactionsOutOfOrder": TestProcessTransactionsOutOfOrder,
		"TestSetInclusionSlot":              TestSetInclusionSlot,
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

func TestSetInclusionSlot(t *testing.T, tf *TestFramework) {
	debug.SetEnabled(true)

	tf.CreateTransaction("tx1", []string{"genesis"}, 1)
	tf.CreateTransaction("tx2", []string{"tx1:0"}, 1)
	tf.CreateTransaction("tx3", []string{"tx2:0"}, 1)

	require.NoError(t, tf.ProcessTransactions("tx3"))
	require.NoError(t, tf.ProcessTransactions("tx2"))
	require.NoError(t, tf.ProcessTransactions("tx1"))

	tf.RequireBooked("tx1", "tx2", "tx3")

	tf.SetTransactionIncluded("tx2", 1)

	time.Sleep(1 * time.Second)

	tf.SetTransactionIncluded("tx1", 1)

	time.Sleep(5 * time.Second)
}
