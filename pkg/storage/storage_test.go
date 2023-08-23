package storage_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/iota-core/pkg/storage"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestStorage_PruneByEpochIndex(t *testing.T) {
	tf := NewTestFramework(t)
	defer tf.Shutdown()

	totalEpochs := 10
	tf.GeneratePermanentData(10 * MB)
	for i := 0; i <= totalEpochs; i++ {
		tf.GeneratePrunableData(iotago.EpochIndex(i), 10*KB)
		tf.GenerateSemiPermanentData(iotago.EpochIndex(i))
	}

	tf.SetLatestFinalizedEpoch(9)

	err := tf.Instance.PruneByEpochIndex(7)
	require.NoError(t, err)
	tf.AssertPrunedUntil(
		types.NewTuple(7, true),
		types.NewTuple(0, true),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
	)

	err = tf.Instance.PruneByEpochIndex(8)
	require.NoError(t, err)
	tf.AssertPrunedUntil(
		types.NewTuple(8, true),
		types.NewTuple(1, true),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
	)

	err = tf.Instance.PruneByEpochIndex(10)
	require.ErrorContains(t, err, "too new")

	err = tf.Instance.PruneByEpochIndex(8)
	require.ErrorContains(t, err, "too old")
}

func TestStorage_PruneByDepth(t *testing.T) {
	tf := NewTestFramework(t)
	defer tf.Shutdown()

	totalEpochs := 20
	tf.GeneratePermanentData(10 * MB)
	for i := 0; i <= totalEpochs; i++ {
		tf.GeneratePrunableData(iotago.EpochIndex(i), 10*KB)
		tf.GenerateSemiPermanentData(iotago.EpochIndex(i))
	}

	tf.SetLatestFinalizedEpoch(10)

	_, _, err := tf.Instance.PruneByDepth(21)
	require.ErrorContains(t, err, "too big")

	start, end, err := tf.Instance.PruneByDepth(4)
	require.NoError(t, err)
	require.EqualValues(t, 0, start)
	require.EqualValues(t, 6, end)
	tf.AssertPrunedUntil(
		types.NewTuple(6, true),
		types.NewTuple(3, true),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
	)

	tf.SetLatestFinalizedEpoch(20)

	start, end, err = tf.Instance.PruneByDepth(10)
	require.NoError(t, err)
	require.EqualValues(t, 7, start)
	require.EqualValues(t, 10, end)
	tf.AssertPrunedUntil(
		types.NewTuple(10, true),
		types.NewTuple(10, true),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
	)

	// last pruned: 10, should prune till 15
	start, end, err = tf.Instance.PruneByDepth(5)
	require.NoError(t, err)
	require.EqualValues(t, 11, start)
	require.EqualValues(t, 15, end)
	tf.AssertPrunedUntil(
		types.NewTuple(15, true),
		types.NewTuple(13, true),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
	)

	// last pruned: 15, should not prune as 20 - 6 = 14 which is < 15
	_, _, err = tf.Instance.PruneByDepth(6)
	require.ErrorContains(t, err, "pruned epoch is already 15")

	// last pruned: 15, should prune
	start, end, err = tf.Instance.PruneByDepth(2)
	require.NoError(t, err)
	require.EqualValues(t, 16, start)
	require.EqualValues(t, 18, end)
	tf.AssertPrunedUntil(
		types.NewTuple(18, true),
		types.NewTuple(13, true),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
	)
}

func TestStorage_PruneBySize(t *testing.T) {
	tf := NewTestFramework(t,
		storage.WithPruningDelay(2),
		storage.WithPruningSizeEnable(true),
		storage.WithPruningSizeMaxTargetSizeBytes(15*MB),
		storage.WithPruningSizeReductionPercentage(0.2),
		storage.WithPruningSizeCooldownTime(0),
	)
	defer tf.Shutdown()

	totalEpochs := 14
	tf.GeneratePermanentData(5 * MB)
	for i := 0; i <= totalEpochs; i++ {
		tf.GeneratePrunableData(iotago.EpochIndex(i), 120*KB)
		tf.GenerateSemiPermanentData(iotago.EpochIndex(i))
	}

	tf.SetLatestFinalizedEpoch(13)

	// db size < max size=15 MB, should NOT prune
	err := tf.Instance.PruneBySize()
	require.ErrorIs(t, err, database.ErrNoPruningNeeded)

	// prunable can't reach to pruned bytes size, should prune but return an insufficient pruning error
	err = tf.Instance.PruneBySize(4 * MB)
	require.ErrorIs(t, err, database.ErrDatabaseFull)
	tf.AssertPrunedUntil(
		types.NewTuple(12, true),
		types.NewTuple(5, true),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
	)

	tf.AssertStorageSizeBelow(6 * MB)

	// We already pruned the maximum and can't prune any more data right now.
	err = tf.Instance.PruneBySize(4 * MB)
	require.ErrorIs(t, err, database.ErrEpochPruned)
}

func TestStorage_RestoreFromDisk(t *testing.T) {
	tf := NewTestFramework(t, storage.WithPruningDelay(1))

	totalEpochs := 9
	tf.GeneratePermanentData(5 * MB)
	for i := 0; i <= totalEpochs; i++ {
		tf.GeneratePrunableData(iotago.EpochIndex(i), 1*B)
		tf.GenerateSemiPermanentData(iotago.EpochIndex(i))
	}

	tf.SetLatestFinalizedEpoch(8)

	// restore from disk
	tf.RestoreFromDisk()

	tf.AssertPrunedUntil(
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
	)

	err := tf.Instance.PruneByEpochIndex(7)
	require.NoError(t, err)
	tf.AssertPrunedUntil(
		types.NewTuple(7, true),
		types.NewTuple(0, true),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
	)

	// restore from disk
	tf.RestoreFromDisk()

	tf.AssertPrunedUntil(
		types.NewTuple(7, true),
		types.NewTuple(0, true),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
	)
}
