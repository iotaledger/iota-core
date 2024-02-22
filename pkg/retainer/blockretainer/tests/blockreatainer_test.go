package tests

import (
	"testing"

	"github.com/iotaledger/iota.go/v4/api"
)

// TestBlockRetainer_RetainBlock tests the BlockRetainer.
// A: BlockBooked -> BlockAccepted -> BlockConfirmed -> BlockFinalized
// A2: BlockBooked -> BlockAccepted -> BlockConfirmed -> BlockFinalized (issued one slot later)
// B: BlockBooked -> BlockAccepted -> BlockFinalized
// C: BlockBooked -> BlockDropped
// D: BlockBooked -> BlockDropped -> BlockAccepted
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
		tf.initiateRetainerBlockFlow(6, []string{"A2", "C", "D"})
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
				none,
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
		tf.commitSlot(6)
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
