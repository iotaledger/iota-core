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
	for i := 1; i <= totalEpochs; i++ {
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

	totalEpochs := 10
	tf.GeneratePermanentData(10 * MB)
	for i := 1; i <= totalEpochs; i++ {
		tf.GeneratePrunableData(iotago.EpochIndex(i), 10*KB)
		tf.GenerateSemiPermanentData(iotago.EpochIndex(i))
	}

	tf.SetLatestFinalizedEpoch(9)

	_, _, err := tf.Instance.PruneByDepth(10)
	require.ErrorContains(t, err, "too big")

	start, end, err := tf.Instance.PruneByDepth(5)
	require.NoError(t, err)
	require.EqualValues(t, 0, start)
	require.EqualValues(t, 4, end)
	tf.AssertPrunedUntil(
		types.NewTuple(4, true),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
	)

	_, _, err = tf.Instance.PruneByDepth(6)
	require.ErrorContains(t, err, "pruned epoch is already")

	start, end, err = tf.Instance.PruneByDepth(2)
	require.NoError(t, err)
	require.EqualValues(t, 5, start)
	require.EqualValues(t, 7, end)
	tf.AssertPrunedUntil(
		types.NewTuple(7, true),
		types.NewTuple(0, true),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
	)
}

func TestStorage_PruneBySize(t *testing.T) {
	tf := NewTestFramework(t,
		storage.WithPruningDelay(2),
		storage.WithPruningSizeEnable(true),
		storage.WithPruningSizeMaxTargetSizeBytes(10*MB))
	defer tf.Shutdown()

	totalEpochs := 14
	tf.GeneratePermanentData(5 * MB)
	for i := 1; i <= totalEpochs; i++ {
		tf.GeneratePrunableData(iotago.EpochIndex(i), 120*KB)
		tf.GenerateSemiPermanentData(iotago.EpochIndex(i))
	}

	tf.SetLatestFinalizedEpoch(13)

	// db size < target size 10 MB, should NOT prune
	err := tf.Instance.PruneBySize()
	require.ErrorContains(t, err, database.ErrNoPruningNeeded.Error())

	// prunable can't reached to pruned bytes size, should NOT prune
	err = tf.Instance.PruneBySize(4 * MB)
	require.ErrorContains(t, err, database.ErrNotEnoughHistory.Error())

	// prunable can reached to pruned bytes size, should prune
	err = tf.Instance.PruneBySize(7 * MB)
	require.NoError(t, err)
	require.LessOrEqual(t, tf.Instance.Size(), 7*MB)

	// execute goroutine that monitors the size of the database and prunes if necessary

	// special cases:
	//  - permanent is already bigger than target size
}

func TestStorage_RestoreFromDisk(t *testing.T) {
	// TODO: we need to restore the last pruned epoch for storage
	tf := NewTestFramework(t, storage.WithPruningDelay(1))

	totalEpochs := 370
	tf.GeneratePermanentData(5 * MB)
	for i := 1; i <= totalEpochs; i++ {
		tf.GeneratePrunableData(iotago.EpochIndex(i), 1*B)
		tf.GenerateSemiPermanentData(iotago.EpochIndex(i))
	}

	tf.SetLatestFinalizedEpoch(366)

	tf.Instance.PruneByEpochIndex(366)
	tf.AssertPrunedUntil(
		types.NewTuple(365, true),
		types.NewTuple(359, true),
		types.NewTuple(1, true),
		types.NewTuple(1, true),
		types.NewTuple(1, true),
	)

	// restore from disk
	tf.RestoreFromDisk()

	tf.AssertPrunedUntil(
		types.NewTuple(365, true),
		types.NewTuple(359, true),
		types.NewTuple(1, true),
		types.NewTuple(1, true),
		types.NewTuple(1, true),
	)
}
