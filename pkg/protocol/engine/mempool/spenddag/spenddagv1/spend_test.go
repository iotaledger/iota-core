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

type TestSpend = *Spend[iotago.TransactionID, iotago.OutputID, vote.MockedRank]

//var NewTestSpend = NewSpend[iotago.TransactionID, iotago.OutputID, vote.MockedRank]

func NewTestSpend(id iotago.TransactionID, parentSpends ds.Set[*Spend[iotago.TransactionID, iotago.OutputID, vote.MockedRank]], SpendSets ds.Set[*ConflictSet[iotago.TransactionID, iotago.OutputID, vote.MockedRank]], initialWeight *weight.Weight, pendingTasksCounter *syncutils.Counter, acceptanceThresholdProvider func() int64) *Spend[iotago.TransactionID, iotago.OutputID, vote.MockedRank] {
	spend := NewSpend[iotago.TransactionID, iotago.OutputID, vote.MockedRank](id, initialWeight, pendingTasksCounter, acceptanceThresholdProvider)
	_, err := spend.JoinSpendSets(SpendSets)
	if err != nil {
		// TODO: change this
		panic(err)
	}
	spend.UpdateParents(parentSpends, ds.NewSet[*Spend[iotago.TransactionID, iotago.OutputID, vote.MockedRank]]())

	return spend
}

func TestSpend_SetRejected(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	Spend1 := NewTestSpend(transactionID("Spend1"), nil, nil, weight.New(), pendingTasks, thresholdProvider)
	Spend2 := NewTestSpend(transactionID("Spend2"), ds.NewSet(Spend1), nil, weight.New(), pendingTasks, thresholdProvider)
	Spend3 := NewTestSpend(transactionID("Spend3"), ds.NewSet(Spend2), nil, weight.New(), pendingTasks, thresholdProvider)

	Spend1.setAcceptanceState(acceptance.Rejected)
	require.True(t, Spend1.IsRejected())
	require.True(t, Spend2.IsRejected())
	require.True(t, Spend3.IsRejected())

	Spend4 := NewTestSpend(transactionID("Spend4"), ds.NewSet(Spend1), nil, weight.New(), pendingTasks, thresholdProvider)
	require.True(t, Spend4.IsRejected())
}

func TestSpend_UpdateParents(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	Spend1 := NewTestSpend(transactionID("Spend1"), nil, nil, weight.New(), pendingTasks, thresholdProvider)
	Spend2 := NewTestSpend(transactionID("Spend2"), nil, nil, weight.New(), pendingTasks, thresholdProvider)
	Spend3 := NewTestSpend(transactionID("Spend3"), ds.NewSet(Spend1, Spend2), nil, weight.New(), pendingTasks, thresholdProvider)

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
		ConflictSet1 := NewTestConflictSet(id("ConflictSet1"))
		ConflictSet2 := NewTestConflictSet(id("ConflictSet2"))

		Spend1 := NewTestSpend(transactionID("Spend1"), nil, ds.NewSet(ConflictSet1), weight.New(), pendingTasks, thresholdProvider)
		Spend2 := NewTestSpend(transactionID("Spend2"), nil, ds.NewSet(ConflictSet1, ConflictSet2), weight.New(), pendingTasks, thresholdProvider)
		Spend3 := NewTestSpend(transactionID("Spend3"), nil, ds.NewSet(ConflictSet2), weight.New(), pendingTasks, thresholdProvider)

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
		SpendSet1 := NewTestConflictSet(id("ConflictSet1"))
		SpendSet2 := NewTestConflictSet(id("ConflictSet2"))

		Spend1 := NewTestSpend(transactionID("Spend1"), nil, ds.NewSet(SpendSet1), weight.New(), pendingTasks, thresholdProvider)
		Spend2 := NewTestSpend(transactionID("Spend2"), nil, ds.NewSet(SpendSet1, SpendSet2), weight.New(), pendingTasks, thresholdProvider)
		Spend3 := NewTestSpend(transactionID("Spend3"), nil, ds.NewSet(SpendSet2), weight.New(), pendingTasks, thresholdProvider)

		Spend2.setAcceptanceState(acceptance.Accepted)
		require.True(t, Spend1.IsRejected())
		require.True(t, Spend2.IsAccepted())
		require.True(t, Spend3.IsRejected())
	}
}

