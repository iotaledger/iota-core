package storage_test

import "testing"

func TestStorage_Pruning(t *testing.T) {
	tf := NewTestFramework(t)
	defer tf.Shutdown()

	tf.GeneratePermanentData(100 * MB)
	tf.GeneratePrunableData(1, 100*MB)
}

func TestStorage_PruneByEpochIndex(t *testing.T) {
	// want to prune to a certain epoch index
	// need to check that the pruned epochs are not accessible anymore. specifically:
	//  - check all slot based storages
	//  - check for each epoch based storage specifically which epoch should or shouldn't be pruned

	// defaultPruningDelay < pruningDelay of epoch storages
	//  1. if pruning at > defaultPruningDelay, buckets should be pruned but not epoch storages
	//  2. if pruning at >= pruningDelay of epoch storages, buckets and epoch storages should be pruned

	// defaultPruningDelay >= pruningDelay of epoch storages
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
