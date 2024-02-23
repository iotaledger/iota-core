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
				api.BlockStatePending, // we do not expose the accepted state
			},
			{
				"A2",
				eventAccepted,
				api.BlockStatePending,
			},
			{
				"B",
				eventAccepted,
				api.BlockStatePending,
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
				api.BlockStatePending,
			},
			{
				"C",
				none,
				api.BlockStateDropped,
			},
			{
				"D",
				eventAccepted,
				api.BlockStatePending,
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
				api.BlockStatePending,
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
