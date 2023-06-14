package tests

import (
	"testing"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/blockfactory"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

func Test_TransitionAccount(t *testing.T) {
	oldKey := utils.RandPubKey().ToEd25519()
	ts := testsuite.NewTestSuite(t, testsuite.WithAccounts(snapshotcreator.AccountDetails{
		Alias:  "A1",
		Amount: 2,
		Key:    oldKey,
	}))
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1", 50)

	ts.Run(map[string][]options.Option[protocol.Protocol]{})
	ts.HookLogging()

	account1 := ts.AccountOutput("A1")
	account1Output := account1.Output().(*iotago.AccountOutput)

	ts.AssertAccountData(&accounts.AccountData{
		ID:       account1Output.AccountID,
		Credits:  accounts.NewBlockIssuanceCredits(2, 0),
		OutputID: account1.OutputID(),
		PubKeys:  advancedset.New(ed25519.PublicKey(oldKey)),
	}, ts.Node("node1"))

	// Extract keys and add key for account
	keys := account1.Output().FeatureSet().BlockIssuer().BlockIssuerKeys
	newKey := utils.RandPubKey()
	keys = append(keys, newKey[:])

	account2 := ts.TransitionAccount("A1", 2, keys...)

	consumedInputs, outputs, wallets := ts.TransactionFramework.PrepareTransaction(1, "Genesis")

	consumedInputs = append(consumedInputs, account1)
	outputs = append(outputs, account2)

	tx1 := ts.CreateTransactionWithInputsAndOutputs(consumedInputs, outputs, wallets)
	// tx1.Essence.Allotments = append(tx1.Essence.Allotments, iotago.Allotment{
	// 	AccountID: account1Output.AccountID,
	// 	Value:     100,
	// })

	ts.TransactionFramework.RegisterTransaction("TX1", tx1)

	ts.IssueBlockAtSlotWithOptions("block1", 1, iotago.NewEmptyCommitment(), node1, blockfactory.WithPayload(tx1))
	ts.IssueBlockAtSlot("block2", 10, iotago.NewEmptyCommitment(), node1, ts.BlockIDs("block1")...)
	ts.IssueBlockAtSlot("block3", 10, iotago.NewEmptyCommitment(), node1, ts.BlockIDs("block2")...)

	// account2Output := account2.Output().(*iotago.AccountOutput)

	ts.AssertAccountDiff(account1Output.AccountID, 1, &prunable.AccountDiff{
		Change:              0,
		PreviousUpdatedTime: 0,
		NewOutputID:         iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(ts.TransactionFramework.Transaction("TX1").ID()), 1),
		PreviousOutputID:    account1.OutputID(),
		PubKeysRemoved:      []ed25519.PublicKey{},
		PubKeysAdded:        []ed25519.PublicKey{newKey},
	}, false, ts.Node("node1"))

	ts.AssertAccountData(&accounts.AccountData{
		ID:       account1Output.AccountID,
		Credits:  accounts.NewBlockIssuanceCredits(2, 1),
		OutputID: iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(ts.TransactionFramework.Transaction("TX1").ID()), 1),
		PubKeys:  advancedset.New(ed25519.PublicKey(oldKey), newKey),
	}, ts.Node("node1"))

	ts.Wait(ts.Node("node1"))
}

/*
For Mana allotment and stored:
1. Collect potential and stored on the input side.
2. Add options to allot amounts to accounts upon TX creation.
3. Add option to store mana on the output side.
4. Optionally add option to split amount on outputs unevenly.

WithAllotments
{
	A1: amount
	A3: amount
}
WithStoredOnOutput
{
	0: amount
	3: amount
}
*/

/*
TX involving Accounts:
1. Add option to add accounts as inputs.
2. Add option to add accounts as outputs.
3. Create account.
4. Destroy accounts.
5. Accounts w/out and w/ BIC.

Testcases:
1. Create account w/out BIC from normal UTXO.
2. Create account w/ BIC from normal UTXO.
3. Transition non-BIC account to BIC account.
4. Transition BIC account to non-BIC account.
5. Transition BIC account to BIC account changing amount/keys/expiry.
6. Destroy account w/out BIC feature.
7. Destroy account w/ BIC feature.
*/
