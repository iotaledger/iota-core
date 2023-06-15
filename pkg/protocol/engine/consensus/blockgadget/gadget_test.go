package blockgadget_test

import (
	"testing"
)

func TestBlockGadget(t *testing.T) {
	tf := NewTestFramework(t)

	tf.AddAccount("A", 50)
	tf.AddAccount("B", 50)

	tf.TrackWitnessWeight("A.1", "A", "Genesis")
	tf.TrackWitnessWeight("B.1", "B", "Genesis")

	tf.TrackWitnessWeight("A.2", "A", "A.1", "B.1")
	tf.TrackWitnessWeight("B.2", "B", "A.1", "B.1")

	tf.TrackWitnessWeight("A.3", "A", "A.2", "B.2")
	tf.TrackWitnessWeight("B.3", "B", "A.2", "B.2")

	tf.AssertBlocksPreAccepted(tf.Blocks("A.1", "B.1", "A.2", "B.2"), true)
	tf.AssertBlocksAccepted(tf.Blocks("A.1", "B.1"), false)

	tf.SetOnline("A")
	tf.SetOnline("B")

	tf.TrackWitnessWeight("A.4", "A", "A.3", "B.3")
	tf.TrackWitnessWeight("B.4", "B", "A.3", "B.3")

	tf.AssertBlocksAccepted(tf.Blocks("A.1", "B.1", "A.2", "B.2"), true)
}
