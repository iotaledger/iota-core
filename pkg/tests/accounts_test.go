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
		Address:   nil,                               // nil address will be replaced with the address generated from genesis seed
		Amount:    testsuite.MinIssuerAccountDeposit, // min amount to cover the rent. if it's too little then the snapshot creation will fail
		Mana:      0,
		IssuerKey: oldKey,
	}), testsuite.WithGenesisTimestampOffset(100*10))
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")

	ts.Run(map[string][]options.Option[protocol.Protocol]{})
	ts.HookLogging()

	genesisAccount := ts.AccountOutput("Genesis:1")
	genesisAccountOutput := genesisAccount.Output().(*iotago.AccountOutput)

	ts.AssertAccountData(&accounts.AccountData{
		ID:       genesisAccountOutput.AccountID,
		Credits:  accounts.NewBlockIssuanceCredits(int64(testsuite.MinIssuerAccountDeposit), 0),
		OutputID: genesisAccount.OutputID(),
		PubKeys:  advancedset.New(ed25519.PublicKey(oldKey)),
	}, node1)

	// MODIFY EXISTING GENESIS ACCOUNT AND PREPARE SOME BASIC OUTPUTS
	{
		newKey := utils.RandPubKey()
		accountInput, accountOutputs, accountWallets := ts.TransactionFramework.TransitionAccount("Genesis:1", testsuite.AddBlockIssuerKey(newKey[:]), testsuite.WithBlockIssuerExpirySlot(1))
		consumedInputs, equalOutputs, equalWallets := ts.TransactionFramework.CreateBasicOutputsEqually(5, "Genesis:0")

		tx1 := lo.PanicOnErr(ts.TransactionFramework.CreateTransactionWithOptions("TX1", append(accountWallets, equalWallets...),
			testsuite.WithAccountInput(accountInput, true),
			testsuite.WithInputs(consumedInputs),
			testsuite.WithOutputs(append(accountOutputs, equalOutputs...)),
			testsuite.WithAllotments(iotago.Allotments{&iotago.Allotment{
				AccountID: genesisAccountOutput.AccountID,
				Value:     0,
			}}),
		))

		ts.IssueBlockAtSlotWithOptions("block1", 1, iotago.NewEmptyCommitment(), node1, blockfactory.WithPayload(tx1))
		ts.IssueBlockAtSlot("block2", 10, iotago.NewEmptyCommitment(), node1, ts.BlockIDs("block1")...)
		ts.IssueBlockAtSlot("block3", 10, iotago.NewEmptyCommitment(), node1, ts.BlockIDs("block2")...)
		ts.AssertLatestCommitmentSlotIndex(3, node1)

		ts.AssertAccountDiff(genesisAccountOutput.AccountID, 1, &prunable.AccountDiff{
			Change:              0,
			PreviousUpdatedTime: 0,
			NewOutputID:         iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(ts.TransactionFramework.Transaction("TX1").ID()), 0),
			PreviousOutputID:    genesisAccount.OutputID(),
			PubKeysRemoved:      []ed25519.PublicKey{},
			PubKeysAdded:        []ed25519.PublicKey{newKey},
		}, false, node1)

		ts.AssertAccountData(&accounts.AccountData{
			ID:       genesisAccountOutput.AccountID,
			Credits:  accounts.NewBlockIssuanceCredits(int64(testsuite.MinIssuerAccountDeposit), 1),
			OutputID: iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(ts.TransactionFramework.Transaction("TX1").ID()), 0),
			PubKeys:  advancedset.New(ed25519.PublicKey(oldKey), newKey),
		}, node1)
	}

	// DESTROY ACCOUNT A1, CREATE NEW ACCOUNT FROM BASIC UTXO
	{
		newAccountBlockIssuerKey := utils.RandPubKey()
		inputForNewAccount, newAccountOutputs, newAccountWallets := ts.TransactionFramework.CreateAccountFromInput("TX1:1",
			testsuite.WithConditions(iotago.AccountOutputUnlockConditions{
				&iotago.StateControllerAddressUnlockCondition{Address: ts.TransactionFramework.DefaultAddress()},
				&iotago.GovernorAddressUnlockCondition{Address: ts.TransactionFramework.DefaultAddress()},
			}),
			testsuite.WithBlockIssuerFeature(&iotago.BlockIssuerFeature{
				BlockIssuerKeys: iotago.BlockIssuerKeys{newAccountBlockIssuerKey[:]},
			}),
		)

		destroyedAccountInput, destroyAccountOutputs, destroyWallets := ts.TransactionFramework.DestroyAccount("TX1:0")

		tx2 := lo.PanicOnErr(ts.TransactionFramework.CreateTransactionWithOptions("TX2", append(newAccountWallets, destroyWallets...),
			testsuite.WithContextInputs(iotago.TxEssenceContextInputs{
				&iotago.BICInput{
					AccountID:    genesisAccountOutput.AccountID,
					CommitmentID: node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
				},
			}),
			testsuite.WithInputs(inputForNewAccount),
			testsuite.WithAccountInput(destroyedAccountInput, true),
			testsuite.WithOutputs(append(newAccountOutputs, destroyAccountOutputs...)),
			testsuite.WithCreationTime(11),
		))

		ts.IssueBlockAtSlotWithOptions("block4", 11, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node1, blockfactory.WithStrongParents(ts.BlockID("block3")), blockfactory.WithPayload(tx2))
		ts.IssueBlockAtSlot("block4.1", 13, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node1, ts.BlockIDs("block4")...)
		ts.IssueBlockAtSlot("block4.2", 13, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node1, ts.BlockIDs("block4.1")...)
		ts.AssertLatestCommitmentSlotIndex(6, node1)

		ts.IssueBlockAtSlot("block4.3", 16, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node1, ts.BlockIDs("block4.2")...)
		ts.IssueBlockAtSlot("block4.4", 16, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node1, ts.BlockIDs("block4.3")...)
		ts.AssertLatestCommitmentSlotIndex(9, node1)

		ts.IssueBlockAtSlot("block4.5", 19, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node1, ts.BlockIDs("block4.4")...)
		ts.IssueBlockAtSlot("block4.6", 19, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node1, ts.BlockIDs("block4.5")...)
		ts.AssertLatestCommitmentSlotIndex(12, node1)

		ts.IssueBlockAtSlot("block4.7", 20, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node1, ts.BlockIDs("block4.6")...)
		ts.IssueBlockAtSlot("block4.8", 20, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node1, ts.BlockIDs("block4.7")...)

		ts.AssertLatestCommitmentSlotIndex(13, node1)

		ts.AssertAccountDiff(genesisAccountOutput.AccountID, 13, &prunable.AccountDiff{}, true, node1)

		newAccount := ts.AccountOutput("TX2:0")
		newAccountOutput := newAccount.Output().(*iotago.AccountOutput)

		ts.AssertAccountDiff(newAccountOutput.AccountID, 13, &prunable.AccountDiff{
			Change:              0,
			PreviousUpdatedTime: 0,
			NewOutputID:         newAccount.OutputID(),
			PreviousOutputID:    iotago.EmptyOutputID,
			PubKeysRemoved:      []ed25519.PublicKey{},
			PubKeysAdded:        []ed25519.PublicKey{newAccountBlockIssuerKey},
		}, false, node1)

		ts.AssertAccountData(&accounts.AccountData{
			ID:       newAccountOutput.AccountID,
			Credits:  accounts.NewBlockIssuanceCredits(0, 11),
			OutputID: newAccount.OutputID(),
			PubKeys:  advancedset.New(newAccountBlockIssuerKey),
		}, node1)
	}
	ts.Wait(node1)
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
