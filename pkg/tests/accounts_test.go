package tests

import (
	"testing"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TODO: implement tests for staking and delegation transitions that cover edge cases - part of hardening phase.
func Test_TransitionAndDestroyAccount(t *testing.T) {
	oldGenesisOutputKey := utils.RandBlockIssuerKey()

	// add a genesis account to the snapshot which expires in slot 1.
	ts := testsuite.NewTestSuite(t, testsuite.WithAccounts(snapshotcreator.AccountDetails{
		// Nil address will be replaced with the address generated from genesis seed.
		Address: nil,
		// Set an amount enough to cover storage deposit and more issuer keys.
		Amount: testsuite.MinIssuerAccountAmount * 10,
		Mana:   0,
		// AccountID is derived from this field, so this must be set uniquely for each account.
		IssuerKey: oldGenesisOutputKey,
		// Expiry Slot is the slot index at which the account expires.
		ExpirySlot: 1,
		// BlockIssuanceCredits on this account is custom because it never needs to issue.
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

	// add a validator node to the network. This will add a validator account to the snapshot.
	node1 := ts.AddValidatorNode("node1")
	// add a non-validator node to the network. This will not add any accounts to the snapshot.
	_ = ts.AddNode("node2")
	// add a default block issuer to the network. This will add another block issuer account to the snapshot.
	blockIssuer := ts.AddBasicBlockIssuer("default", iotago.MaxBlockIssuanceCredits/2)

	ts.Run(true)

	// check that the accounts added in the genesis snapshot were added to account manager correctly.
	// genesis account.
	genesisAccount := ts.AccountOutput("Genesis:1")
	genesisAccountOutput := genesisAccount.Output().(*iotago.AccountOutput)
	ts.AssertAccountData(&accounts.AccountData{
		ID:              genesisAccountOutput.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.BlockIssuanceCredits(123), 0),
		OutputID:        genesisAccount.OutputID(),
		ExpirySlot:      1,
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(oldGenesisOutputKey),
	}, ts.Nodes()...)
	// validator node account.
	validatorAccountOutput := ts.AccountOutput("Genesis:2")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              node1.Validator.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.MaxBlockIssuanceCredits/2, 0),
		OutputID:        validatorAccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: node1.Validator.BlockIssuerKeys(),
		StakeEndEpoch:   iotago.MaxEpochIndex,
		ValidatorStake:  testsuite.MinValidatorAccountAmount,
	}, ts.Nodes()...)
	// default block issuer account.
	blockIssuerAccountOutput := ts.AccountOutput("Genesis:3")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              blockIssuer.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.MaxBlockIssuanceCredits/2, 0),
		OutputID:        blockIssuerAccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: blockIssuer.BlockIssuerKeys(),
	}, ts.Nodes()...)

	// MODIFY EXISTING GENESIS ACCOUNT

	newGenesisOutputKey := utils.RandBlockIssuerKey()
	latestCommitmentID := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID()

	accountInput, accountOutputs, accountWallets := ts.TransactionFramework.TransitionAccount(
		"Genesis:1",
		testsuite.WithAddBlockIssuerKey(newGenesisOutputKey),
		testsuite.WithBlockIssuerExpirySlot(1),
	)

	tx1 := lo.PanicOnErr(ts.TransactionFramework.CreateSignedTransactionWithOptions("TX1", accountWallets,
		testsuite.WithAccountInput(accountInput, true),
		testsuite.WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.BlockIssuanceCreditInput{
				AccountID: genesisAccountOutput.AccountID,
			},
			&iotago.CommitmentInput{
				CommitmentID: latestCommitmentID,
			},
		}),
		testsuite.WithOutputs(accountOutputs),
	))

	// defaults block issuer issues a block containing the transaction in slot 1.
	var block1Slot iotago.SlotIndex = 1
	genesisCommitment := iotago.NewEmptyCommitment(ts.API.ProtocolParameters().Version())
	genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
	block1 := ts.IssueBasicBlockAtSlotWithOptions("block1", block1Slot, genesisCommitment, blockIssuer, node1, tx1)
	latestParent := ts.CommitUntilSlot(ts.BlockID("block1").Slot(), block1)

	// assert diff of a genesis account, it should have a new output ID, same expiry slot and a new block issuer key.
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

	// DESTROY GENESIS ACCOUNT
	// commit until the expiry slot of the transitioned genesis account plus one.
	latestParent = ts.CommitUntilSlot(accountOutputs[0].FeatureSet().BlockIssuer().ExpirySlot+1, latestParent)

	// create a transaction which destroys the genesis account.
	destroyedAccountInput, destructionOutputs, destroyWallets := ts.TransactionFramework.DestroyAccount("TX1:0")

	// issue the block containing the transaction in the same slot as the latest parent block.
	block2Slot := latestParent.ID().Slot()

	tx2 := lo.PanicOnErr(ts.TransactionFramework.CreateSignedTransactionWithOptions("TX2", destroyWallets,
		testsuite.WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.BlockIssuanceCreditInput{
				AccountID: genesisAccountOutput.AccountID,
			},
			&iotago.CommitmentInput{
				CommitmentID: node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
			},
		}),
		testsuite.WithAccountInput(destroyedAccountInput, true),
		testsuite.WithOutputs(destructionOutputs),
		testsuite.WithSlotCreated(block2Slot),
	))

	block2 := ts.IssueBasicBlockAtSlotWithOptions("block2", block2Slot, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), blockIssuer, node1, tx2, mock.WithStrongParents(latestParent.ID()))
	latestParent = ts.CommitUntilSlot(block2Slot, block2)

	// assert diff of the destroyed account.
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
}

