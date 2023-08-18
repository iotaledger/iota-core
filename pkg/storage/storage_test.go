package storage_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/iota-core/pkg/storage"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestStorage_PruneByEpochIndex_SmallerDefault(t *testing.T) {
	tf := NewTestFramework(t, storage.WithPruningDelay(1))
	defer tf.Shutdown()

	totalEpochs := 10
	tf.GeneratePermanentData(10 * MB)
	for i := 1; i <= totalEpochs; i++ {
		tf.GeneratePrunableData(iotago.EpochIndex(i), 10*KB)
		tf.GenerateSemiPermanentData(iotago.EpochIndex(i))
	}

	tf.SetLatestFinalizedEpoch(9)

	// 7 > default pruning delay 1, should prune
	err := tf.Instance.PruneByEpochIndex(7)
	require.NoError(t, err)
	tf.AssertPrunedUntil(
		types.NewTuple(6, true),
		types.NewTuple(0, true),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
	)

	// 8 > default pruning delay 1, should prune
	err = tf.Instance.PruneByEpochIndex(8)
	require.NoError(t, err)
	tf.AssertPrunedUntil(
		types.NewTuple(7, true),
		types.NewTuple(1, true),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
	)
}

func TestStorage_PruneByEpochIndex_BiggerDefault(t *testing.T) {
	tf := NewTestFramework(t, storage.WithPruningDelay(10))
	defer tf.Shutdown()

	totalEpochs := 14
	tf.GeneratePermanentData(10 * MB)
	for i := 0; i <= totalEpochs; i++ {
		tf.GeneratePrunableData(iotago.EpochIndex(i), 10*KB)
		tf.GenerateSemiPermanentData(iotago.EpochIndex(i))
	}

	tf.SetLatestFinalizedEpoch(13)

	// 7 < default pruning delay 10, should NOT prune
	err := tf.Instance.PruneByEpochIndex(7)
	require.ErrorContains(t, err, database.ErrNoPruningNeeded.Error())

	tf.AssertPrunedUntil(
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
	)

	// 10 == default pruning delay 10, should NOT prune
	// FIXME: this scenario fails bc the slotEnd of epoch 0 is slotIndex(0),
	// however, slotIndex(0) is converted to epochIndex(1)
	// thus the assert function always check on epoch 1 instead of epoch 0.
	// err = tf.Instance.PruneByEpochIndex(10)
	// require.NoError(t, err)

	// tf.AssertPrunedUntil(
	// 	types.NewTuple(0, true),
	// 	types.NewTuple(0, true),
	// 	types.NewTuple(0, false),
	// 	types.NewTuple(0, false),
	// 	types.NewTuple(0, false),
	// )

	// 12 > default pruning delay 10, should prune
	tf.Instance.PruneByEpochIndex(12)
	tf.AssertPrunedUntil(
		types.NewTuple(2, true),
		types.NewTuple(2, true),
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
