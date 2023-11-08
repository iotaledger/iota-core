package spenddagv1

import (
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"

	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/acceptance"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/core/weight"
	iotago "github.com/iotaledger/iota.go/v4"
)

type SortedConflictSet = *SortedSpends[iotago.TransactionID, iotago.OutputID, vote.MockedRank]

var NewSortedConflictSet = NewSortedSpends[iotago.TransactionID, iotago.OutputID, vote.MockedRank]

func TestSortedConflict(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	conflict1 := NewTestSpend(transactionID("conflict1"), nil, nil, weight.New().AddCumulativeWeight(12), pendingTasks, thresholdProvider)
	conflict1.setAcceptanceState(acceptance.Rejected)
	conflict2 := NewTestSpend(transactionID("conflict2"), nil, nil, weight.New().AddCumulativeWeight(10), pendingTasks, thresholdProvider)
	conflict3 := NewTestSpend(transactionID("conflict3"), nil, nil, weight.New().AddCumulativeWeight(1), pendingTasks, thresholdProvider)
	conflict3.setAcceptanceState(acceptance.Accepted)
	conflict4 := NewTestSpend(transactionID("conflict4"), nil, nil, weight.New().AddCumulativeWeight(11), pendingTasks, thresholdProvider)
	conflict4.setAcceptanceState(acceptance.Rejected)
	conflict5 := NewTestSpend(transactionID("conflict5"), nil, nil, weight.New().AddCumulativeWeight(11), pendingTasks, thresholdProvider)
	conflict6 := NewTestSpend(transactionID("conflict6"), nil, nil, weight.New().AddCumulativeWeight(2), pendingTasks, thresholdProvider)
	conflict6.setAcceptanceState(acceptance.Accepted)

	sortedConflicts := NewSortedConflictSet(conflict1, pendingTasks)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict1:0")

	sortedConflicts.Add(conflict2)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict2:0", "conflict1:0")

	sortedConflicts.Add(conflict3)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict3:0", "conflict2:0", "conflict1:0")

	sortedConflicts.Add(conflict4)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict3:0", "conflict2:0", "conflict1:0", "conflict4:0")

	sortedConflicts.Add(conflict5)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict3:0", "conflict5:0", "conflict2:0", "conflict1:0", "conflict4:0")

	sortedConflicts.Add(conflict6)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict6:0", "conflict3:0", "conflict5:0", "conflict2:0", "conflict1:0", "conflict4:0")

	conflict2.Weight.AddCumulativeWeight(3)
	require.Equal(t, int64(13), conflict2.Weight.Value().CumulativeWeight())
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict6:0", "conflict3:0", "conflict2:0", "conflict5:0", "conflict1:0", "conflict4:0")

	conflict2.Weight.RemoveCumulativeWeight(3)
	require.Equal(t, int64(10), conflict2.Weight.Value().CumulativeWeight())
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict6:0", "conflict3:0", "conflict5:0", "conflict2:0", "conflict1:0", "conflict4:0")

	conflict5.Weight.SetAcceptanceState(acceptance.Accepted)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict5:0", "conflict6:0", "conflict3:0", "conflict2:0", "conflict1:0", "conflict4:0")
}

func TestSortedDecreaseHeaviest(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	conflict1 := NewTestSpend(transactionID("conflict1"), nil, nil, weight.New().AddCumulativeWeight(1), pendingTasks, thresholdProvider)
	conflict1.setAcceptanceState(acceptance.Accepted)
	conflict2 := NewTestSpend(transactionID("conflict2"), nil, nil, weight.New().AddCumulativeWeight(2), pendingTasks, thresholdProvider)

	sortedConflicts := NewSortedConflictSet(conflict1, pendingTasks)

	sortedConflicts.Add(conflict1)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict1:0")

	sortedConflicts.Add(conflict2)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict1:0", "conflict2:0")

	conflict1.Weight.SetAcceptanceState(acceptance.Pending)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict2:0", "conflict1:0")
}

