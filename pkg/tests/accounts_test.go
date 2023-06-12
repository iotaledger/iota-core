package tests

import (
	"testing"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/blockfactory"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/attestation/slotattestation"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/storage"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/utils"
)

func Test_TransitionAccount(t *testing.T) {
	ts := testsuite.NewTestSuite(t, testsuite.WithAccounts(snapshotcreator.AccountDetails{
		Alias:  "A1",
		Amount: 2,
		Key:    utils.RandPubKey().ToEd25519(),
	}))
	// ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1", 50)
	ts.Run(map[string][]options.Option[protocol.Protocol]{
		"node1": {
			protocol.WithStorageOptions(
				storage.WithPrunableManagerOptions(prunable.WithGranularity(1)),
				storage.WithPruningDelay(3),
			),
			protocol.WithNotarizationProvider(
				slotnotarization.NewProvider(1),
			),
			protocol.WithAttestationProvider(
				slotattestation.NewProvider(3),
			),
		},
	})

	ts.HookLogging()
	ts.Wait()

	account1 := ts.AccountOutput("A1")

	// Extract keys and add key for account
	keys := account1.Output().FeatureSet().BlockIssuer().BlockIssuerKeys
	newKey := utils.RandPubKey()
	keys = append(keys, newKey[:])

	account2 := ts.CreateOrTransitionAccount("A1", 2, keys...)

	consumedInputs, outputs, wallets := ts.TransactionFramework.PrepareTransaction(1, "Genesis")

	consumedInputs = append(consumedInputs, account1)
	outputs = append(outputs, account2.Output())

	tx1 := ts.CreateTransactionWithInputsAndOutputs(consumedInputs, outputs, wallets)

	ts.TransactionFramework.RegisterTransaction("TX1", tx1)

	ts.IssueBlock("block1", node1, blockfactory.WithPayload(tx1))

	ts.AssertTransactionsExist(ts.TransactionFramework.Transactions("TX1"), true, node1)
}
