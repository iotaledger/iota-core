package slotattestation_test

import (
	"testing"
)

func TestManager(t *testing.T) {
	tf := NewTestFramework(t)

	tf.AssertCommit(0, 0, map[string]string{}, true)

	// Slot 1
	{
		tf.AddFutureAttestation("A", "A.1-0", 1, 0)
		tf.AddFutureAttestation("B", "B.1-0", 1, 0)
		tf.AddFutureAttestation("B", "B.1.2-0", 1, 0)
		tf.AddFutureAttestation("C", "C.1-0", 1, 0)

		tf.AssertCommit(1, 0, map[string]string{}, true)
	}

	// Slot 2
	{
		tf.AddFutureAttestation("A", "A.2-0", 2, 0)
		// This should not have any effect.
		tf.AddFutureAttestation("C", "C.3-2", 3, 2)

		tf.AssertCommit(2, 3, map[string]string{
			"A": "A.2-0",
			"B": "B.1.2-0",
			"C": "C.1-0",
		})
	}

	// Slot 3
	{
		tf.AddFutureAttestation("A", "A.3-0", 3, 0)
		tf.AddFutureAttestation("B", "B.3-1", 3, 1)

		tf.AssertCommit(3, 5, map[string]string{
			"B": "B.3-1",
			"C": "C.3-2",
		})
	}

	// Slot 4
	{
		tf.AssertCommit(4, 6, map[string]string{
			"C": "C.3-2",
		})
	}

	// Slot 5
	{
		tf.AssertCommit(5, 6, map[string]string{})
	}

	// Slot 6
	{
		tf.AddFutureAttestation("A", "A.6-5", 6, 5)
		tf.AddFutureAttestation("A", "A.6-4", 6, 4)

		tf.AddFutureAttestation("B", "B.6-4", 6, 4)

		tf.AddFutureAttestation("C", "C.6-3", 6, 3)

		tf.AssertCommit(6, 8, map[string]string{
			"A": "A.6-5",
			"B": "B.6-4",
		})
	}

	// Slot 7
	{
		tf.AssertCommit(7, 9, map[string]string{
			"A": "A.6-5",
		})
	}
}
