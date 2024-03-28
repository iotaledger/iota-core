package spenddagv1

import (
	"math/rand"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/acceptance"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/core/weight"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TestSpender = *Spender[iotago.TransactionID, iotago.OutputID, vote.MockedRank]

//var NewTestSpend = NewSpender[iotago.TransactionID, iotago.OutputID, vote.MockedRank]

func NewTestSpender(id iotago.TransactionID, parentSpends ds.Set[*Spender[iotago.TransactionID, iotago.OutputID, vote.MockedRank]], SpendSets ds.Set[*SpendSet[iotago.TransactionID, iotago.OutputID, vote.MockedRank]], initialWeight *weight.Weight, pendingTasksCounter *syncutils.Counter, acceptanceThresholdProvider func() int64) *Spender[iotago.TransactionID, iotago.OutputID, vote.MockedRank] {
	spender := NewSpender[iotago.TransactionID, iotago.OutputID, vote.MockedRank](id, initialWeight, pendingTasksCounter, acceptanceThresholdProvider)
	_, err := spender.JoinSpendSets(SpendSets)
	if err != nil {
		// TODO: change this
		panic(err)
	}
	spender.UpdateParents(parentSpends, ds.NewSet[*Spender[iotago.TransactionID, iotago.OutputID, vote.MockedRank]]())

	return spender
}

func TestSpend_SetRejected(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	Spend1 := NewTestSpender(transactionID("Spend1"), nil, nil, weight.New(), pendingTasks, thresholdProvider)
	Spend2 := NewTestSpender(transactionID("Spend2"), ds.NewSet(Spend1), nil, weight.New(), pendingTasks, thresholdProvider)
	Spend3 := NewTestSpender(transactionID("Spend3"), ds.NewSet(Spend2), nil, weight.New(), pendingTasks, thresholdProvider)

	Spend1.setAcceptanceState(acceptance.Rejected)
	require.True(t, Spend1.IsRejected())
	require.True(t, Spend2.IsRejected())
	require.True(t, Spend3.IsRejected())

	Spend4 := NewTestSpender(transactionID("Spend4"), ds.NewSet(Spend1), nil, weight.New(), pendingTasks, thresholdProvider)
	require.True(t, Spend4.IsRejected())
}

func TestSpend_UpdateParents(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	Spend1 := NewTestSpender(transactionID("Spend1"), nil, nil, weight.New(), pendingTasks, thresholdProvider)
	Spend2 := NewTestSpender(transactionID("Spend2"), nil, nil, weight.New(), pendingTasks, thresholdProvider)
	Spend3 := NewTestSpender(transactionID("Spend3"), ds.NewSet(Spend1, Spend2), nil, weight.New(), pendingTasks, thresholdProvider)

	require.True(t, Spend3.Parents.Has(Spend1))
	require.True(t, Spend3.Parents.Has(Spend2))
}

func TestSpend_SetAccepted(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	{
		SpendSet1 := NewTestSpendSet(id("SpendSet1"))
		SpendSet2 := NewTestSpendSet(id("SpendSet2"))

		Spend1 := NewTestSpender(transactionID("Spend1"), nil, ds.NewSet(SpendSet1), weight.New(), pendingTasks, thresholdProvider)
		Spend2 := NewTestSpender(transactionID("Spend2"), nil, ds.NewSet(SpendSet1, SpendSet2), weight.New(), pendingTasks, thresholdProvider)
		Spend3 := NewTestSpender(transactionID("Spend3"), nil, ds.NewSet(SpendSet2), weight.New(), pendingTasks, thresholdProvider)

		require.Equal(t, acceptance.Pending, Spend1.setAcceptanceState(acceptance.Accepted))
		require.True(t, Spend1.IsAccepted())
		require.True(t, Spend2.IsRejected())
		require.True(t, Spend3.IsPending())

		// set acceptance twice to make sure that  the event is not triggered twice
		// TODO: attach to the event and make sure that it's not triggered
		require.Equal(t, acceptance.Accepted, Spend1.setAcceptanceState(acceptance.Accepted))
		require.True(t, Spend1.IsAccepted())
		require.True(t, Spend2.IsRejected())
		require.True(t, Spend3.IsPending())
	}

	{
		SpendSet1 := NewTestSpendSet(id("SpendSet1"))
		SpendSet2 := NewTestSpendSet(id("SpendSet2"))

		Spend1 := NewTestSpender(transactionID("Spend1"), nil, ds.NewSet(SpendSet1), weight.New(), pendingTasks, thresholdProvider)
		Spend2 := NewTestSpender(transactionID("Spend2"), nil, ds.NewSet(SpendSet1, SpendSet2), weight.New(), pendingTasks, thresholdProvider)
		Spend3 := NewTestSpender(transactionID("Spend3"), nil, ds.NewSet(SpendSet2), weight.New(), pendingTasks, thresholdProvider)

		Spend2.setAcceptanceState(acceptance.Accepted)
		require.True(t, Spend1.IsRejected())
		require.True(t, Spend2.IsAccepted())
		require.True(t, Spend3.IsRejected())
	}
}

func TestSpend_SpendSets(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	red := NewTestSpendSet(id("red"))
	blue := NewTestSpendSet(id("blue"))
	green := NewTestSpendSet(id("green"))
	yellow := NewTestSpendSet(id("yellow"))

	SpendA := NewTestSpender(transactionID("A"), nil, ds.NewSet(red), weight.New().AddCumulativeWeight(7), pendingTasks, thresholdProvider)
	SpendB := NewTestSpender(transactionID("B"), nil, ds.NewSet(red, blue), weight.New().AddCumulativeWeight(3), pendingTasks, thresholdProvider)
	SpendC := NewTestSpender(transactionID("C"), nil, ds.NewSet(blue, green), weight.New().AddCumulativeWeight(5), pendingTasks, thresholdProvider)
	SpendD := NewTestSpender(transactionID("D"), nil, ds.NewSet(green, yellow), weight.New().AddCumulativeWeight(7), pendingTasks, thresholdProvider)
	SpendE := NewTestSpender(transactionID("E"), nil, ds.NewSet(yellow), weight.New().AddCumulativeWeight(9), pendingTasks, thresholdProvider)

	preferredInsteadMap := map[TestSpender]TestSpender{
		SpendA: SpendA,
		SpendB: SpendA,
		SpendC: SpendC,
		SpendD: SpendE,
		SpendE: SpendE,
	}

	pendingTasks.WaitIsZero()
	assertPreferredInstead(t, preferredInsteadMap)

	SpendD.Weight.SetCumulativeWeight(10)
	pendingTasks.WaitIsZero()

	assertPreferredInstead(t, lo.MergeMaps(preferredInsteadMap, map[TestSpender]TestSpender{
		SpendC: SpendD,
		SpendD: SpendD,
		SpendE: SpendD,
	}))

	SpendD.Weight.SetCumulativeWeight(0)
	pendingTasks.WaitIsZero()

	assertPreferredInstead(t, lo.MergeMaps(preferredInsteadMap, map[TestSpender]TestSpender{
		SpendC: SpendC,
		SpendD: SpendE,
		SpendE: SpendE,
	}))

	SpendC.Weight.SetCumulativeWeight(8)
	pendingTasks.WaitIsZero()

	assertPreferredInstead(t, lo.MergeMaps(preferredInsteadMap, map[TestSpender]TestSpender{
		SpendB: SpendC,
	}))

	SpendC.Weight.SetCumulativeWeight(8)
	pendingTasks.WaitIsZero()

	assertPreferredInstead(t, lo.MergeMaps(preferredInsteadMap, map[TestSpender]TestSpender{
		SpendB: SpendC,
	}))

	SpendD.Weight.SetCumulativeWeight(3)
	pendingTasks.WaitIsZero()

	assertPreferredInstead(t, preferredInsteadMap)

	SpendE.Weight.SetCumulativeWeight(1)
	pendingTasks.WaitIsZero()

	assertPreferredInstead(t, lo.MergeMaps(preferredInsteadMap, map[TestSpender]TestSpender{
		SpendD: SpendC,
	}))

	SpendE.Weight.SetCumulativeWeight(9)
	pendingTasks.WaitIsZero()

	assertPreferredInstead(t, lo.MergeMaps(preferredInsteadMap, map[TestSpender]TestSpender{
		SpendD: SpendE,
	}))

	SpendF := NewTestSpender(transactionID("F"), nil, ds.NewSet(yellow), weight.New().AddCumulativeWeight(19), pendingTasks, thresholdProvider)

	pendingTasks.WaitIsZero()

	assertPreferredInstead(t, lo.MergeMaps(preferredInsteadMap, map[TestSpender]TestSpender{
		SpendD: SpendF,
		SpendE: SpendF,
		SpendF: SpendF,
	}))

	assertCorrectOrder(t, SpendA, SpendB, SpendC, SpendD, SpendE, SpendF)
}

func TestSpendParallel(t *testing.T) {
	sequentialPendingTasks := syncutils.NewCounter()
	parallelPendingTasks := syncutils.NewCounter()

	sequentialSpenders := createSpenders(sequentialPendingTasks)
	sequentialPendingTasks.WaitIsZero()

	parallelSpenders := createSpenders(parallelPendingTasks)
	parallelPendingTasks.WaitIsZero()

	const updateCount = 100000

	permutations := make([]func(Spend TestSpender), 0)
	for i := 0; i < updateCount; i++ {
		permutations = append(permutations, generateRandomSpendPermutation())
	}

	var wg sync.WaitGroup
	for _, permutation := range permutations {
		targetAlias := lo.Keys(parallelSpenders)[rand.Intn(len(parallelSpenders))]

		permutation(sequentialSpenders[targetAlias])

		wg.Add(1)
		go func(permutation func(Spend TestSpender)) {
			permutation(parallelSpenders[targetAlias])

			wg.Done()
		}(permutation)
	}

	sequentialPendingTasks.WaitIsZero()

	wg.Wait()

	parallelPendingTasks.WaitIsZero()

	lo.ForEach(lo.Keys(parallelSpenders), func(SpendAlias string) {
		require.EqualValuesf(t, sequentialSpenders[SpendAlias].PreferredInstead().ID, parallelSpenders[SpendAlias].PreferredInstead().ID, "parallel Spend %s prefers %s, but sequential Spend prefers %s", SpendAlias, parallelSpenders[SpendAlias].PreferredInstead().ID, sequentialSpenders[SpendAlias].PreferredInstead().ID)
	})

	assertCorrectOrder(t, lo.Values(sequentialSpenders)...)
	assertCorrectOrder(t, lo.Values(parallelSpenders)...)
}

func TestLikedInstead1(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	masterBranch := NewTestSpender(transactionID("M"), nil, nil, weight.New(), pendingTasks, thresholdProvider)
	require.True(t, masterBranch.IsLiked())
	require.True(t, masterBranch.LikedInstead().IsEmpty())

	SpendSet1 := NewTestSpendSet(id("O1"))

	Spend1 := NewTestSpender(transactionID("TxA"), ds.NewSet(masterBranch), ds.NewSet(SpendSet1), weight.New().SetCumulativeWeight(6), pendingTasks, thresholdProvider)
	Spend2 := NewTestSpender(transactionID("TxB"), ds.NewSet(masterBranch), ds.NewSet(SpendSet1), weight.New().SetCumulativeWeight(3), pendingTasks, thresholdProvider)

	require.True(t, Spend1.IsPreferred())
	require.True(t, Spend1.IsLiked())
	require.Equal(t, 0, Spend1.LikedInstead().Size())

	require.False(t, Spend2.IsPreferred())
	require.False(t, Spend2.IsLiked())
	require.Equal(t, 1, Spend2.LikedInstead().Size())
	require.True(t, Spend2.LikedInstead().Has(Spend1))
}

func TestLikedInsteadFromPreferredInstead(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	masterBranch := NewTestSpender(transactionID("M"), nil, nil, weight.New(), pendingTasks, thresholdProvider)
	require.True(t, masterBranch.IsLiked())
	require.True(t, masterBranch.LikedInstead().IsEmpty())

	SpendSet1 := NewTestSpendSet(id("O1"))
	SpendA := NewTestSpender(transactionID("TxA"), ds.NewSet(masterBranch), ds.NewSet(SpendSet1), weight.New().SetCumulativeWeight(200), pendingTasks, thresholdProvider)
	SpendB := NewTestSpender(transactionID("TxB"), ds.NewSet(masterBranch), ds.NewSet(SpendSet1), weight.New().SetCumulativeWeight(100), pendingTasks, thresholdProvider)

	require.True(t, SpendA.IsPreferred())
	require.True(t, SpendA.IsLiked())
	require.Equal(t, 0, SpendA.LikedInstead().Size())

	require.False(t, SpendB.IsPreferred())
	require.False(t, SpendB.IsLiked())
	require.Equal(t, 1, SpendB.LikedInstead().Size())
	require.True(t, SpendB.LikedInstead().Has(SpendA))

	SpendSet2 := NewTestSpendSet(id("O2"))
	SpendC := NewTestSpender(transactionID("TxC"), ds.NewSet(SpendA), ds.NewSet(SpendSet2), weight.New().SetCumulativeWeight(200), pendingTasks, thresholdProvider)
	SpendD := NewTestSpender(transactionID("TxD"), ds.NewSet(SpendA), ds.NewSet(SpendSet2), weight.New().SetCumulativeWeight(100), pendingTasks, thresholdProvider)

	require.True(t, SpendC.IsPreferred())
	require.True(t, SpendC.IsLiked())
	require.Equal(t, 0, SpendC.LikedInstead().Size())

	require.False(t, SpendD.IsPreferred())
	require.False(t, SpendD.IsLiked())
	require.Equal(t, 1, SpendD.LikedInstead().Size())
	require.True(t, SpendD.LikedInstead().Has(SpendC))

	SpendB.Weight.SetCumulativeWeight(300)
	pendingTasks.WaitIsZero()

	require.True(t, SpendB.IsPreferred())
	require.True(t, SpendB.IsLiked())
	require.Equal(t, 0, SpendB.LikedInstead().Size())

	require.False(t, SpendA.IsPreferred())
	require.False(t, SpendA.IsLiked())
	require.Equal(t, 1, SpendA.LikedInstead().Size())
	require.True(t, SpendA.LikedInstead().Has(SpendB))

	require.False(t, SpendD.IsPreferred())
	require.False(t, SpendD.IsLiked())
	require.Equal(t, 1, SpendD.LikedInstead().Size())
	require.True(t, SpendD.LikedInstead().Has(SpendB))

	SpendB.Weight.SetCumulativeWeight(100)
	pendingTasks.WaitIsZero()

	require.True(t, SpendA.IsPreferred())
	require.True(t, SpendA.IsLiked())
	require.Equal(t, 0, SpendA.LikedInstead().Size())

	require.False(t, SpendB.IsPreferred())
	require.False(t, SpendB.IsLiked())
	require.Equal(t, 1, SpendB.LikedInstead().Size())
	require.True(t, SpendB.LikedInstead().Has(SpendA))

	require.True(t, SpendC.IsPreferred())
	require.True(t, SpendC.IsLiked())
	require.Equal(t, 0, SpendC.LikedInstead().Size())

	require.False(t, SpendD.IsPreferred())
	require.False(t, SpendD.IsLiked())
	require.Equal(t, 1, SpendD.LikedInstead().Size())
	require.True(t, SpendD.LikedInstead().Has(SpendC))
}

func TestLikedInstead21(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	masterBranch := NewTestSpender(transactionID("M"), nil, nil, weight.New(), pendingTasks, thresholdProvider)
	require.True(t, masterBranch.IsLiked())
	require.True(t, masterBranch.LikedInstead().IsEmpty())

	SpendSet1 := NewTestSpendSet(id("O1"))
	SpendA := NewTestSpender(transactionID("TxA"), ds.NewSet(masterBranch), ds.NewSet(SpendSet1), weight.New().SetCumulativeWeight(200), pendingTasks, thresholdProvider)
	SpendB := NewTestSpender(transactionID("TxB"), ds.NewSet(masterBranch), ds.NewSet(SpendSet1), weight.New().SetCumulativeWeight(100), pendingTasks, thresholdProvider)

	require.True(t, SpendA.IsPreferred())
	require.True(t, SpendA.IsLiked())
	require.Equal(t, 0, SpendA.LikedInstead().Size())

	require.False(t, SpendB.IsPreferred())
	require.False(t, SpendB.IsLiked())
	require.Equal(t, 1, SpendB.LikedInstead().Size())
	require.True(t, SpendB.LikedInstead().Has(SpendA))

	SpendSet4 := NewTestSpendSet(id("O4"))
	SpendF := NewTestSpender(transactionID("TxF"), ds.NewSet(SpendA), ds.NewSet(SpendSet4), weight.New().SetCumulativeWeight(20), pendingTasks, thresholdProvider)
	SpendG := NewTestSpender(transactionID("TxG"), ds.NewSet(SpendA), ds.NewSet(SpendSet4), weight.New().SetCumulativeWeight(10), pendingTasks, thresholdProvider)

	require.True(t, SpendF.IsPreferred())
	require.True(t, SpendF.IsLiked())
	require.Equal(t, 0, SpendF.LikedInstead().Size())

	require.False(t, SpendG.IsPreferred())
	require.False(t, SpendG.IsLiked())
	require.Equal(t, 1, SpendG.LikedInstead().Size())
	require.True(t, SpendG.LikedInstead().Has(SpendF))

	SpendSet2 := NewTestSpendSet(id("O2"))
	SpendC := NewTestSpender(transactionID("TxC"), ds.NewSet(masterBranch), ds.NewSet(SpendSet2), weight.New().SetCumulativeWeight(200), pendingTasks, thresholdProvider)
	SpendH := NewTestSpender(transactionID("TxH"), ds.NewSet(masterBranch, SpendA), ds.NewSet(SpendSet2, SpendSet4), weight.New().SetCumulativeWeight(150), pendingTasks, thresholdProvider)

	require.True(t, SpendC.IsPreferred())
	require.True(t, SpendC.IsLiked())
	require.Equal(t, 0, SpendC.LikedInstead().Size())

	require.False(t, SpendH.IsPreferred())
	require.False(t, SpendH.IsLiked())
	require.Equal(t, 1, SpendH.LikedInstead().Size())
	require.True(t, SpendH.LikedInstead().Has(SpendC))

	SpendSet3 := NewTestSpendSet(id("O12"))
	SpendI := NewTestSpender(transactionID("TxI"), ds.NewSet(SpendF), ds.NewSet(SpendSet3), weight.New().SetCumulativeWeight(5), pendingTasks, thresholdProvider)
	SpendJ := NewTestSpender(transactionID("TxJ"), ds.NewSet(SpendF), ds.NewSet(SpendSet3), weight.New().SetCumulativeWeight(15), pendingTasks, thresholdProvider)

	require.True(t, SpendJ.IsPreferred())
	require.True(t, SpendJ.IsLiked())
	require.Equal(t, 0, SpendJ.LikedInstead().Size())

	require.False(t, SpendI.IsPreferred())
	require.False(t, SpendI.IsLiked())
	require.Equal(t, 1, SpendI.LikedInstead().Size())
	require.True(t, SpendI.LikedInstead().Has(SpendJ))

	SpendH.Weight.SetCumulativeWeight(250)

	pendingTasks.WaitIsZero()

	require.True(t, SpendH.IsPreferred())
	require.True(t, SpendH.IsLiked())
	require.Equal(t, 0, SpendH.LikedInstead().Size())

	require.False(t, SpendF.IsPreferred())
	require.False(t, SpendF.IsLiked())
	require.Equal(t, 1, SpendF.LikedInstead().Size())
	require.True(t, SpendF.LikedInstead().Has(SpendH))

	require.False(t, SpendG.IsPreferred())
	require.False(t, SpendG.IsLiked())
	require.Equal(t, 1, SpendG.LikedInstead().Size())
	require.True(t, SpendG.LikedInstead().Has(SpendH))

	require.True(t, SpendJ.IsPreferred())
	require.False(t, SpendJ.IsLiked())
	require.Equal(t, 1, SpendJ.LikedInstead().Size())
	require.True(t, SpendJ.LikedInstead().Has(SpendH))
}

func TestSpendSet_AllMembersEvicted(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())

	pendingTasks := syncutils.NewCounter()
	yellow := NewTestSpendSet(id("yellow"))
	green := NewTestSpendSet(id("green"))

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	Spend1 := NewTestSpender(transactionID("Spend1"), nil, ds.NewSet(yellow), weight.New(), pendingTasks, thresholdProvider)
	evictedSpends := Spend1.Evict()
	require.Len(t, evictedSpends, 1)
	require.Contains(t, evictedSpends, Spend1.ID)

	// evict the Spend another time and make sure that none Spends were evicted
	evictedSpends = Spend1.Evict()
	require.Len(t, evictedSpends, 0)

	// Spend tries to join Spendset who's all members were evicted
	Spend2 := NewSpender[iotago.TransactionID, iotago.OutputID, vote.MockedRank](transactionID("Spend1"), weight.New(), pendingTasks, thresholdProvider)
	_, err := Spend2.JoinSpendSets(ds.NewSet(yellow))
	require.Error(t, err)

	// evicted Spend tries to join Spendset
	_, err = Spend1.JoinSpendSets(ds.NewSet(green))
	require.Error(t, err)
}

