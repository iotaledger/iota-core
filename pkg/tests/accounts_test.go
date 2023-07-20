package tests

import (
	"math"
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

// TODO: implement tests for staking and delegation transitions
func Test_TransitionAccount(t *testing.T) {

	oldGenesisOutputKey := utils.RandPubKey().ToEd25519()
	ts := testsuite.NewTestSuite(t, testsuite.WithAccounts(snapshotcreator.AccountDetails{
		// Nil address will be replaced with the address generated from genesis seed.
		// A single key may unlock multiple accounts; that's why it can't be used as a source for AccountID derivation.
		Address: nil,
		// Min amount to cover the rent. If it's too little, then the snapshot creation will fail
		Amount: testsuite.MinIssuerAccountDeposit,
		// AccountID is derived from this field, so this must be set uniquely for each account.
		IssuerKey: oldGenesisOutputKey,
		// Expiry Slot is the slot index at which the account expires.
		ExpirySlot: math.MaxUint64,
	}),
		testsuite.WithGenesisTimestampOffset(100*10),
	)
	defer ts.Shutdown()

	minSlotCommittableAge := ts.API.ProtocolParameters().MinCommittableAge()

	node1 := ts.AddValidatorNode("node1")

	ts.Run(map[string][]options.Option[protocol.Protocol]{})
	ts.HookLogging()

	genesisAccount := ts.AccountOutput("Genesis:1")
	genesisAccountOutput := genesisAccount.Output().(*iotago.AccountOutput)

	ts.AssertAccountData(&accounts.AccountData{
		ID: genesisAccountOutput.AccountID,
		// TODO: why do we use the deposit here as credits?
		Credits:    accounts.NewBlockIssuanceCredits(iotago.BlockIssuanceCredits(testsuite.MinIssuerAccountDeposit), 0),
		OutputID:   genesisAccount.OutputID(),
		ExpirySlot: math.MaxUint64,
		PubKeys:    advancedset.New(ed25519.PublicKey(oldGenesisOutputKey)),
	}, node1)

	// MODIFY EXISTING GENESIS ACCOUNT AND PREPARE SOME BASIC OUTPUTS

	newGenesisOutputKey := utils.RandPubKey()
	{
		accountInput, accountOutputs, accountWallets := ts.TransactionFramework.TransitionAccount("Genesis:1", testsuite.AddBlockIssuerKey(newGenesisOutputKey[:]), testsuite.WithBlockIssuerExpirySlot(100))
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

		var slotIndexBlock1 iotago.SlotIndex = 1

		ts.IssueBlockAtSlotWithOptions("block1", slotIndexBlock1, iotago.NewEmptyCommitment(ts.API.ProtocolParameters().Version()), node1, blockfactory.WithPayload(tx1))

		slotIndexChildrenBlock1 := ts.BlockID("block1").Index() + minSlotCommittableAge + 1
		ts.IssueBlockAtSlot("block2", slotIndexChildrenBlock1, iotago.NewEmptyCommitment(ts.API.ProtocolParameters().Version()), node1, ts.BlockIDs("block1")...)
		ts.IssueBlockAtSlot("block3", slotIndexChildrenBlock1, iotago.NewEmptyCommitment(ts.API.ProtocolParameters().Version()), node1, ts.BlockIDs("block2")...)

		ts.AssertLatestCommitmentSlotIndex(slotIndexBlock1, node1)

		ts.AssertAccountDiff(genesisAccountOutput.AccountID, slotIndexBlock1, &prunable.AccountDiff{
			BICChange:           0,
			PreviousUpdatedTime: 0,
			PreviousExpirySlot:  math.MaxUint64,
			NewExpirySlot:       100,
			NewOutputID:         iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(ts.TransactionFramework.Transaction("TX1").ID(ts.API)), 0),
			PreviousOutputID:    genesisAccount.OutputID(),
			PubKeysRemoved:      []ed25519.PublicKey{},
			PubKeysAdded:        []ed25519.PublicKey{newGenesisOutputKey},
		}, false, node1)

		ts.AssertAccountData(&accounts.AccountData{
			ID: genesisAccountOutput.AccountID,
			// TODO: why do we use the deposit here as credits?
			Credits:  accounts.NewBlockIssuanceCredits(iotago.BlockIssuanceCredits(testsuite.MinIssuerAccountDeposit), 0),
			OutputID: iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(ts.TransactionFramework.Transaction("TX1").ID(ts.API)), 0),
			PubKeys:  advancedset.New(ed25519.PublicKey(oldGenesisOutputKey), newGenesisOutputKey),
		}, node1)
	}

	// DESTROY GENESIS ACCOUNT, CREATE NEW ACCOUNT WITH BLOCK ISSUER AND STAKING FEATURES FROM BASIC UTXO
	newAccountBlockIssuerKey := utils.RandPubKey()
	{
		inputForNewAccount, newAccountOutputs, newAccountWallets := ts.TransactionFramework.CreateAccountFromInput("TX1:1",
			testsuite.WithAccountConditions(iotago.AccountOutputUnlockConditions{
				&iotago.StateControllerAddressUnlockCondition{Address: ts.TransactionFramework.DefaultAddress()},
				&iotago.GovernorAddressUnlockCondition{Address: ts.TransactionFramework.DefaultAddress()},
			}),
			testsuite.WithBlockIssuerFeature(&iotago.BlockIssuerFeature{
				BlockIssuerKeys: iotago.BlockIssuerKeys{newAccountBlockIssuerKey[:]},
				ExpirySlot:      10,
			}),
			testsuite.WithStakingFeature(&iotago.StakingFeature{
				StakedAmount: 10000,
				FixedCost:    421,
				StartEpoch:   1,
				EndEpoch:     10,
			}),
		)

		destroyedAccountInput, destroyAccountOutputs, destroyWallets := ts.TransactionFramework.DestroyAccount("TX1:0")

		var slotIndexBlock4 iotago.SlotIndex = ts.BlockID("block3").Index() + 2

		tx2 := lo.PanicOnErr(ts.TransactionFramework.CreateTransactionWithOptions("TX2", append(newAccountWallets, destroyWallets...),
			testsuite.WithContextInputs(iotago.TxEssenceContextInputs{
				&iotago.BlockIssuanceCreditInput{
					AccountID: genesisAccountOutput.AccountID,
				},
				&iotago.CommitmentInput{
					CommitmentID: node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
				},
			}),
			testsuite.WithInputs(inputForNewAccount),
			testsuite.WithAccountInput(destroyedAccountInput, true),
			testsuite.WithOutputs(append(newAccountOutputs, destroyAccountOutputs...)),
			testsuite.WithCreationTime(slotIndexBlock4),
		))

		ts.IssueBlockAtSlotWithOptions("block4", slotIndexBlock4, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node1, blockfactory.WithStrongParents(ts.BlockID("block3")), blockfactory.WithPayload(tx2))

		slotIndexChildrenBlock4 := ts.BlockID("block4").Index() + minSlotCommittableAge + 1
		ts.IssueBlockAtSlot("block5", slotIndexChildrenBlock4, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node1, ts.BlockIDs("block4")...)
		ts.IssueBlockAtSlot("block6", slotIndexChildrenBlock4, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node1, ts.BlockIDs("block5")...)

		ts.AssertLatestCommitmentSlotIndex(slotIndexBlock4, node1)

		// assert diff of a destroyed account, to make sure we can correctly restore it
		ts.AssertAccountDiff(genesisAccountOutput.AccountID, slotIndexBlock4, &prunable.AccountDiff{
			BICChange:             -iotago.BlockIssuanceCredits(testsuite.MinIssuerAccountDeposit),
			PreviousUpdatedTime:   0,
			NewOutputID:           iotago.EmptyOutputID,
			PreviousOutputID:      iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(ts.TransactionFramework.Transaction("TX1").ID(ts.API)), 0),
			PubKeysAdded:          []ed25519.PublicKey{},
			PubKeysRemoved:        []ed25519.PublicKey{ed25519.PublicKey(oldGenesisOutputKey), newGenesisOutputKey},
			ValidatorStakeChange:  0,
			StakeEndEpochChange:   0,
			FixedCostChange:       0,
			DelegationStakeChange: 0,
		}, true, node1)

		newAccount := ts.AccountOutput("TX2:0")
		newAccountOutput := newAccount.Output().(*iotago.AccountOutput)

		ts.AssertAccountDiff(newAccountOutput.AccountID, slotIndexBlock4, &prunable.AccountDiff{
			BICChange:             0,
			PreviousUpdatedTime:   0,
			NewOutputID:           newAccount.OutputID(),
			PreviousOutputID:      iotago.EmptyOutputID,
			PubKeysAdded:          []ed25519.PublicKey{newAccountBlockIssuerKey},
			PubKeysRemoved:        []ed25519.PublicKey{},
			ValidatorStakeChange:  10000,
			StakeEndEpochChange:   10,
			FixedCostChange:       421,
			DelegationStakeChange: 0,
		}, false, node1)

		ts.AssertAccountData(&accounts.AccountData{
			ID:              newAccountOutput.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(0, 10),
			OutputID:        newAccount.OutputID(),
			PubKeys:         advancedset.New(newAccountBlockIssuerKey),
			StakeEndEpoch:   10,
			FixedCost:       421,
			DelegationStake: 0,
			ValidatorStake:  10000,
		}, node1)
	}

	// create a delegation output delegating to the newly created account
	{
		newAccount := ts.AccountOutput("TX2:0")
		newAccountOutput := newAccount.Output().(*iotago.AccountOutput)

		inputForNewDelegation, newDelegationOutputs, newDelegationWallets := ts.TransactionFramework.CreateDelegationFromInput("TX1:2",
			testsuite.WithDelegatedValidatorID(newAccountOutput.AccountID),
			testsuite.WithDelegationStartEpoch(2),
		)

		var slotIndexBlock7 iotago.SlotIndex = ts.BlockID("block6").Index() + 3

		tx2 := lo.PanicOnErr(ts.TransactionFramework.CreateTransactionWithOptions("TX2", newDelegationWallets,
			testsuite.WithInputs(inputForNewDelegation),
			testsuite.WithOutputs(newDelegationOutputs),
			testsuite.WithCreationTime(slotIndexBlock7),
		))

		ts.IssueBlockAtSlotWithOptions("block7", slotIndexBlock7, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node1, blockfactory.WithStrongParents(ts.BlockID("block6")), blockfactory.WithPayload(tx2))

		slotIndexChildrenBlock3 := ts.BlockID("block7").Index() + minSlotCommittableAge + 1
		ts.IssueBlockAtSlot("block8", slotIndexChildrenBlock3, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node1, ts.BlockIDs("block7")...)
		ts.IssueBlockAtSlot("block9", slotIndexChildrenBlock3, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node1, ts.BlockIDs("block8")...)

		ts.AssertLatestCommitmentSlotIndex(slotIndexBlock7, node1)

		ts.AssertAccountDiff(newAccountOutput.AccountID, slotIndexBlock7, &prunable.AccountDiff{
			BICChange:             0,
			PreviousUpdatedTime:   0,
			NewOutputID:           iotago.EmptyOutputID,
			PreviousOutputID:      iotago.EmptyOutputID,
			PubKeysAdded:          []ed25519.PublicKey{},
			PubKeysRemoved:        []ed25519.PublicKey{},
			ValidatorStakeChange:  0,
			StakeEndEpochChange:   0,
			FixedCostChange:       0,
			DelegationStakeChange: 1965580,
		}, false, node1)

		ts.AssertAccountData(&accounts.AccountData{
			ID:              newAccountOutput.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(0, 10),
			OutputID:        newAccount.OutputID(),
			PubKeys:         advancedset.New(newAccountBlockIssuerKey),
			StakeEndEpoch:   10,
			FixedCost:       421,
			DelegationStake: 1966240,
			ValidatorStake:  10000,
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