func TestSpend_ConflictSets(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	red := NewTestConflictSet(id("red"))
	blue := NewTestConflictSet(id("blue"))
	green := NewTestConflictSet(id("green"))
	yellow := NewTestConflictSet(id("yellow"))

	SpendA := NewTestSpend(transactionID("A"), nil, ds.NewSet(red), weight.New().AddCumulativeWeight(7), pendingTasks, thresholdProvider)
	SpendB := NewTestSpend(transactionID("B"), nil, ds.NewSet(red, blue), weight.New().AddCumulativeWeight(3), pendingTasks, thresholdProvider)
	SpendC := NewTestSpend(transactionID("C"), nil, ds.NewSet(blue, green), weight.New().AddCumulativeWeight(5), pendingTasks, thresholdProvider)
	SpendD := NewTestSpend(transactionID("D"), nil, ds.NewSet(green, yellow), weight.New().AddCumulativeWeight(7), pendingTasks, thresholdProvider)
	SpendE := NewTestSpend(transactionID("E"), nil, ds.NewSet(yellow), weight.New().AddCumulativeWeight(9), pendingTasks, thresholdProvider)

	preferredInsteadMap := map[TestSpend]TestSpend{
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

	assertPreferredInstead(t, lo.MergeMaps(preferredInsteadMap, map[TestSpend]TestSpend{
		SpendC: SpendD,
		SpendD: SpendD,
		SpendE: SpendD,
	}))

	SpendD.Weight.SetCumulativeWeight(0)
	pendingTasks.WaitIsZero()

	assertPreferredInstead(t, lo.MergeMaps(preferredInsteadMap, map[TestSpend]TestSpend{
		SpendC: SpendC,
		SpendD: SpendE,
		SpendE: SpendE,
	}))

	SpendC.Weight.SetCumulativeWeight(8)
	pendingTasks.WaitIsZero()

	assertPreferredInstead(t, lo.MergeMaps(preferredInsteadMap, map[TestSpend]TestSpend{
		SpendB: SpendC,
	}))

	SpendC.Weight.SetCumulativeWeight(8)
	pendingTasks.WaitIsZero()

	assertPreferredInstead(t, lo.MergeMaps(preferredInsteadMap, map[TestSpend]TestSpend{
		SpendB: SpendC,
	}))

	SpendD.Weight.SetCumulativeWeight(3)
	pendingTasks.WaitIsZero()

	assertPreferredInstead(t, preferredInsteadMap)

	SpendE.Weight.SetCumulativeWeight(1)
	pendingTasks.WaitIsZero()

	assertPreferredInstead(t, lo.MergeMaps(preferredInsteadMap, map[TestSpend]TestSpend{
		SpendD: SpendC,
	}))

	SpendE.Weight.SetCumulativeWeight(9)
	pendingTasks.WaitIsZero()

	assertPreferredInstead(t, lo.MergeMaps(preferredInsteadMap, map[TestSpend]TestSpend{
		SpendD: SpendE,
	}))

	SpendF := NewTestSpend(transactionID("F"), nil, ds.NewSet(yellow), weight.New().AddCumulativeWeight(19), pendingTasks, thresholdProvider)

	pendingTasks.WaitIsZero()

	assertPreferredInstead(t, lo.MergeMaps(preferredInsteadMap, map[TestSpend]TestSpend{
		SpendD: SpendF,
		SpendE: SpendF,
		SpendF: SpendF,
	}))

	assertCorrectOrder(t, SpendA, SpendB, SpendC, SpendD, SpendE, SpendF)
}

func TestSpendParallel(t *testing.T) {
	sequentialPendingTasks := syncutils.NewCounter()
	parallelPendingTasks := syncutils.NewCounter()

	sequentialSpends := createSpends(sequentialPendingTasks)
	sequentialPendingTasks.WaitIsZero()

	parallelSpends := createSpends(parallelPendingTasks)
	parallelPendingTasks.WaitIsZero()

	const updateCount = 100000

	permutations := make([]func(Spend TestSpend), 0)
	for i := 0; i < updateCount; i++ {
		permutations = append(permutations, generateRandomSpendPermutation())
	}

	var wg sync.WaitGroup
	for _, permutation := range permutations {
		targetAlias := lo.Keys(parallelSpends)[rand.Intn(len(parallelSpends))]

		permutation(sequentialSpends[targetAlias])

		wg.Add(1)
		go func(permutation func(Spend TestSpend)) {
			permutation(parallelSpends[targetAlias])

			wg.Done()
		}(permutation)
	}

	sequentialPendingTasks.WaitIsZero()

	wg.Wait()

	parallelPendingTasks.WaitIsZero()

	lo.ForEach(lo.Keys(parallelSpends), func(SpendAlias string) {
		assert.EqualValuesf(t, sequentialSpends[SpendAlias].PreferredInstead().ID, parallelSpends[SpendAlias].PreferredInstead().ID, "parallel Spend %s prefers %s, but sequential Spend prefers %s", SpendAlias, parallelSpends[SpendAlias].PreferredInstead().ID, sequentialSpends[SpendAlias].PreferredInstead().ID)
	})

	assertCorrectOrder(t, lo.Values(sequentialSpends)...)
	assertCorrectOrder(t, lo.Values(parallelSpends)...)
}

func TestLikedInstead1(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())
	pendingTasks := syncutils.NewCounter()

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	masterBranch := NewTestSpend(transactionID("M"), nil, nil, weight.New(), pendingTasks, thresholdProvider)
	require.True(t, masterBranch.IsLiked())
	require.True(t, masterBranch.LikedInstead().IsEmpty())

	SpendSet1 := NewTestConflictSet(id("O1"))

	Spend1 := NewTestSpend(transactionID("TxA"), ds.NewSet(masterBranch), ds.NewSet(SpendSet1), weight.New().SetCumulativeWeight(6), pendingTasks, thresholdProvider)
	Spend2 := NewTestSpend(transactionID("TxB"), ds.NewSet(masterBranch), ds.NewSet(SpendSet1), weight.New().SetCumulativeWeight(3), pendingTasks, thresholdProvider)

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

	masterBranch := NewTestSpend(transactionID("M"), nil, nil, weight.New(), pendingTasks, thresholdProvider)
	require.True(t, masterBranch.IsLiked())
	require.True(t, masterBranch.LikedInstead().IsEmpty())

	SpendSet1 := NewTestConflictSet(id("O1"))
	SpendA := NewTestSpend(transactionID("TxA"), ds.NewSet(masterBranch), ds.NewSet(SpendSet1), weight.New().SetCumulativeWeight(200), pendingTasks, thresholdProvider)
	SpendB := NewTestSpend(transactionID("TxB"), ds.NewSet(masterBranch), ds.NewSet(SpendSet1), weight.New().SetCumulativeWeight(100), pendingTasks, thresholdProvider)

	require.True(t, SpendA.IsPreferred())
	require.True(t, SpendA.IsLiked())
	require.Equal(t, 0, SpendA.LikedInstead().Size())

	require.False(t, SpendB.IsPreferred())
	require.False(t, SpendB.IsLiked())
	require.Equal(t, 1, SpendB.LikedInstead().Size())
	require.True(t, SpendB.LikedInstead().Has(SpendA))

	SpendSet2 := NewTestConflictSet(id("O2"))
	SpendC := NewTestSpend(transactionID("TxC"), ds.NewSet(SpendA), ds.NewSet(SpendSet2), weight.New().SetCumulativeWeight(200), pendingTasks, thresholdProvider)
	SpendD := NewTestSpend(transactionID("TxD"), ds.NewSet(SpendA), ds.NewSet(SpendSet2), weight.New().SetCumulativeWeight(100), pendingTasks, thresholdProvider)

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

	masterBranch := NewTestSpend(transactionID("M"), nil, nil, weight.New(), pendingTasks, thresholdProvider)
	require.True(t, masterBranch.IsLiked())
	require.True(t, masterBranch.LikedInstead().IsEmpty())

	SpendSet1 := NewTestConflictSet(id("O1"))
	SpendA := NewTestSpend(transactionID("TxA"), ds.NewSet(masterBranch), ds.NewSet(SpendSet1), weight.New().SetCumulativeWeight(200), pendingTasks, thresholdProvider)
	SpendB := NewTestSpend(transactionID("TxB"), ds.NewSet(masterBranch), ds.NewSet(SpendSet1), weight.New().SetCumulativeWeight(100), pendingTasks, thresholdProvider)

	require.True(t, SpendA.IsPreferred())
	require.True(t, SpendA.IsLiked())
	require.Equal(t, 0, SpendA.LikedInstead().Size())

	require.False(t, SpendB.IsPreferred())
	require.False(t, SpendB.IsLiked())
	require.Equal(t, 1, SpendB.LikedInstead().Size())
	require.True(t, SpendB.LikedInstead().Has(SpendA))

	SpendSet4 := NewTestConflictSet(id("O4"))
	SpendF := NewTestSpend(transactionID("TxF"), ds.NewSet(SpendA), ds.NewSet(SpendSet4), weight.New().SetCumulativeWeight(20), pendingTasks, thresholdProvider)
	SpendG := NewTestSpend(transactionID("TxG"), ds.NewSet(SpendA), ds.NewSet(SpendSet4), weight.New().SetCumulativeWeight(10), pendingTasks, thresholdProvider)

	require.True(t, SpendF.IsPreferred())
	require.True(t, SpendF.IsLiked())
	require.Equal(t, 0, SpendF.LikedInstead().Size())

	require.False(t, SpendG.IsPreferred())
	require.False(t, SpendG.IsLiked())
	require.Equal(t, 1, SpendG.LikedInstead().Size())
	require.True(t, SpendG.LikedInstead().Has(SpendF))

	SpendSet2 := NewTestConflictSet(id("O2"))
	SpendC := NewTestSpend(transactionID("TxC"), ds.NewSet(masterBranch), ds.NewSet(SpendSet2), weight.New().SetCumulativeWeight(200), pendingTasks, thresholdProvider)
	SpendH := NewTestSpend(transactionID("TxH"), ds.NewSet(masterBranch, SpendA), ds.NewSet(SpendSet2, SpendSet4), weight.New().SetCumulativeWeight(150), pendingTasks, thresholdProvider)

	require.True(t, SpendC.IsPreferred())
	require.True(t, SpendC.IsLiked())
	require.Equal(t, 0, SpendC.LikedInstead().Size())

	require.False(t, SpendH.IsPreferred())
	require.False(t, SpendH.IsLiked())
	require.Equal(t, 1, SpendH.LikedInstead().Size())
	require.True(t, SpendH.LikedInstead().Has(SpendC))

	SpendSet3 := NewTestConflictSet(id("O12"))
	SpendI := NewTestSpend(transactionID("TxI"), ds.NewSet(SpendF), ds.NewSet(SpendSet3), weight.New().SetCumulativeWeight(5), pendingTasks, thresholdProvider)
	SpendJ := NewTestSpend(transactionID("TxJ"), ds.NewSet(SpendF), ds.NewSet(SpendSet3), weight.New().SetCumulativeWeight(15), pendingTasks, thresholdProvider)

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

func TestConflictSet_AllMembersEvicted(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())

	pendingTasks := syncutils.NewCounter()
	yellow := NewTestConflictSet(id("yellow"))
	green := NewTestConflictSet(id("green"))

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	Spend1 := NewTestSpend(transactionID("Spend1"), nil, ds.NewSet(yellow), weight.New(), pendingTasks, thresholdProvider)
	evictedSpends := Spend1.Evict()
	require.Len(t, evictedSpends, 1)
	require.Contains(t, evictedSpends, Spend1.ID)

	// evict the Spend another time and make sure that none Spends were evicted
	evictedSpends = Spend1.Evict()
	require.Len(t, evictedSpends, 0)

	// Spend tries to join Spendset who's all members were evicted
	Spend2 := NewSpend[iotago.TransactionID, iotago.OutputID, vote.MockedRank](transactionID("Spend1"), weight.New(), pendingTasks, thresholdProvider)
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

	var Spend1, Spend2 TestSpend

	Spend1 = NewTestSpend(transactionID("M"), nil, nil, weight.New(), pendingTasks, thresholdProvider)

	require.Equal(t, weight.Heavier, Spend1.Compare(nil))
	require.Equal(t, weight.Lighter, Spend2.Compare(Spend1))
	require.Equal(t, weight.Equal, Spend2.Compare(nil))
}