func TestSpend_Compare(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	var Spend1, Spend2 TestSpender

	Spend1 = NewTestSpender(transactionID("M"), nil, nil, weight.New(), pendingTasks, thresholdProvider)

	require.Equal(t, weight.Heavier, Spend1.Compare(nil))
	require.Equal(t, weight.Lighter, Spend2.Compare(Spend1))
	require.Equal(t, weight.Equal, Spend2.Compare(nil))
}

func TestSpend_Inheritance(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())

	pendingTasks := syncutils.NewCounter()
	yellow := NewTestSpendSet(id("yellow"))
	green := NewTestSpendSet(id("green"))

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	Spend1 := NewTestSpender(transactionID("Spend1"), nil, ds.NewSet(yellow), weight.New().SetCumulativeWeight(1), pendingTasks, thresholdProvider)
	Spend2 := NewTestSpender(transactionID("Spend2"), nil, ds.NewSet(green), weight.New().SetCumulativeWeight(1), pendingTasks, thresholdProvider)
	Spend3 := NewTestSpender(transactionID("Spend3"), ds.NewSet(Spend1, Spend2), nil, weight.New(), pendingTasks, thresholdProvider)
	Spend4 := NewTestSpender(transactionID("Spend4"), nil, ds.NewSet(yellow, green), weight.New(), pendingTasks, thresholdProvider)

	pendingTasks.WaitIsZero()
	require.True(t, Spend3.LikedInstead().IsEmpty())

	Spend4.Weight.SetCumulativeWeight(10)
	pendingTasks.WaitIsZero()
	require.True(t, Spend3.LikedInstead().Has(Spend4))

	// set it manually again, to make sure that it's idempotent
	Spend2.setPreferredInstead(Spend4)
	pendingTasks.WaitIsZero()
	require.True(t, Spend3.LikedInstead().Has(Spend4))

	// make sure that inheritance of LikedInstead works correctly for newly created Spends
	Spend5 := NewTestSpender(transactionID("Spend5"), ds.NewSet(Spend3), nil, weight.New(), pendingTasks, thresholdProvider)
	pendingTasks.WaitIsZero()
	require.True(t, Spend5.LikedInstead().Has(Spend4))

	Spend1.Weight.SetCumulativeWeight(15)
	pendingTasks.WaitIsZero()
	require.True(t, Spend3.LikedInstead().IsEmpty())
}

