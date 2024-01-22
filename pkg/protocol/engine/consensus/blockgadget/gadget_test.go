package blockgadget_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

func TestBlockGadget(t *testing.T) {
	tf := NewTestFramework(t)

	expectedPreAcceptanceOrder := []string{"A.1", "A.2", "A.3", "B.1"}
	expectedAcceptanceOrder := []string{"A.1", "A.2"}
	expectedPreConfirmationOrder := []string{"A.1", "A.2", "A.3", "B.1"}
	expectedConfirmationOrder := []string{"A.1", "A.2"}

	checkOrder := func(expectedOrder *[]string, name string) func(*blocks.Block) {
		return func(block *blocks.Block) {
			if block.ID().Alias() == (*expectedOrder)[0] {
				*expectedOrder = (*expectedOrder)[1:]
				return
			}

			tf.T.Fatalf("unexpected block %s: expected %s, got %s", name, (*expectedOrder)[0], block.ID().Alias())
		}
	}

	tf.Events.BlockPreAccepted.Hook(checkOrder(&expectedPreAcceptanceOrder, "pre-acceptance"))
	tf.Events.BlockAccepted.Hook(checkOrder(&expectedAcceptanceOrder, "acceptance"))
	tf.Events.BlockPreConfirmed.Hook(checkOrder(&expectedPreConfirmationOrder, "pre-confirmation"))
	tf.Events.BlockConfirmed.Hook(checkOrder(&expectedConfirmationOrder, "confirmation"))

	tf.SeatManager.AddRandomAccounts("A", "B")

	tf.SeatManager.SetOnline("A")
	tf.SeatManager.SetOnline("B")

	tf.CreateBlockAndTrackWitnessWeight("A.1", "A", "Genesis")
	tf.CreateBlockAndTrackWitnessWeight("A.2", "A", "A.1")

	tf.AssertBlocksPreAccepted(tf.Blocks("A.1", "A.2"), false)
	tf.SeatManager.SetOffline("B")

	tf.CreateBlockAndTrackWitnessWeight("A.3", "A", "A.2")
	tf.AssertBlocksPreAccepted(tf.Blocks("A.1", "A.2", "A.3"), true)
	tf.AssertBlocksAccepted(tf.Blocks("A.1", "A.2"), true)

	tf.SeatManager.SetOnline("B")
	tf.CreateBlockAndTrackWitnessWeight("B.1", "B", "A.3")

	tf.AssertBlocksPreAccepted(tf.Blocks("B.1"), false)
	tf.AssertBlocksAccepted(tf.Blocks("A.1", "A.2"), true)
	tf.AssertBlocksPreConfirmed(tf.Blocks("A.1", "A.2", "A.3"), true)

	tf.CreateBlockAndTrackWitnessWeight("A.4", "A", "B.1")
	tf.AssertBlocksPreAccepted(tf.Blocks("A.4"), false)
	tf.AssertBlocksPreAccepted(tf.Blocks("B.1"), true)
	tf.AssertBlocksPreConfirmed(tf.Blocks("B.1"), true)
	tf.AssertBlocksConfirmed(tf.Blocks("A.1", "A.2"), true)

	require.Empty(t, expectedPreAcceptanceOrder, "not all blocks were pre-accepted")
	require.Empty(t, expectedAcceptanceOrder, "not all blocks were accepted")
	require.Empty(t, expectedPreConfirmationOrder, "not all blocks were pre-confirmed")
	require.Empty(t, expectedConfirmationOrder, "not all blocks were confirmed")
}