func TestSpend_Inheritance(t *testing.T) {
	weights := account.NewSeatedAccounts(account.NewAccounts())

	pendingTasks := syncutils.NewCounter()
	yellow := NewTestConflictSet(id("yellow"))
	green := NewTestConflictSet(id("green"))

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	Spend1 := NewTestSpend(transactionID("Spend1"), nil, ds.NewSet(yellow), weight.New().SetCumulativeWeight(1), pendingTasks, thresholdProvider)
	Spend2 := NewTestSpend(transactionID("Spend2"), nil, ds.NewSet(green), weight.New().SetCumulativeWeight(1), pendingTasks, thresholdProvider)
	Spend3 := NewTestSpend(transactionID("Spend3"), ds.NewSet(Spend1, Spend2), nil, weight.New(), pendingTasks, thresholdProvider)
	Spend4 := NewTestSpend(transactionID("Spend4"), nil, ds.NewSet(yellow, green), weight.New(), pendingTasks, thresholdProvider)

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
	Spend5 := NewTestSpend(transactionID("Spend5"), ds.NewSet(Spend3), nil, weight.New(), pendingTasks, thresholdProvider)
	pendingTasks.WaitIsZero()
	require.True(t, Spend5.LikedInstead().Has(Spend4))

	Spend1.Weight.SetCumulativeWeight(15)
	pendingTasks.WaitIsZero()
	require.True(t, Spend3.LikedInstead().IsEmpty())
}

