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
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func TestBooker(t *testing.T) {
	genesisSeed := tpkg.RandEd25519Seed()
	ts := testsuite.NewTestSuite(t, testsuite.WithSnapshotOptions(
		snapshotcreator.WithGenesisSeed(genesisSeed[:]),
	))
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1", 1)
	ts.Run(map[string][]options.Option[protocol.Protocol]{
		"node1": {
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(slotnotarization.WithMinCommittableSlotAge(1)),
			),
		},
	})
	time.Sleep(time.Second)

	transactionFramework := testsuite.NewTransactionFramework(node1.Protocol, genesisSeed[:])
	tx1, err := transactionFramework.CreateTransaction("Tx1", 1, "Genesis")
	require.NoError(t, err)

	tx2, err := transactionFramework.CreateTransaction("Tx2", 1, "Tx1:0")

	blocksBooked := 0
	node1.Protocol.MainEngineInstance().Events.Booker.BlockBooked.Hook(func(_ *blocks.Block) {
		blocksBooked++
	})

	node1.IssueBlock("block1", blockissuer.WithPayload(tx2))
	ts.Wait(node1)

	require.Equal(t, 0, blocksBooked)

	node1.IssueBlock("block2", blockissuer.WithPayload(tx1))
	ts.Wait(node1)

	require.Equal(t, 2, blocksBooked)
}
