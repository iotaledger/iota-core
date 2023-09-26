package conflictdagv1

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/runtime/memanalyzer"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag/tests"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	TestTransactionCreationSlot = 0
)

// TestConflictDAG runs the generic tests for the ConflictDAG.
func TestConflictDAG(t *testing.T) {
	tests.TestAll(t, newTestFramework)
}

// newTestFramework creates a new instance of the TestFramework for internal unit tests.
func newTestFramework(t *testing.T) *tests.Framework {
	accountsTestFramework := tests.NewAccountsTestFramework(t, account.NewAccounts())

	return tests.NewFramework(
		t,
		New[iotago.TransactionID, iotago.OutputID, vote.MockedRank](accountsTestFramework.Committee.SeatCount),
		accountsTestFramework,
		transactionID,
		outputID,
	)
}

// transactionID creates a (made up) TransactionID from the given alias.
func transactionID(alias string) iotago.TransactionID {
	result := iotago.TransactionIDFromData(TestTransactionCreationSlot, []byte(alias))
	result.RegisterAlias(alias)

	return result
}

// outputID creates a (made up) OutputID from the given alias.
func outputID(alias string) iotago.OutputID {
	return iotago.OutputIDFromTransactionIDAndIndex(iotago.TransactionIDFromData(TestTransactionCreationSlot, []byte(alias)), 1)
}

func TestMemoryRelease(t *testing.T) {
	//t.Skip("skip memory test as for some reason it's failing")
	tf := newTestFramework(t)

	createConflictSets := func(startIndex, conflictSetCount, evictionDelay, conflictsInConflictSet int, prevConflictSetAlias string) (int, string) {
		index := startIndex
		for ; index < startIndex+conflictSetCount; index++ {
			conflictSetAlias := fmt.Sprintf("conflictSet-%d", index)
			for conflictIndex := 0; conflictIndex < conflictsInConflictSet; conflictIndex++ {
				conflictAlias := fmt.Sprintf("conflictSet-%d:%d", index, conflictIndex)
				require.NoError(t, tf.CreateOrUpdateConflict(conflictAlias, []string{conflictSetAlias}))
				if prevConflictSetAlias != "" {
					require.NoError(t, tf.UpdateConflictParents(conflictAlias, []string{fmt.Sprintf("%s:%d", prevConflictSetAlias, 0)}, []string{}))
				}
			}
			prevConflictSetAlias = conflictSetAlias

			if indexToEvict := index - evictionDelay; indexToEvict >= 0 {
				for conflictIndex := 0; conflictIndex < conflictsInConflictSet; conflictIndex++ {
					conflictAlias := fmt.Sprintf("conflictSet-%d:%d", indexToEvict, conflictIndex)
					tf.EvictConflict(conflictAlias)
				}
			}
		}

		return index, prevConflictSetAlias
	}
	_, prevAlias := createConflictSets(0, 30000, 1, 2, "")

	tf.Instance.EvictConflict(tf.ConflictID(prevAlias + ":0"))
	tf.Instance.EvictConflict(tf.ConflictID(prevAlias + ":1"))

	iotago.UnregisterIdentifierAliases()

	fmt.Println("Memory report before:")
	fmt.Println(memanalyzer.MemoryReport(tf))
	memStatsStart := memanalyzer.MemSize(tf)
	_, alias := createConflictSets(0, 30000, 1, 2, "")

	tf.Instance.EvictConflict(tf.ConflictID(alias + ":0"))
	tf.Instance.EvictConflict(tf.ConflictID(alias + ":1"))

	tf.Instance.Shutdown()

	iotago.UnregisterIdentifierAliases()

	time.Sleep(time.Second)

	require.Equal(t, 0, tf.Instance.(*ConflictDAG[iotago.TransactionID, iotago.OutputID, vote.MockedRank]).conflictSetsByID.Size())
	require.Equal(t, 0, tf.Instance.(*ConflictDAG[iotago.TransactionID, iotago.OutputID, vote.MockedRank]).conflictsByID.Size())
	require.Equal(t, 0, tf.Instance.(*ConflictDAG[iotago.TransactionID, iotago.OutputID, vote.MockedRank]).conflictUnhooks.Size())
	memStatsEnd := memanalyzer.MemSize(tf)

	fmt.Println("\n\nMemory report after:")
	fmt.Println(memanalyzer.MemoryReport(tf))

	fmt.Println(memStatsEnd, memStatsStart)

	require.Less(t, float64(memStatsEnd), 1.1*float64(memStatsStart), "the objects in the heap should not grow by more than 10%")
}