func TestSortedConflictParallel(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	const conflictCount = 1000
	const updateCount = 100000

	conflicts := make(map[string]TestSpend)
	parallelConflicts := make(map[string]TestSpend)
	for i := 0; i < conflictCount; i++ {
		alias := "conflict" + strconv.Itoa(i)

		conflicts[alias] = NewTestSpend(transactionID(alias), nil, nil, weight.New(), pendingTasks, thresholdProvider)
		parallelConflicts[alias] = NewTestSpend(transactionID(alias), nil, nil, weight.New(), pendingTasks, thresholdProvider)
	}

	sortedConflicts := NewSortedConflictSet(conflicts["conflict0"], pendingTasks)
	sortedParallelConflicts := NewSortedConflictSet(parallelConflicts["conflict0"], pendingTasks)
	sortedParallelConflicts1 := NewSortedConflictSet(parallelConflicts["conflict0"], pendingTasks)

	for i := 0; i < conflictCount; i++ {
		alias := "conflict" + strconv.Itoa(i)

		sortedConflicts.Add(conflicts[alias])
		sortedParallelConflicts.Add(parallelConflicts[alias])
		sortedParallelConflicts1.Add(parallelConflicts[alias])
	}

	originalSortingBefore := sortedConflicts.String()
	parallelSortingBefore := sortedParallelConflicts.String()
	require.Equal(t, originalSortingBefore, parallelSortingBefore)

	permutations := make([]func(conflict TestSpend), 0)
	for i := 0; i < updateCount; i++ {
		permutations = append(permutations, generateRandomWeightPermutation())
	}

	var wg sync.WaitGroup
	for i, permutation := range permutations {
		targetAlias := "conflict" + strconv.Itoa(i%conflictCount)

		permutation(conflicts[targetAlias])

		wg.Add(1)
		go func(permutation func(conflict TestSpend)) {
			permutation(parallelConflicts[targetAlias])

			wg.Done()
		}(permutation)
	}

	pendingTasks.WaitIsZero()
	wg.Wait()
	pendingTasks.WaitIsZero()

	originalSortingAfter := sortedConflicts.String()
	parallelSortingAfter := sortedParallelConflicts.String()
	require.Equal(t, originalSortingAfter, parallelSortingAfter)
	require.NotEqualf(t, originalSortingBefore, originalSortingAfter, "original sorting should have changed")

	pendingTasks.WaitIsZero()

	parallelSortingAfter = sortedParallelConflicts1.String()
	require.Equal(t, originalSortingAfter, parallelSortingAfter)
	require.NotEqualf(t, originalSortingBefore, originalSortingAfter, "original sorting should have changed")
}

func generateRandomWeightPermutation() func(conflict TestSpend) {
	switch rand.Intn(2) {
	case 0:
		return generateRandomCumulativeWeightPermutation(int64(rand.Intn(100)))
	default:
		// return generateRandomConfirmationStatePermutation()
		return func(conflict TestSpend) {
		}
	}
}

func generateRandomCumulativeWeightPermutation(delta int64) func(conflict TestSpend) {
	updateType := rand.Intn(100)

	return func(conflict TestSpend) {
		if updateType%2 == 0 {
			conflict.Weight.AddCumulativeWeight(delta)
		} else {
			conflict.Weight.RemoveCumulativeWeight(delta)
		}

		conflict.Weight.AddCumulativeWeight(delta)
	}
}

func assertSortedConflictsOrder(t *testing.T, sortedConflicts SortedConflictSet, aliases ...string) {
	require.NoError(t, sortedConflicts.ForEach(func(c TestSpend) error {
		currentAlias := aliases[0]
		aliases = aliases[1:]

		require.Equal(t, fmt.Sprintf("TransactionID(%s)", currentAlias), c.ID.String())

		return nil
	}, true))

	require.Empty(t, aliases)
}

func id(alias string) iotago.OutputID {
	bytes := blake2b.Sum256([]byte(alias))
	txIdentifier := iotago.TransactionIDRepresentingData(TestTransactionCreationSlot, bytes[:])
	spendID := iotago.OutputIDFromTransactionIDAndIndex(txIdentifier, 0)
	txIdentifier.RegisterAlias(alias)

	return spendID
}
