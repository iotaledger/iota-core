package tests

import (
	"testing"

	"github.com/iotaledger/iota.go/v4/api"
)

// TestBlockRetainer_RetainBlock tests the BlockRetainer.
// A: BlockBooked  -> BlockAccepted -> BlockConfirmed -> slot finalized
// A2: BlockBooked -> BlockAccepted -> BlockConfirmed -> slot finalized (issued one slot later)
// B: BlockBooked  -> BlockAccepted ->                -> slot finalized
// C: BlockBooked  -> BlockDropped
// D: BlockBooked  -> BlockDropped  -> BlockAccepted  -> slot finalized
// E: BlockBooked
func TestBlockRetainer_RetainBlockNoFailures(t *testing.T) {
	tf := NewTestFramework(t)

	{
		tf.commitSlot(2)
		tf.finalizeSlot(0)
		tf.initiateRetainerBlockFlow(5, []string{"A", "B"})
	}
	{
		tf.commitSlot(3)
		tf.finalizeSlot(0)
		tf.initiateRetainerBlockFlow(6, []string{"A2", "C", "D", "E"})
	}
	{
		tf.commitSlot(4)
		tf.finalizeSlot(1)
		tf.assertBlockMetadata([]*BlockRetainerAction{
			{
				"A",
				eventAccepted,
				api.BlockStateAccepted,
			},
			{
				"A2",
				eventAccepted,
				api.BlockStateAccepted,
			},
			{
				"B",
				eventAccepted,
				api.BlockStateAccepted,
			},
			{
				"C",
				eventDropped,
				api.BlockStateDropped,
			},
			{
				"D",
				eventDropped,
				api.BlockStateDropped,
			},
			{
				"E",
				none,
				api.BlockStatePending,
			},
		})
	}
	{
		tf.commitSlot(5)
		tf.finalizeSlot(2)
		tf.assertBlockMetadata([]*BlockRetainerAction{
			{
				"A",
				eventConfirmed,
				api.BlockStateConfirmed,
			},
			{
				"A2",
				eventConfirmed,
				api.BlockStateConfirmed,
			},
			{
				"B",
				none,
				api.BlockStateAccepted,
			},
			{
				"C",
				none,
				api.BlockStateDropped,
			},
			{
				"D",
				eventAccepted,
				api.BlockStateAccepted,
			},
			{
				"E",
				none,
				api.BlockStatePending,
			},
		})

	}
	{
		tf.commitSlot(10)
		tf.finalizeSlot(5)

		tf.assertBlockMetadata([]*BlockRetainerAction{
			{
				"A",
				none,
				api.BlockStateFinalized,
			},
			{
				"A2",
				none,
				api.BlockStateConfirmed,
			},
			{
				"B",
				none,
				api.BlockStateFinalized,
			},
			{
				"C",
				none,
				api.BlockStateOrphaned,
			},
			{
				"D",
				none,
				api.BlockStateAccepted,
			},
			{
				"E",
				none,
				api.BlockStateOrphaned,
			},
		})
	}

	{
		tf.commitSlot(11)
		tf.finalizeSlot(6)

		tf.assertBlockMetadata([]*BlockRetainerAction{
			{
				"A",
				none,
				api.BlockStateFinalized,
			},
			{
				"A2",
				none,
				api.BlockStateFinalized,
			},
			{
				"B",
				none,
				api.BlockStateFinalized,
			},
			{
				"C",
				none,
				api.BlockStateOrphaned,
			},
			{
				"D",
				none,
				api.BlockStateFinalized,
			},
			{
				"E",
				none,
				api.BlockStateOrphaned,
			},
		})
	}
}

// TestBlockRetainer_Dropped makes sure that it is not possible to overwrite Accepted or higher states with Dropped.
// And also we return an error if we try to drop a block that is already committed, as this should never happen.
func TestBlockRetainer_Dropped(t *testing.T) {
	tf := NewTestFramework(t)

	{
		tf.commitSlot(2)
		tf.finalizeSlot(0)
		tf.initiateRetainerBlockFlow(5, []string{"A"})
	}
	{
		tf.commitSlot(4)
		tf.finalizeSlot(1)
		tf.assertBlockMetadata([]*BlockRetainerAction{
			{
				"A",
				eventAccepted,
				api.BlockStateAccepted,
			},
		})
	}
	{
		tf.assertBlockMetadata([]*BlockRetainerAction{
			{
				"A",
				eventDropped,
				api.BlockStateAccepted,
			},
		})

	}
	{
		tf.commitSlot(10)
		tf.finalizeSlot(5)

		tf.assertBlockMetadata([]*BlockRetainerAction{
			{
				"A",
				none,
				api.BlockStateFinalized,
			},
		})
	}

	{
		tf.commitSlot(11)
		tf.finalizeSlot(6)

		tf.assertError(&BlockRetainerAction{
			"A",
			eventDropped,
			api.BlockStateFinalized,
		})
	}
}

// TestBlockRetainer_Reset makes sure that the BlockRetainer cleares all cache on Reset.
func TestBlockRetainer_Reset(t *testing.T) {
	tf := NewTestFramework(t)

	{
		tf.commitSlot(2)
		tf.finalizeSlot(0)
		tf.initiateRetainerBlockFlow(5, []string{"A", "B"})
		tf.initiateRetainerBlockFlow(6, []string{"C", "D"})

	}
	{
		tf.commitSlot(4)
		tf.finalizeSlot(1)
		tf.assertBlockMetadata([]*BlockRetainerAction{
			{
				"A",
				eventConfirmed,
				api.BlockStateConfirmed,
			},
			{
				"B",
				eventAccepted,
				api.BlockStateAccepted,
			},
			{
				"C",
				eventAccepted,
				api.BlockStateAccepted,
			},
			{
				"D",
				eventAccepted,
				api.BlockStateAccepted,
			},
		})
	}
	{
		tf.commitSlot(5)
		tf.finalizeSlot(2)
		tf.assertBlockMetadata([]*BlockRetainerAction{
			{
				"A", // is committed
				none,
				api.BlockStateConfirmed,
			},
			{
				"B", // is committed
				eventConfirmed,
				api.BlockStateConfirmed,
			},
			{
				"C",
				eventConfirmed,
				api.BlockStateConfirmed,
			},
			{
				"D",
				eventAccepted,
				api.BlockStateAccepted,
			},
		})
	}
	{
		tf.Instance.Reset()
		tf.assertBlockMetadata([]*BlockRetainerAction{
			{
				"A",
				none,
				api.BlockStateConfirmed,
			},
			{
				"B",
				none,
				api.BlockStateConfirmed,
			},
			{
				"C",
				none, // cache cleared
				api.BlockStateUnknown,
			},
			{
				"D",
				none, // cache cleared
				api.BlockStateUnknown,
			},
		})
	}
}
