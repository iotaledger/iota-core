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

type SortedSpendSet = *SortedSpends[iotago.TransactionID, iotago.OutputID, vote.MockedRank]

var NewSortedSpendSet = NewSortedSpends[iotago.TransactionID, iotago.OutputID, vote.MockedRank]

func TestSortedSpend(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	spend1 := NewTestSpend(transactionID("spend1"), nil, nil, weight.New().AddCumulativeWeight(12), pendingTasks, thresholdProvider)
	spend1.setAcceptanceState(acceptance.Rejected)
	spend2 := NewTestSpend(transactionID("spend2"), nil, nil, weight.New().AddCumulativeWeight(10), pendingTasks, thresholdProvider)
	spend3 := NewTestSpend(transactionID("spend3"), nil, nil, weight.New().AddCumulativeWeight(1), pendingTasks, thresholdProvider)
	spend3.setAcceptanceState(acceptance.Accepted)
	spend4 := NewTestSpend(transactionID("spend4"), nil, nil, weight.New().AddCumulativeWeight(11), pendingTasks, thresholdProvider)
	spend4.setAcceptanceState(acceptance.Rejected)
	spend5 := NewTestSpend(transactionID("spend5"), nil, nil, weight.New().AddCumulativeWeight(11), pendingTasks, thresholdProvider)
	spend6 := NewTestSpend(transactionID("spend6"), nil, nil, weight.New().AddCumulativeWeight(2), pendingTasks, thresholdProvider)
	spend6.setAcceptanceState(acceptance.Accepted)

	sortedSpends := NewSortedSpendSet(spend1, pendingTasks)
	pendingTasks.WaitIsZero()
	assertSortedSpendsOrder(t, sortedSpends, "spend1:0")

	sortedSpends.Add(spend2)
	pendingTasks.WaitIsZero()
	assertSortedSpendsOrder(t, sortedSpends, "spend2:0", "spend1:0")

	sortedSpends.Add(spend3)
	pendingTasks.WaitIsZero()
	assertSortedSpendsOrder(t, sortedSpends, "spend3:0", "spend2:0", "spend1:0")

	sortedSpends.Add(spend4)
	pendingTasks.WaitIsZero()
	assertSortedSpendsOrder(t, sortedSpends, "spend3:0", "spend2:0", "spend1:0", "spend4:0")

	sortedSpends.Add(spend5)
	pendingTasks.WaitIsZero()
	assertSortedSpendsOrder(t, sortedSpends, "spend3:0", "spend5:0", "spend2:0", "spend1:0", "spend4:0")

	sortedSpends.Add(spend6)
	pendingTasks.WaitIsZero()
	assertSortedSpendsOrder(t, sortedSpends, "spend6:0", "spend3:0", "spend5:0", "spend2:0", "spend1:0", "spend4:0")

	spend2.Weight.AddCumulativeWeight(3)
	require.Equal(t, int64(13), spend2.Weight.Value().CumulativeWeight())
	pendingTasks.WaitIsZero()
	assertSortedSpendsOrder(t, sortedSpends, "spend6:0", "spend3:0", "spend2:0", "spend5:0", "spend1:0", "spend4:0")

	spend2.Weight.RemoveCumulativeWeight(3)
	require.Equal(t, int64(10), spend2.Weight.Value().CumulativeWeight())
	pendingTasks.WaitIsZero()
	assertSortedSpendsOrder(t, sortedSpends, "spend6:0", "spend3:0", "spend5:0", "spend2:0", "spend1:0", "spend4:0")

	spend5.Weight.SetAcceptanceState(acceptance.Accepted)
	pendingTasks.WaitIsZero()
	assertSortedSpendsOrder(t, sortedSpends, "spend5:0", "spend6:0", "spend3:0", "spend2:0", "spend1:0", "spend4:0")
}

