package mempooltests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAll(t *testing.T, frameworkProvider func(*testing.T) *TestFramework) {
	for testName, testCase := range map[string]func(*testing.T, *TestFramework){
		"TestProcessTransaction": TestProcessTransaction,
	} {
		t.Run(testName, func(t *testing.T) { testCase(t, frameworkProvider(t)) })
	}
}

func TestProcessTransaction(t *testing.T, tf *TestFramework) {
	require.NoError(t, tf.ProcessTransaction("tx1", []string{"genesis"}, 1))
	require.NoError(t, tf.ProcessTransaction("tx2", []string{"tx1:0"}, 1))
	tf.RequireBooked("tx1", "tx2")
}
