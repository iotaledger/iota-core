package storage_test

import (
	"testing"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/iota-core/pkg/storage"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestStorage_Pruning(t *testing.T) {
	tf := NewTestFramework(t)
	defer tf.Shutdown()

	tf.GeneratePermanentData(100 * MB)
	tf.GeneratePrunableData(1, 100*MB)
}

func TestStorage_PruneByEpochIndex_SmallerDefault(t *testing.T) {
	tf := NewTestFramework(t, storage.WithPruningDelay(1))
	defer tf.Shutdown()

	totalEpochs := 10
	tf.GeneratePermanentData(10 * MB)
	for i := 1; i <= totalEpochs; i++ {
		tf.GeneratePrunableData(iotago.EpochIndex(i), 10*KB)
		tf.GenerateSemiPermanentData(iotago.EpochIndex(i))
	}

	tf.Instance.PruneByEpochIndex(7)
	tf.AssertPrunedUntil(
		types.NewTuple(6, true),
		types.NewTuple(0, true),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
	)

	tf.Instance.PruneByEpochIndex(8)
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
	for i := 1; i <= totalEpochs; i++ {
		tf.GeneratePrunableData(iotago.EpochIndex(i), 10*KB)
		tf.GenerateSemiPermanentData(iotago.EpochIndex(i))
	}

	tf.Instance.PruneByEpochIndex(7)
	tf.AssertPrunedUntil(
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
	)

	tf.Instance.PruneByEpochIndex(10)
	tf.AssertPrunedUntil(
		types.NewTuple(0, true),
		types.NewTuple(0, true),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
		types.NewTuple(0, false),
	)

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
	// need to add some stuff to permanent storage for multiple epochs
	// need to add some stuff to prunable bucket and semipermanent storages for multiple epochs

	// execute goroutine that monitors the size of the database and prunes if necessary

	// special cases:
	//  - permanent is already bigger than target size
}

func TestStorage_RestoreFromDisk(t *testing.T) {

}
