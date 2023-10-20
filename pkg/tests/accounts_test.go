package tests

import (
	"testing"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TODO: implement tests for staking and delegation transitions that cover edge cases - part of hardening phase.
func Test_TransitionAccount(t *testing.T) {
	oldGenesisOutputKey := utils.RandBlockIssuerKey()
	ts := testsuite.NewTestSuite(t, testsuite.WithAccounts(snapshotcreator.AccountDetails{
		// Nil address will be replaced with the address generated from genesis seed.
		// A single key may unlock multiple accounts; that's why it can't be used as a source for AccountID derivation.
		Address: nil,
		// Set an amount enough to cover the rent and to cover an additional key that is added in the test.
		// If it's too little, then the test will fail.
		Amount: testsuite.MinIssuerAccountAmount * 10,
		Mana:   0,
		// AccountID is derived from this field, so this must be set uniquely for each account.
		IssuerKey: oldGenesisOutputKey,
		// Expiry Slot is the slot index at which the account expires.
		ExpirySlot: 1,
		// BlockIssuanceCredits on this account is custom because it never needs to issue.
		// On Validator nodes it's unlimited (MaxInt64).
		BlockIssuanceCredits: iotago.BlockIssuanceCredits(123),
	}),
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				testsuite.GenesisTimeWithOffsetBySlots(100, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				8,
			),
			iotago.WithLivenessOptions(
				testsuite.DefaultLivenessThresholdLowerBoundInSeconds,
				testsuite.DefaultLivenessThresholdUpperBoundInSeconds,
				testsuite.DefaultMinCommittableAge,
				100,
				testsuite.DefaultEpochNearingThreshold,
			),
		),
	)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	_ = ts.AddNode("node2")
	blockIssuer := ts.AddBasicBlockIssuer("default", iotago.MaxBlockIssuanceCredits/2)

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{})

	genesisAccount := ts.AccountOutput("Genesis:1")
	genesisAccountOutput := genesisAccount.Output().(*iotago.AccountOutput)

	ts.AssertAccountData(&accounts.AccountData{
		ID:              genesisAccountOutput.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.BlockIssuanceCredits(123), 0),
		OutputID:        genesisAccount.OutputID(),
		ExpirySlot:      1,
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(oldGenesisOutputKey),
	}, ts.Nodes()...)

	// MODIFY EXISTING GENESIS ACCOUNT AND PREPARE SOME BASIC OUTPUTS

	newGenesisOutputKey := utils.RandBlockIssuerKey()
	newAccountBlockIssuerKey := utils.RandBlockIssuerKey()
	latestCommitmentID := node1.Protocol.MainEngine.Get().Storage.Settings().LatestCommitment().Commitment().MustID()

	accountInput, accountOutputs, accountWallets := ts.TransactionFramework.TransitionAccount(
		"Genesis:1",
		testsuite.WithAddBlockIssuerKey(newGenesisOutputKey),
		testsuite.WithBlockIssuerExpirySlot(1),
	)
	consumedInputs, equalOutputs, equalWallets := ts.TransactionFramework.CreateBasicOutputsEqually(4, "Genesis:0")

	tx1 := lo.PanicOnErr(ts.TransactionFramework.CreateSignedTransactionWithOptions("TX1", append(accountWallets, equalWallets...),
		testsuite.WithAccountInput(accountInput, true),
		testsuite.WithInputs(consumedInputs),
		testsuite.WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.BlockIssuanceCreditInput{
				AccountID: genesisAccountOutput.AccountID,
			},
			&iotago.CommitmentInput{
				CommitmentID: latestCommitmentID,
			},
		}),
		testsuite.WithOutputs(append(accountOutputs, equalOutputs...)),
		testsuite.WithAllotments(iotago.Allotments{&iotago.Allotment{
			AccountID: genesisAccountOutput.AccountID,
			Value:     0,
		}}),
	))

	var block1Slot iotago.SlotIndex = 1
	genesisCommitment := iotago.NewEmptyCommitment(ts.API.ProtocolParameters().Version())
	genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost

	block1 := ts.IssueBasicBlockAtSlotWithOptions("block1", block1Slot, genesisCommitment, blockIssuer, node1, tx1)

	latestParent := ts.CommitUntilSlot(ts.BlockID("block1").Slot(), block1)

	ts.AssertAccountDiff(genesisAccountOutput.AccountID, block1Slot, &model.AccountDiff{
		BICChange:              0,
		PreviousUpdatedTime:    0,
		PreviousExpirySlot:     1,
		NewExpirySlot:          1,
		NewOutputID:            iotago.OutputIDFromTransactionIDAndIndex(ts.TransactionFramework.TransactionID("TX1"), 0),
		PreviousOutputID:       genesisAccount.OutputID(),
		BlockIssuerKeysRemoved: iotago.NewBlockIssuerKeys(),
		BlockIssuerKeysAdded:   iotago.NewBlockIssuerKeys(newGenesisOutputKey),
	}, false, ts.Nodes()...)

	ts.AssertAccountData(&accounts.AccountData{
		ID:              genesisAccountOutput.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.BlockIssuanceCredits(123), 0),
		OutputID:        iotago.OutputIDFromTransactionIDAndIndex(ts.TransactionFramework.TransactionID("TX1"), 0),
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(oldGenesisOutputKey, newGenesisOutputKey),
		ExpirySlot:      1,
	}, ts.Nodes()...)

	// DESTROY GENESIS ACCOUNT, CREATE NEW ACCOUNT WITH BLOCK ISSUER AND STAKING FEATURES FROM BASIC UTXO

	// commit until the expiry slot of the transitioned genesis account plus one
	latestParent = ts.CommitUntilSlot(accountOutputs[0].FeatureSet().BlockIssuer().ExpirySlot+1, latestParent)
	// set the expiry slof of the transitioned genesis account to the latest committed + MaxCommittableAge
	newAccountExpirySlot := node1.Protocol.MainEngine.Get().Storage.Settings().LatestCommitment().Slot() + ts.API.ProtocolParameters().MaxCommittableAge()
	inputForNewAccount, newAccountOutputs, newAccountWallets := ts.TransactionFramework.CreateAccountFromInput("TX1:1",
		testsuite.WithAccountConditions(iotago.AccountOutputUnlockConditions{
			&iotago.StateControllerAddressUnlockCondition{Address: ts.TransactionFramework.DefaultAddress()},
			&iotago.GovernorAddressUnlockCondition{Address: ts.TransactionFramework.DefaultAddress()},
		}),
		testsuite.WithBlockIssuerFeature(iotago.BlockIssuerKeys{newAccountBlockIssuerKey}, newAccountExpirySlot),
		testsuite.WithStakingFeature(10000, 421, 0, 10),
	)

	destroyedAccountInput, destructionOutputs, destroyWallets := ts.TransactionFramework.DestroyAccount("TX1:0")

	block2Slot := latestParent.ID().Slot()

	tx2 := lo.PanicOnErr(ts.TransactionFramework.CreateSignedTransactionWithOptions("TX2", append(newAccountWallets, destroyWallets...),
		testsuite.WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.BlockIssuanceCreditInput{
				AccountID: genesisAccountOutput.AccountID,
			},
			&iotago.CommitmentInput{
				CommitmentID: node1.Protocol.MainEngine.Get().Storage.Settings().LatestCommitment().Commitment().MustID(),
			},
		}),
		testsuite.WithInputs(inputForNewAccount),
		testsuite.WithAccountInput(destroyedAccountInput, true),
		testsuite.WithOutputs(append(newAccountOutputs, destructionOutputs...)),
		testsuite.WithSlotCreated(block2Slot),
	))

	block2 := ts.IssueBasicBlockAtSlotWithOptions("block2", block2Slot, node1.Protocol.MainEngine.Get().Storage.Settings().LatestCommitment().Commitment(), blockIssuer, node1, tx2, mock.WithStrongParents(latestParent.ID()))

	latestParent = ts.CommitUntilSlot(block2Slot, block2)

	// assert diff of a destroyed account, to make sure we can correctly restore it
	ts.AssertAccountDiff(genesisAccountOutput.AccountID, block2Slot, &model.AccountDiff{
		BICChange:              -iotago.BlockIssuanceCredits(123),
		PreviousUpdatedTime:    0,
		NewExpirySlot:          0,
		PreviousExpirySlot:     1,
		NewOutputID:            iotago.EmptyOutputID,
		PreviousOutputID:       iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(ts.TransactionFramework.Transaction("TX1").ID()), 0),
		BlockIssuerKeysAdded:   iotago.NewBlockIssuerKeys(),
		BlockIssuerKeysRemoved: iotago.NewBlockIssuerKeys(oldGenesisOutputKey, newGenesisOutputKey),
		ValidatorStakeChange:   0,
		StakeEndEpochChange:    0,
		FixedCostChange:        0,
		DelegationStakeChange:  0,
	}, true, ts.Nodes()...)

	newAccount := ts.AccountOutput("TX2:0")
	newAccountOutput := newAccount.Output().(*iotago.AccountOutput)

	ts.AssertAccountDiff(newAccountOutput.AccountID, block2Slot, &model.AccountDiff{
		BICChange:              0,
		PreviousUpdatedTime:    0,
		NewExpirySlot:          newAccountExpirySlot,
		PreviousExpirySlot:     0,
		NewOutputID:            newAccount.OutputID(),
		PreviousOutputID:       iotago.EmptyOutputID,
		BlockIssuerKeysAdded:   iotago.NewBlockIssuerKeys(newAccountBlockIssuerKey),
		BlockIssuerKeysRemoved: iotago.NewBlockIssuerKeys(),
		ValidatorStakeChange:   10000,
		StakeEndEpochChange:    10,
		FixedCostChange:        421,
		DelegationStakeChange:  0,
	}, false, ts.Nodes()...)

	ts.AssertAccountData(&accounts.AccountData{
		ID:              newAccountOutput.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(0, block2Slot),
		ExpirySlot:      newAccountExpirySlot,
		OutputID:        newAccount.OutputID(),
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(newAccountBlockIssuerKey),
		StakeEndEpoch:   10,
		FixedCost:       421,
		DelegationStake: 0,
		ValidatorStake:  10000,
	}, ts.Nodes()...)

	accountAddress := iotago.AccountAddress(newAccountOutput.AccountID)
	// create a delegation output delegating to the newly created account
	inputForNewDelegation, newDelegationOutputs, newDelegationWallets := ts.TransactionFramework.CreateDelegationFromInput("TX1:2",
		testsuite.WithDelegatedValidatorAddress(&accountAddress),
		testsuite.WithDelegationStartEpoch(1),
	)

	block3Slot := latestParent.ID().Slot()

	tx3 := lo.PanicOnErr(ts.TransactionFramework.CreateSignedTransactionWithOptions("TX3", newDelegationWallets,
		testsuite.WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.CommitmentInput{
				CommitmentID: node1.Protocol.MainEngine.Get().Storage.Settings().LatestCommitment().Commitment().MustID(),
			},
		}),
		testsuite.WithInputs(inputForNewDelegation),
		testsuite.WithOutputs(newDelegationOutputs),
		testsuite.WithSlotCreated(block3Slot),
	))

	block3 := ts.IssueBasicBlockAtSlotWithOptions("block3", block3Slot, node1.Protocol.MainEngine.Get().Storage.Settings().LatestCommitment().Commitment(), blockIssuer, node1, tx3, mock.WithStrongParents(latestParent.ID()))

	latestParent = ts.CommitUntilSlot(block3Slot, block3)
	delegatedAmount := inputForNewDelegation[0].BaseTokenAmount()

	ts.AssertAccountDiff(newAccountOutput.AccountID, block3Slot, &model.AccountDiff{
		BICChange:              0,
		PreviousUpdatedTime:    0,
		NewOutputID:            iotago.EmptyOutputID,
		PreviousOutputID:       iotago.EmptyOutputID,
		BlockIssuerKeysAdded:   iotago.NewBlockIssuerKeys(),
		BlockIssuerKeysRemoved: iotago.NewBlockIssuerKeys(),
		ValidatorStakeChange:   0,
		StakeEndEpochChange:    0,
		FixedCostChange:        0,
		DelegationStakeChange:  int64(delegatedAmount),
	}, false, ts.Nodes()...)

	ts.AssertAccountData(&accounts.AccountData{
		ID:              newAccountOutput.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(0, block2Slot),
		ExpirySlot:      newAccountExpirySlot,
		OutputID:        newAccount.OutputID(),
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(newAccountBlockIssuerKey),
		StakeEndEpoch:   10,
		FixedCost:       421,
		DelegationStake: iotago.BaseToken(delegatedAmount),
		ValidatorStake:  10000,
	}, ts.Nodes()...)

	// transition a delegation output to a delayed claiming state
	inputForDelegationTransition, delegationTransitionOutputs, delegationTransitionWallets := ts.TransactionFramework.DelayedClaimingTransition("TX3:0", 0)

	tx4 := lo.PanicOnErr(ts.TransactionFramework.CreateSignedTransactionWithOptions("TX4", delegationTransitionWallets,
		testsuite.WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.CommitmentInput{
				CommitmentID: node1.Protocol.MainEngine.Get().Storage.Settings().LatestCommitment().Commitment().MustID(),
			},
		}),
		testsuite.WithInputs(inputForDelegationTransition),
		testsuite.WithOutputs(delegationTransitionOutputs),
		testsuite.WithSlotCreated(block3Slot),
	))

	block4Slot := latestParent.ID().Slot()

	block4 := ts.IssueBasicBlockAtSlotWithOptions("block4", block4Slot, node1.Protocol.MainEngine.Get().Storage.Settings().LatestCommitment().Commitment(), blockIssuer, node1, tx4, mock.WithStrongParents(latestParent.ID()))

	latestParent = ts.CommitUntilSlot(block4Slot, block4)

	// Transitioning to delayed claiming effectively removes the delegation, so we expect a negative delegation stake change.
	ts.AssertAccountDiff(newAccountOutput.AccountID, block4Slot, &model.AccountDiff{
		BICChange:              0,
		PreviousUpdatedTime:    0,
		NewOutputID:            iotago.EmptyOutputID,
		PreviousOutputID:       iotago.EmptyOutputID,
		BlockIssuerKeysAdded:   iotago.NewBlockIssuerKeys(),
		BlockIssuerKeysRemoved: iotago.NewBlockIssuerKeys(),
		ValidatorStakeChange:   0,
		StakeEndEpochChange:    0,
		FixedCostChange:        0,
		DelegationStakeChange:  -int64(delegatedAmount),
	}, false, ts.Nodes()...)

	ts.AssertAccountData(&accounts.AccountData{
		ID:              newAccountOutput.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(0, block2Slot),
		ExpirySlot:      newAccountExpirySlot,
		OutputID:        newAccount.OutputID(),
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(newAccountBlockIssuerKey),
		StakeEndEpoch:   10,
		FixedCost:       421,
		DelegationStake: iotago.BaseToken(0),
		ValidatorStake:  10000,
	}, ts.Nodes()...)

	// CREATE IMPLICIT ACCOUNT FROM BASIC UTXO
	inputForImplicitAccount, outputsForImplicitAccount, implicitAccountAddress, implicitWallet := ts.TransactionFramework.CreateImplicitAccountFromInput("TX1:3")

	tx5 := lo.PanicOnErr(ts.TransactionFramework.CreateSignedTransactionWithOptions("TX5", implicitWallet,
		testsuite.WithInputs(inputForImplicitAccount),
		testsuite.WithOutputs(outputsForImplicitAccount),
	))

	implicitAccountOutput := ts.TransactionFramework.Output("TX5:0")
	implicitAccountOutputID := implicitAccountOutput.OutputID()
	implicitAccountID := iotago.AccountIDFromOutputID(implicitAccountOutputID)

	slotIndexBlock5 := latestParent.ID().Index()

	block5 := ts.IssueBasicBlockAtSlotWithOptions("block5", slotIndexBlock5, node1.Protocol.MainEngine.Get().Storage.Settings().LatestCommitment().Commitment(), blockIssuer, node1, tx5, mock.WithStrongParents(latestParent.ID()))

	latestParent = ts.CommitUntilSlot(slotIndexBlock5, block5)

	var implicitBlockIssuerKey iotago.BlockIssuerKey = iotago.Ed25519PublicKeyHashBlockIssuerKeyFromImplicitAccountCreationAddress(implicitAccountAddress)

	ts.AssertAccountData(&accounts.AccountData{
		ID:              implicitAccountID,
		Credits:         accounts.NewBlockIssuanceCredits(0, slotIndexBlock5),
		ExpirySlot:      iotago.MaxSlotIndex,
		OutputID:        implicitAccountOutputID,
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(implicitBlockIssuerKey),
	}, ts.Nodes()...)

	// TRANSITION IMPLICIT ACCOUNT TO ACCOUNT OUTPUT

	fullAccountBlockIssuerKey := utils.RandBlockIssuerKey()

	inputForImplicitAccountTransition, outputsForImplicitAccountTransition, fullAccountWallet := ts.TransactionFramework.TransitionImplicitAccountToAccountOutput(
		"TX5:0",
		testsuite.WithBlockIssuerFeature(
			iotago.BlockIssuerKeys{fullAccountBlockIssuerKey},
			iotago.MaxSlotIndex,
		),
	)

	tx6 := lo.PanicOnErr(ts.TransactionFramework.CreateSignedTransactionWithOptions("TX6", fullAccountWallet,
		testsuite.WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.BlockIssuanceCreditInput{
				AccountID: implicitAccountID,
			},
			&iotago.CommitmentInput{
				CommitmentID: node1.Protocol.MainEngine.Get().Storage.Settings().LatestCommitment().Commitment().MustID(),
			},
		}),
		testsuite.WithInputs(inputForImplicitAccountTransition),
		testsuite.WithOutputs(outputsForImplicitAccountTransition),
		testsuite.WithSlotCreated(slotIndexBlock5),
	))

	slotIndexBlock6 := latestParent.ID().Index()

	block6 := ts.IssueBasicBlockAtSlotWithOptions("block6", slotIndexBlock6, node1.Protocol.MainEngine.Get().Storage.Settings().LatestCommitment().Commitment(), blockIssuer, node1, tx6, mock.WithStrongParents(latestParent.ID()))

	latestParent = ts.CommitUntilSlot(slotIndexBlock6, block6)

	fullAccountOutputID := ts.TransactionFramework.Output("TX6:0").OutputID()

	ts.AssertAccountDiff(implicitAccountID, slotIndexBlock6, &model.AccountDiff{
		BICChange:              0,
		PreviousUpdatedTime:    0,
		NewOutputID:            fullAccountOutputID,
		PreviousOutputID:       implicitAccountOutputID,
		PreviousExpirySlot:     iotago.MaxSlotIndex,
		NewExpirySlot:          iotago.MaxSlotIndex,
		BlockIssuerKeysAdded:   iotago.BlockIssuerKeys{fullAccountBlockIssuerKey},
		BlockIssuerKeysRemoved: iotago.BlockIssuerKeys{implicitBlockIssuerKey},
		ValidatorStakeChange:   0,
		StakeEndEpochChange:    0,
		FixedCostChange:        0,
		DelegationStakeChange:  0,
	}, false, ts.Nodes()...)

	ts.AssertAccountData(&accounts.AccountData{
		ID:              implicitAccountID,
		Credits:         accounts.NewBlockIssuanceCredits(0, slotIndexBlock5),
		ExpirySlot:      iotago.MaxSlotIndex,
		OutputID:        fullAccountOutputID,
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(fullAccountBlockIssuerKey),
	}, ts.Nodes()...)

	ts.Wait(ts.Nodes()...)
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