func assertCorrectOrder(t *testing.T, spends ...TestSpend) {
	sort.Slice(spends, func(i, j int) bool {
		return spends[i].Compare(spends[j]) == weight.Heavier
	})

	preferredSpends := ds.NewSet[TestSpend]()
	unPreferredSpends := ds.NewSet[TestSpend]()

	for _, spend := range spends {
		if !unPreferredSpends.Has(spend) {
			preferredSpends.Add(spend)
			spend.ConflictingSpends.Range(func(conflictingSpend *Spend[iotago.TransactionID, iotago.OutputID, vote.MockedRank]) {
				if spend != conflictingSpend {
					unPreferredSpends.Add(conflictingSpend)
				}
			}, true)
		}
	}

	for _, Spend := range spends {
		if preferredSpends.Has(Spend) {
			require.True(t, Spend.IsPreferred(), "Spend %s should be preferred", Spend.ID)
		}
		if unPreferredSpends.Has(Spend) {
			require.False(t, Spend.IsPreferred(), "Spend %s should be unPreferred", Spend.ID)
		}
	}

	_ = unPreferredSpends.ForEach(func(unPreferredSpend TestSpend) (err error) {
		// iterating in descending order, so the first preferred Spend
		return unPreferredSpend.ConflictingSpends.ForEach(func(ConflictingSpend TestSpend) error {
			if ConflictingSpend != unPreferredSpend && ConflictingSpend.IsPreferred() {
				require.Equal(t, ConflictingSpend, unPreferredSpend.PreferredInstead())

				return ierrors.New("break the loop")
			}

			return nil
		}, true)
	})
}