func Test_StakeDelegateAndDelayedClaim(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
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

	// add a validator node to the network. This will add a validator account to the snapshot.
	node1 := ts.AddValidatorNode("node1")
	// add a non-validator node to the network. This will not add any accounts to the snapshot.
	_ = ts.AddNode("node2")
	// add a default block issuer to the network. This will add another block issuer account to the snapshot.
	blockIssuer := ts.AddBasicBlockIssuer("default", iotago.MaxBlockIssuanceCredits/2)

	ts.Run(true)

	// assert validator and block issuer accounts in genesis snapshot.
	// validator node account.
	validatorAccountOutput := ts.AccountOutput("Genesis:1")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              node1.Validator.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.MaxBlockIssuanceCredits/2, 0),
		OutputID:        validatorAccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: node1.Validator.BlockIssuerKeys(),
		StakeEndEpoch:   iotago.MaxEpochIndex,
		ValidatorStake:  testsuite.MinValidatorAccountAmount,
	}, ts.Nodes()...)
	// default block issuer account.
	blockIssuerAccountOutput := ts.AccountOutput("Genesis:2")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              blockIssuer.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.MaxBlockIssuanceCredits/2, 0),
		OutputID:        blockIssuerAccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: blockIssuer.BlockIssuerKeys(),
	}, ts.Nodes()...)

	//CREATE NEW ACCOUNT WITH BLOCK ISSUER AND STAKING FEATURES FROM BASIC UTXO
	newAccountBlockIssuerKey := utils.RandBlockIssuerKey()
	// set the expiry slot of the transitioned genesis account to the latest committed + MaxCommittableAge
	newAccountExpirySlot := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Slot() + ts.API.ProtocolParameters().MaxCommittableAge()

	inputForNewAccount, newAccountOutputs, newAccountWallets := ts.TransactionFramework.CreateAccountFromInput("Genesis:0",
		testsuite.WithBlockIssuerFeature(iotago.BlockIssuerKeys{newAccountBlockIssuerKey}, newAccountExpirySlot),
		testsuite.WithStakingFeature(10000, 421, 0, 10),
		testsuite.WithAccountAmount(testsuite.MinIssuerAccountAmount),
	)

	var block1Slot iotago.SlotIndex = 1
	tx1 := lo.PanicOnErr(ts.TransactionFramework.CreateSignedTransactionWithOptions("TX1", newAccountWallets,
		testsuite.WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.CommitmentInput{
				CommitmentID: node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
			},
		}),
		testsuite.WithInputs(inputForNewAccount),
		testsuite.WithOutputs(newAccountOutputs),
		testsuite.WithSlotCreated(block1Slot),
	))

	genesisCommitment := iotago.NewEmptyCommitment(ts.API.ProtocolParameters().Version())
	genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
	block1 := ts.IssueBasicBlockAtSlotWithOptions("block1", block1Slot, genesisCommitment, blockIssuer, node1, tx1)
	latestParent := ts.CommitUntilSlot(block1Slot, block1)

	newAccount := ts.AccountOutput("TX1:0")
	newAccountOutput := newAccount.Output().(*iotago.AccountOutput)

	ts.AssertAccountDiff(newAccountOutput.AccountID, block1Slot, &model.AccountDiff{
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
		Credits:         accounts.NewBlockIssuanceCredits(0, block1Slot),
		ExpirySlot:      newAccountExpirySlot,
		OutputID:        newAccount.OutputID(),
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(newAccountBlockIssuerKey),
		StakeEndEpoch:   10,
		FixedCost:       421,
		DelegationStake: 0,
		ValidatorStake:  10000,
	}, ts.Nodes()...)

	// CREATE DELEGATION TO NEW ACCOUNT FROM BASIC UTXO
	accountAddress := iotago.AccountAddress(newAccountOutput.AccountID)
	inputForNewDelegation, newDelegationOutputs, newDelegationWallets := ts.TransactionFramework.CreateDelegationFromInput("TX1:1",
		testsuite.WithDelegatedValidatorAddress(&accountAddress),
		testsuite.WithDelegationStartEpoch(1),
	)

	block2Slot := latestParent.ID().Slot()

	tx2 := lo.PanicOnErr(ts.TransactionFramework.CreateSignedTransactionWithOptions("TX2", newDelegationWallets,
		testsuite.WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.CommitmentInput{
				CommitmentID: node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
			},
		}),
		testsuite.WithInputs(inputForNewDelegation),
		testsuite.WithOutputs(newDelegationOutputs),
		testsuite.WithSlotCreated(block2Slot),
	))

	block2 := ts.IssueBasicBlockAtSlotWithOptions("block3", block2Slot, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), blockIssuer, node1, tx2, mock.WithStrongParents(latestParent.ID()))

	latestParent = ts.CommitUntilSlot(block2Slot, block2)
	delegatedAmount := inputForNewDelegation[0].BaseTokenAmount()

	ts.AssertAccountDiff(newAccountOutput.AccountID, block2Slot, &model.AccountDiff{
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
		Credits:         accounts.NewBlockIssuanceCredits(0, block1Slot),
		ExpirySlot:      newAccountExpirySlot,
		OutputID:        newAccount.OutputID(),
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(newAccountBlockIssuerKey),
		StakeEndEpoch:   10,
		FixedCost:       421,
		DelegationStake: iotago.BaseToken(delegatedAmount),
		ValidatorStake:  10000,
	}, ts.Nodes()...)

	// TRANSITION DELGATION OUTPUT TO DELAYED CLAIMING STATE
	inputForDelegationTransition, delegationTransitionOutputs, delegationTransitionWallets := ts.TransactionFramework.DelayedClaimingTransition("TX2:0", 0)

	tx3 := lo.PanicOnErr(ts.TransactionFramework.CreateSignedTransactionWithOptions("TX3", delegationTransitionWallets,
		testsuite.WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.CommitmentInput{
				CommitmentID: node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
			},
		}),
		testsuite.WithInputs(inputForDelegationTransition),
		testsuite.WithOutputs(delegationTransitionOutputs),
		testsuite.WithSlotCreated(block2Slot),
	))

	block3Slot := latestParent.ID().Slot()

	block3 := ts.IssueBasicBlockAtSlotWithOptions("block3", block3Slot, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), blockIssuer, node1, tx3, mock.WithStrongParents(latestParent.ID()))

	latestParent = ts.CommitUntilSlot(block3Slot, block3)

	// Transitioning to delayed claiming effectively removes the delegation, so we expect a negative delegation stake change.
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
		DelegationStakeChange:  -int64(delegatedAmount),
	}, false, ts.Nodes()...)

	ts.AssertAccountData(&accounts.AccountData{
		ID:              newAccountOutput.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(0, block1Slot),
		ExpirySlot:      newAccountExpirySlot,
		OutputID:        newAccount.OutputID(),
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(newAccountBlockIssuerKey),
		StakeEndEpoch:   10,
		FixedCost:       421,
		DelegationStake: iotago.BaseToken(0),
		ValidatorStake:  10000,
	}, ts.Nodes()...)
}

