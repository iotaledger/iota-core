package mempooltests

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/runtime/debug"
)

func TestAll(t *testing.T, frameworkProvider func(*testing.T) *TestFramework) {
	for testName, testCase := range map[string]func(*testing.T, *TestFramework){
		"TestJoinConflictSets": TestJoinConflictSets,
	} {
		t.Run(testName, func(t *testing.T) { testCase(t, frameworkProvider(t)) })
	}
}

func TestJoinConflictSets(t *testing.T, tf *TestFramework) {
	debug.SetEnabled(true)

	require.NoError(t, tf.ProcessTransaction("tx1", []string{"genesis"}, 1))

	tf.RequireBooked("tx1")

	require.NoError(t, tf.ProcessTransaction("tx2", []string{"tx1:0"}, 1))

	tf.RequireBooked("tx2")
}
