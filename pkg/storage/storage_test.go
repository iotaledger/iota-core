package storage_test

import "testing"

func TestStorage_Pruning(t *testing.T) {
	tf := NewTestFramework(t)
	defer tf.Shutdown()

	tf.GeneratePermanentData(100 * MB)
	tf.GeneratePrunableData(1, 100*MB)
}