func TestSortedDecreaseHeaviest(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	spend1 := NewTestSpend(transactionID("spend1"), nil, nil, weight.New().AddCumulativeWeight(1), pendingTasks, thresholdProvider)
	spend1.setAcceptanceState(acceptance.Accepted)
	spend2 := NewTestSpend(transactionID("spend2"), nil, nil, weight.New().AddCumulativeWeight(2), pendingTasks, thresholdProvider)

	sortedSpends := NewSortedSpendSet(spend1, pendingTasks)

	sortedSpends.Add(spend1)
	pendingTasks.WaitIsZero()
	assertSortedSpendsOrder(t, sortedSpends, "spend1:0")

	sortedSpends.Add(spend2)
	pendingTasks.WaitIsZero()
	assertSortedSpendsOrder(t, sortedSpends, "spend1:0", "spend2:0")

	spend1.Weight.SetAcceptanceState(acceptance.Pending)
	pendingTasks.WaitIsZero()
	assertSortedSpendsOrder(t, sortedSpends, "spend2:0", "spend1:0")
}

func TestSortedSpendParallel(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	const spendCount = 1000
	const updateCount = 100000

	spends := make(map[string]TestSpend)
	parallelSpends := make(map[string]TestSpend)
	for i := 0; i < spendCount; i++ {
		alias := "spend" + strconv.Itoa(i)

		spends[alias] = NewTestSpend(transactionID(alias), nil, nil, weight.New(), pendingTasks, thresholdProvider)
		parallelSpends[alias] = NewTestSpend(transactionID(alias), nil, nil, weight.New(), pendingTasks, thresholdProvider)
	}

	sortedSpends := NewSortedSpendSet(spends["spend0"], pendingTasks)
	sortedParallelSpends := NewSortedSpendSet(parallelSpends["spend0"], pendingTasks)
	sortedParallelSpends1 := NewSortedSpendSet(parallelSpends["spend0"], pendingTasks)

	for i := 0; i < spendCount; i++ {
		alias := "spend" + strconv.Itoa(i)

		sortedSpends.Add(spends[alias])
		sortedParallelSpends.Add(parallelSpends[alias])
		sortedParallelSpends1.Add(parallelSpends[alias])
	}

	originalSortingBefore := sortedSpends.String()
	parallelSortingBefore := sortedParallelSpends.String()
	require.Equal(t, originalSortingBefore, parallelSortingBefore)

	permutations := make([]func(spend TestSpend), 0)
	for i := 0; i < updateCount; i++ {
		permutations = append(permutations, generateRandomWeightPermutation())
	}

	var wg sync.WaitGroup
	for i, permutation := range permutations {
		targetAlias := "spend" + strconv.Itoa(i%spendCount)

		permutation(spends[targetAlias])

		wg.Add(1)
		go func(permutation func(spend TestSpend)) {
			permutation(parallelSpends[targetAlias])

			wg.Done()
		}(permutation)
	}

	pendingTasks.WaitIsZero()
	wg.Wait()
	pendingTasks.WaitIsZero()

	originalSortingAfter := sortedSpends.String()
	parallelSortingAfter := sortedParallelSpends.String()
	require.Equal(t, originalSortingAfter, parallelSortingAfter)
	require.NotEqualf(t, originalSortingBefore, originalSortingAfter, "original sorting should have changed")

	pendingTasks.WaitIsZero()

	parallelSortingAfter = sortedParallelSpends1.String()
	require.Equal(t, originalSortingAfter, parallelSortingAfter)
	require.NotEqualf(t, originalSortingBefore, originalSortingAfter, "original sorting should have changed")
}

func generateRandomWeightPermutation() func(spend TestSpend) {
	switch rand.Intn(2) {
	case 0:
		return generateRandomCumulativeWeightPermutation(int64(rand.Intn(100)))
	default:
		// return generateRandomConfirmationStatePermutation()
		return func(spend TestSpend) {
		}
	}
}

func generateRandomCumulativeWeightPermutation(delta int64) func(spend TestSpend) {
	updateType := rand.Intn(100)

	return func(spend TestSpend) {
		if updateType%2 == 0 {
			spend.Weight.AddCumulativeWeight(delta)
		} else {
			spend.Weight.RemoveCumulativeWeight(delta)
		}

		spend.Weight.AddCumulativeWeight(delta)
	}
}

func assertSortedSpendsOrder(t *testing.T, sortedSpends SortedSpendSet, aliases ...string) {
	require.NoError(t, sortedSpends.ForEach(func(c TestSpend) error {
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