func assertCorrectOrder(t *testing.T, spenders ...TestSpender) {
	sort.Slice(spenders, func(i, j int) bool {
		return spenders[i].Compare(spenders[j]) == weight.Heavier
	})

	preferredSpenders := ds.NewSet[TestSpender]()
	unPreferredSpenders := ds.NewSet[TestSpender]()

	for _, spender := range spenders {
		if !unPreferredSpenders.Has(spender) {
			preferredSpenders.Add(spender)
			spender.ConflictingSpenders.Range(func(conflictingSpender *Spender[iotago.TransactionID, iotago.OutputID, vote.MockedRank]) {
				if spender != conflictingSpender {
					unPreferredSpenders.Add(conflictingSpender)
				}
			}, true)
		}
	}

	for _, spender := range spenders {
		if preferredSpenders.Has(spender) {
			require.True(t, spender.IsPreferred(), "spender %s should be preferred", spender.ID)
		}
		if unPreferredSpenders.Has(spender) {
			require.False(t, spender.IsPreferred(), "spender %s should be unPreferred", spender.ID)
		}
	}

	_ = unPreferredSpenders.ForEach(func(unPreferredSpender TestSpender) (err error) {
		// iterating in descending order, so the first preferred Spend
		return unPreferredSpender.ConflictingSpenders.ForEach(func(conflictingSpender TestSpender) error {
			if conflictingSpender != unPreferredSpender && conflictingSpender.IsPreferred() {
				require.Equal(t, conflictingSpender, unPreferredSpender.PreferredInstead())

				return ierrors.New("break the loop")
			}

			return nil
		}, true)
	})
}

