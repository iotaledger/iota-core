package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/blockissuer"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	"github.com/iotaledger/iota-core/pkg/testsuite"
)

func TestBooker(t *testing.T) {
	ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1", 1)
	ts.Run(map[string][]options.Option[protocol.Protocol]{
		"node1": {
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(slotnotarization.WithMinCommittableSlotAge(1)),
			),
		},
	})

	node1.HookLogging()

	time.Sleep(time.Second)

	tx1 := ts.CreateTransaction("Tx1", 1, "Genesis")

	tx2 := ts.CreateTransaction("Tx2", 1, "Tx1:0")

	blocksBooked := 0
	node1.Protocol.MainEngineInstance().Events.Booker.BlockBooked.Hook(func(_ *blocks.Block) {
		blocksBooked++
	})

	ts.RegisterBlock("block1", node1.IssueBlock("block1", blockissuer.WithPayload(tx2)))
	ts.Wait(node1)

	ts.AssertTransactionsExist([]string{"Tx2"}, true, node1)
	ts.AssertTransactionsExist([]string{"Tx1"}, false, node1)

	ts.AssertTransactionsInCacheBooked([]string{"Tx2"}, false, node1)
	require.Equal(t, 0, blocksBooked)

	ts.RegisterBlock("block2", node1.IssueBlock("block2", blockissuer.WithPayload(tx1)))
	ts.Wait(node1)

	ts.AssertTransactionsExist([]string{"Tx1", "Tx2"}, true, node1)
	ts.AssertTransactionsInCacheBooked([]string{"Tx1", "Tx2"}, true, node1)
	require.Equal(t, 2, blocksBooked)
	ts.AssertBlocksInCacheConflicts(map[string][]string{
		"block1": {"Tx2"},
		"block2": {"Tx1"},
	}, node1)

	ts.AssertTransactionInCacheConflicts(map[string][]string{
		"Tx2": {"Tx2"},
		"Tx1": {"Tx1"},
	}, node1)
}
