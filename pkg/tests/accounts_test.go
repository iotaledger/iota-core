package tests

import (
	"testing"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/blockfactory"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

func Test_TransitionAccount(t *testing.T) {
	ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1", 1)
	ts.Run(map[string][]options.Option[protocol.Protocol]{})
	node1.HookLogging()

	account1 := ts.CreateOrTransitionAccount("A1", 2)

	// Extract keys and add key for account
	keys := account1.Output().FeatureSet().BlockIssuer().BlockIssuerKeys
	newKey := utils.RandPubKey()
	keys = append(keys, newKey[:])

	account2 := ts.CreateOrTransitionAccount("A1", 2, keys...)

	consumedInputs, outputs, wallets := ts.TransactionFramework.PrepareTransaction(1, "genesis")

	consumedInputs = append(consumedInputs, account1)
	outputs = append(outputs, account2.Output())

	tx1 := ts.CreateTransactionWithInputsAndOutputs(consumedInputs, outputs, wallets)

	ts.TransactionFramework.RegisterTransaction("TX1", tx1)

	tx2 := ts.CreateTransaction("Tx2", 1, "Tx1:0")

	ts.IssueBlock("block1", node1, blockfactory.WithPayload(tx2))

	ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("Tx2"), true, node1)
	ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("Tx1"), false, node1)

	ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.Transactions("Tx2"), false, node1)
	// make sure that the block is not booked

	ts.IssueBlock("block2", node1, blockfactory.WithPayload(tx1))

	ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("Tx1", "Tx2"), true, node1)
	ts.AssertTransactionsInCacheBooked(ts.TransactionFramework.Transactions("Tx1", "Tx2"), true, node1)
	ts.AssertBlocksInCacheConflicts(map[*blocks.Block][]string{
		ts.Block("block1"): {"Tx2"},
		ts.Block("block2"): {"Tx1"},
	}, node1)

	ts.AssertTransactionInCacheConflicts(map[*iotago.Transaction][]string{
		ts.TransactionFramework.Transaction("Tx2"): {"Tx2"},
		ts.TransactionFramework.Transaction("Tx1"): {"Tx1"},
	}, node1)
}