func generateRandomSpendPermutation() func(spender TestSpender) {
	updateType := rand.Intn(100)
	delta := rand.Intn(100)

	return func(spender TestSpender) {
		if updateType%2 == 0 {
			spender.Weight.AddCumulativeWeight(int64(delta))
		} else {
			spender.Weight.RemoveCumulativeWeight(int64(delta))
		}
	}
}

func createSpenders(pendingTasks *syncutils.Counter) map[string]TestSpender {
	weights := account.NewSeatedAccounts(account.NewAccounts())

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	red := NewTestSpendSet(id("red"))
	blue := NewTestSpendSet(id("blue"))
	green := NewTestSpendSet(id("green"))
	yellow := NewTestSpendSet(id("yellow"))

	SpendA := NewTestSpender(transactionID("A"), nil, ds.NewSet(red), weight.New(), pendingTasks, thresholdProvider)
	SpendB := NewTestSpender(transactionID("B"), nil, ds.NewSet(red, blue), weight.New(), pendingTasks, thresholdProvider)
	SpendC := NewTestSpender(transactionID("C"), nil, ds.NewSet(green, blue), weight.New(), pendingTasks, thresholdProvider)
	SpendD := NewTestSpender(transactionID("D"), nil, ds.NewSet(green, yellow), weight.New(), pendingTasks, thresholdProvider)
	SpendE := NewTestSpender(transactionID("E"), nil, ds.NewSet(yellow), weight.New(), pendingTasks, thresholdProvider)

	return map[string]TestSpender{
		"SpendA": SpendA,
		"SpendB": SpendB,
		"SpendC": SpendC,
		"SpendD": SpendD,
		"SpendE": SpendE,
	}
}

func assertPreferredInstead(t *testing.T, preferredInsteadMap map[TestSpender]TestSpender) {
	for spender, preferredInsteadSpender := range preferredInsteadMap {
		assert.Equalf(t, preferredInsteadSpender.ID, spender.PreferredInstead().ID, "Spend %s should prefer %s instead of %s", spender.ID, preferredInsteadSpender.ID, spender.PreferredInstead().ID)
	}
}
