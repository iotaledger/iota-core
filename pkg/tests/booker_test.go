package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/blockissuer"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func TestBooker(t *testing.T) {
	genesisSeed := tpkg.RandEd25519Seed()
	ts := testsuite.NewTestSuite(t, testsuite.WithSnapshotOptions(
		snapshotcreator.WithGenesisSeed(genesisSeed[:]),
	))
	defer ts.Shutdown()

	walletFrom := mock.NewHDWallet("genesis", genesisSeed[:], 0)
	walletTo := mock.NewHDWallet("genesis", genesisSeed[:], 2)

	node1 := ts.AddValidatorNode("node1", 1)
	ts.Run(map[string][]options.Option[protocol.Protocol]{
		"node1": {
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(1),
			),
		},
	})
	time.Sleep(time.Second)

	output, err := node1.Protocol.MainEngineInstance().Ledger.Output(&iotago.UTXOInput{TransactionID: iotago.TransactionID{}, TransactionOutputIndex: 0})
	require.NoError(t, err)

	transaction, err := builder.NewTransactionBuilder(ts.Node("node1").Protocol.MainEngineInstance().Storage.Settings().ProtocolParameters().NetworkID()).
		AddInput(&builder.TxInput{UnlockTarget: output.Output().UnlockConditionSet().Address().Address, InputID: output.OutputID(), Input: output.Output()}).
		AddOutput(&iotago.BasicOutput{
			Amount: 10000000,
			Conditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: walletTo.Address()},
			},
		}).
		Build(node1.Protocol.MainEngineInstance().Storage.Settings().ProtocolParameters(), iotago.NewInMemoryAddressSigner(iotago.NewAddressKeysForEd25519Address(walletFrom.Address(), lo.Return1(walletFrom.KeyPair()))))
	require.NoError(t, err)

	node1.IssueBlock("block1", blockissuer.WithPayload(transaction))

	ts.Wait(node1)

	newOutput, err := node1.Protocol.MainEngineInstance().Ledger.Output(&iotago.UTXOInput{TransactionID: lo.PanicOnErr(transaction.ID()), TransactionOutputIndex: 0})
	require.NoError(t, err)

	genesisOutput, err := node1.Protocol.MainEngineInstance().Ledger.Output(&iotago.UTXOInput{TransactionID: iotago.TransactionID{}, TransactionOutputIndex: 0})
	require.NoError(t, err)

	require.EqualValues(t, 0, genesisOutput.SlotIndexBooked())
	require.EqualValues(t, 101, newOutput.SlotIndexBooked())
}