func generateRandomSpendPermutation() func(spend TestSpend) {
	updateType := rand.Intn(100)
	delta := rand.Intn(100)

	return func(spend TestSpend) {
		if updateType%2 == 0 {
			spend.Weight.AddCumulativeWeight(int64(delta))
		} else {
			spend.Weight.RemoveCumulativeWeight(int64(delta))
		}
	}
}

func createSpends(pendingTasks *syncutils.Counter) map[string]TestSpend {
	weights := account.NewSeatedAccounts(account.NewAccounts())

	thresholdProvider := acceptance.ThresholdProvider(func() int64 {
		return int64(weights.SeatCount())
	})

	red := NewTestConflictSet(id("red"))
	blue := NewTestConflictSet(id("blue"))
	green := NewTestConflictSet(id("green"))
	yellow := NewTestConflictSet(id("yellow"))

	SpendA := NewTestSpend(transactionID("A"), nil, ds.NewSet(red), weight.New(), pendingTasks, thresholdProvider)
	SpendB := NewTestSpend(transactionID("B"), nil, ds.NewSet(red, blue), weight.New(), pendingTasks, thresholdProvider)
	SpendC := NewTestSpend(transactionID("C"), nil, ds.NewSet(green, blue), weight.New(), pendingTasks, thresholdProvider)
	SpendD := NewTestSpend(transactionID("D"), nil, ds.NewSet(green, yellow), weight.New(), pendingTasks, thresholdProvider)
	SpendE := NewTestSpend(transactionID("E"), nil, ds.NewSet(yellow), weight.New(), pendingTasks, thresholdProvider)

	return map[string]TestSpend{
		"SpendA": SpendA,
		"SpendB": SpendB,
		"SpendC": SpendC,
		"SpendD": SpendD,
		"SpendE": SpendE,
	}
}

func assertPreferredInstead(t *testing.T, preferredInsteadMap map[TestSpend]TestSpend) {
	for Spend, preferredInsteadSpend := range preferredInsteadMap {
		assert.Equalf(t, preferredInsteadSpend.ID, Spend.PreferredInstead().ID, "Spend %s should prefer %s instead of %s", Spend.ID, preferredInsteadSpend.ID, Spend.PreferredInstead().ID)
	}
}