func Test_ImplicitAccounts(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
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

	// add a validator node to the network. This will add a validator account to the snapshot.
	node1 := ts.AddValidatorNode("node1")
	// add a non-validator node to the network. This will not add any accounts to the snapshot.
	_ = ts.AddNode("node2")
	// add a default block issuer to the network. This will add another block issuer account to the snapshot.
	blockIssuer := ts.AddBasicBlockIssuer("default", iotago.MaxBlockIssuanceCredits/2)

	ts.Run(true)

	// assert validator and block issuer accounts in genesis snapshot.
	// validator node account.
	validatorAccountOutput := ts.AccountOutput("Genesis:1")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              node1.Validator.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.MaxBlockIssuanceCredits/2, 0),
		OutputID:        validatorAccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: node1.Validator.BlockIssuerKeys(),
		StakeEndEpoch:   iotago.MaxEpochIndex,
		ValidatorStake:  testsuite.MinValidatorAccountAmount,
	}, ts.Nodes()...)
	// default block issuer account.
	blockIssuerAccountOutput := ts.AccountOutput("Genesis:2")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              blockIssuer.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.MaxBlockIssuanceCredits/2, 0),
		OutputID:        blockIssuerAccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: blockIssuer.BlockIssuerKeys(),
	}, ts.Nodes()...)

	// CREATE IMPLICIT ACCOUNT FROM BASIC UTXO
	inputForImplicitAccount, outputsForImplicitAccount, implicitAccountAddress, implicitWallet := ts.TransactionFramework.CreateImplicitAccountFromInput("Genesis:0")
	tx1 := lo.PanicOnErr(ts.TransactionFramework.CreateSignedTransactionWithOptions("TX1", implicitWallet,
		testsuite.WithInputs(inputForImplicitAccount),
		testsuite.WithOutputs(outputsForImplicitAccount),
	))

	implicitAccountOutput := ts.TransactionFramework.Output("TX1:0")
	implicitAccountOutputID := implicitAccountOutput.OutputID()
	implicitAccountID := iotago.AccountIDFromOutputID(implicitAccountOutputID)

	var block1Slot iotago.SlotIndex = 1

	block1 := ts.IssueBasicBlockAtSlotWithOptions("block1", block1Slot, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), blockIssuer, node1, tx1)

	latestParent := ts.CommitUntilSlot(block1Slot, block1)

	var implicitBlockIssuerKey iotago.BlockIssuerKey = iotago.Ed25519PublicKeyHashBlockIssuerKeyFromImplicitAccountCreationAddress(implicitAccountAddress)

	ts.AssertAccountData(&accounts.AccountData{
		ID:              implicitAccountID,
		Credits:         accounts.NewBlockIssuanceCredits(0, block1Slot),
		ExpirySlot:      iotago.MaxSlotIndex,
		OutputID:        implicitAccountOutputID,
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(implicitBlockIssuerKey),
	}, ts.Nodes()...)

	// TRANSITION IMPLICIT ACCOUNT TO ACCOUNT OUTPUT.
	// TODO: USE IMPLICIT ACCOUNT AS BLOCK ISSUER.
	fullAccountBlockIssuerKey := utils.RandBlockIssuerKey()
	inputForImplicitAccountTransition, outputsForImplicitAccountTransition, fullAccountWallet := ts.TransactionFramework.TransitionImplicitAccountToAccountOutput(
		"TX1:0",
		testsuite.WithBlockIssuerFeature(
			iotago.BlockIssuerKeys{fullAccountBlockIssuerKey},
			iotago.MaxSlotIndex,
		),
	)
	tx2 := lo.PanicOnErr(ts.TransactionFramework.CreateSignedTransactionWithOptions("TX2", fullAccountWallet,
		testsuite.WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.BlockIssuanceCreditInput{
				AccountID: implicitAccountID,
			},
			&iotago.CommitmentInput{
				CommitmentID: node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
			},
		}),
		testsuite.WithInputs(inputForImplicitAccountTransition),
		testsuite.WithOutputs(outputsForImplicitAccountTransition),
		testsuite.WithSlotCreated(block1Slot),
	))

	block2Slot := latestParent.ID().Index()

	block2 := ts.IssueBasicBlockAtSlotWithOptions("block2", block2Slot, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), blockIssuer, node1, tx2, mock.WithStrongParents(latestParent.ID()))

	latestParent = ts.CommitUntilSlot(block2Slot, block2)

	fullAccountOutputID := ts.TransactionFramework.Output("TX2:0").OutputID()

	ts.AssertAccountDiff(implicitAccountID, block2Slot, &model.AccountDiff{
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
		Credits:         accounts.NewBlockIssuanceCredits(0, block1Slot),
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
