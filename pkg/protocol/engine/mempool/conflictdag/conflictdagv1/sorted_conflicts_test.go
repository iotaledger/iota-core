package conflictdagv1

import (
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

type SortedConflictSet = *SortedConflicts[iotago.TransactionID, iotago.OutputID, vote.MockedRank]

var NewSortedConflictSet = NewSortedConflicts[iotago.TransactionID, iotago.OutputID, vote.MockedRank]

func TestSortedConflict(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	conflict1 := NewTestConflict(transactionID("conflict1"), nil, nil, weight.New().AddCumulativeWeight(12), pendingTasks, thresholdProvider)
	conflict1.setAcceptanceState(acceptance.Rejected)
	conflict2 := NewTestConflict(transactionID("conflict2"), nil, nil, weight.New().AddCumulativeWeight(10), pendingTasks, thresholdProvider)
	conflict3 := NewTestConflict(transactionID("conflict3"), nil, nil, weight.New().AddCumulativeWeight(1), pendingTasks, thresholdProvider)
	conflict3.setAcceptanceState(acceptance.Accepted)
	conflict4 := NewTestConflict(transactionID("conflict4"), nil, nil, weight.New().AddCumulativeWeight(11), pendingTasks, thresholdProvider)
	conflict4.setAcceptanceState(acceptance.Rejected)
	conflict5 := NewTestConflict(transactionID("conflict5"), nil, nil, weight.New().AddCumulativeWeight(11), pendingTasks, thresholdProvider)
	conflict6 := NewTestConflict(transactionID("conflict6"), nil, nil, weight.New().AddCumulativeWeight(2), pendingTasks, thresholdProvider)
	conflict6.setAcceptanceState(acceptance.Accepted)

	sortedConflicts := NewSortedConflictSet(conflict1, pendingTasks)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict1")

	sortedConflicts.Add(conflict2)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict2", "conflict1")

	sortedConflicts.Add(conflict3)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict3", "conflict2", "conflict1")

	sortedConflicts.Add(conflict4)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict3", "conflict2", "conflict1", "conflict4")

	sortedConflicts.Add(conflict5)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict3", "conflict5", "conflict2", "conflict1", "conflict4")

	sortedConflicts.Add(conflict6)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict6", "conflict3", "conflict5", "conflict2", "conflict1", "conflict4")

	conflict2.Weight.AddCumulativeWeight(3)
	require.Equal(t, int64(13), conflict2.Weight.Value().CumulativeWeight())
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict6", "conflict3", "conflict2", "conflict5", "conflict1", "conflict4")

	conflict2.Weight.RemoveCumulativeWeight(3)
	require.Equal(t, int64(10), conflict2.Weight.Value().CumulativeWeight())
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict6", "conflict3", "conflict5", "conflict2", "conflict1", "conflict4")

	conflict5.Weight.SetAcceptanceState(acceptance.Accepted)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict5", "conflict6", "conflict3", "conflict2", "conflict1", "conflict4")
}

func TestSortedDecreaseHeaviest(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	conflict1 := NewTestConflict(transactionID("conflict1"), nil, nil, weight.New().AddCumulativeWeight(1), pendingTasks, thresholdProvider)
	conflict1.setAcceptanceState(acceptance.Accepted)
	conflict2 := NewTestConflict(transactionID("conflict2"), nil, nil, weight.New().AddCumulativeWeight(2), pendingTasks, thresholdProvider)

	sortedConflicts := NewSortedConflictSet(conflict1, pendingTasks)

	sortedConflicts.Add(conflict1)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict1")

	sortedConflicts.Add(conflict2)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict1", "conflict2")

	conflict1.Weight.SetAcceptanceState(acceptance.Pending)
	pendingTasks.WaitIsZero()
	assertSortedConflictsOrder(t, sortedConflicts, "conflict2", "conflict1")
}

func TestSortedConflictParallel(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	const conflictCount = 1000
	const updateCount = 100000

	conflicts := make(map[string]TestConflict)
	parallelConflicts := make(map[string]TestConflict)
	for i := 0; i < conflictCount; i++ {
		alias := "conflict" + strconv.Itoa(i)

		conflicts[alias] = NewTestConflict(transactionID(alias), nil, nil, weight.New(), pendingTasks, thresholdProvider)
		parallelConflicts[alias] = NewTestConflict(transactionID(alias), nil, nil, weight.New(), pendingTasks, thresholdProvider)
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

	permutations := make([]func(conflict TestConflict), 0)
	for i := 0; i < updateCount; i++ {
		permutations = append(permutations, generateRandomWeightPermutation())
	}

	var wg sync.WaitGroup
	for i, permutation := range permutations {
		targetAlias := "conflict" + strconv.Itoa(i%conflictCount)

		permutation(conflicts[targetAlias])

		wg.Add(1)
		go func(permutation func(conflict TestConflict)) {
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

func generateRandomWeightPermutation() func(conflict TestConflict) {
	switch rand.Intn(2) {
	case 0:
		return generateRandomCumulativeWeightPermutation(int64(rand.Intn(100)))
	default:
		// return generateRandomConfirmationStatePermutation()
		return func(conflict TestConflict) {
		}
	}
}

func generateRandomCumulativeWeightPermutation(delta int64) func(conflict TestConflict) {
	updateType := rand.Intn(100)

	return func(conflict TestConflict) {
		if updateType%2 == 0 {
			conflict.Weight.AddCumulativeWeight(delta)
		} else {
			conflict.Weight.RemoveCumulativeWeight(delta)
		}

		conflict.Weight.AddCumulativeWeight(delta)
	}
}

func assertSortedConflictsOrder(t *testing.T, sortedConflicts SortedConflictSet, aliases ...string) {
	require.NoError(t, sortedConflicts.ForEach(func(c TestConflict) error {
		currentAlias := aliases[0]
		aliases = aliases[1:]

		require.Equal(t, currentAlias, c.ID.String())

		return nil
	}, true))

	require.Empty(t, aliases)
}

func id(alias string) iotago.OutputID {
	bytes := blake2b.Sum256([]byte(alias))
	txIdentifier := iotago.TransactionIDFromData(TestTransactionCreationSlot, bytes[:])
	conflictID := iotago.OutputIDFromTransactionIDAndIndex(txIdentifier, 0)
	txIdentifier.RegisterAlias(alias)

	return conflictID
}
