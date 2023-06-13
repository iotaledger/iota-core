package tests

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/blockfactory"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

func Test_TransitionAccount(t *testing.T) {
	ts := testsuite.NewTestSuite(t, testsuite.WithAccounts(snapshotcreator.AccountDetails{
		Alias:  "A1",
		Amount: 2,
		Key:    utils.RandPubKey().ToEd25519(),
	}))
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1", 50)

	ts.Run(map[string][]options.Option[protocol.Protocol]{})
	ts.HookLogging()
	ts.Wait()

	account1 := ts.AccountOutput("A1")

	// Extract keys and add key for account
	keys := account1.Output().FeatureSet().BlockIssuer().BlockIssuerKeys
	newKey := utils.RandPubKey()
	keys = append(keys, newKey[:])

	account2 := ts.TransitionAccount("A1", 2, keys...)

	consumedInputs, outputs, wallets := ts.TransactionFramework.PrepareTransaction(1, "Genesis")

	consumedInputs = append(consumedInputs, account1)
	outputs = append(outputs, account2)

	tx1 := ts.CreateTransactionWithInputsAndOutputs(consumedInputs, outputs, wallets)

	ts.TransactionFramework.RegisterTransaction("TX1", tx1)

	ts.IssueBlockAtSlotWithOptions("block1", 1, iotago.NewEmptyCommitment(), node1, blockfactory.WithPayload(tx1))
	ts.IssueBlockAtSlot("block2", 10, iotago.NewEmptyCommitment(), node1, ts.BlockIDs("block1")...)
	ts.IssueBlockAtSlot("block3", 10, iotago.NewEmptyCommitment(), node1, ts.BlockIDs("block2")...)

	account1Output := account1.Output().(*iotago.AccountOutput)
	// account2Output := account2.Output().(*iotago.AccountOutput)

	ts.Wait(ts.Node("node1"))

	require.True(t, lo.PanicOnErr(ts.Node("node1").Protocol.MainEngineInstance().Storage.AccountDiffs(1).Has(account1Output.AccountID)))
	accountDiff, destroyed, err := ts.Node("node1").Protocol.MainEngineInstance().Storage.AccountDiffs(1).Load(account1Output.AccountID)
	require.NoError(t, err)
	require.False(t, destroyed)

	require.Equal(t, accountDiff.Change, int64(0))
	require.Equal(t, accountDiff.PreviousUpdatedTime, iotago.SlotIndex(0))
	require.Equal(t, accountDiff.PreviousOutputID, account1.OutputID())
	require.Equal(t, accountDiff.NewOutputID, iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(ts.TransactionFramework.Transaction("TX1").ID()), 1))
	require.Equal(t, len(accountDiff.PubKeysRemoved), 0)
	require.Equal(t, len(accountDiff.PubKeysAdded), 1)

	// ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("TX1"), true, node1)
}
