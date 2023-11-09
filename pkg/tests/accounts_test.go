package tests

import (
	"testing"

	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/pkg/testsuite/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

func Test_TransitionAndDestroyAccount(t *testing.T) {
	oldGenesisOutputKey := utils.RandBlockIssuerKey()

	ts := testsuite.NewTestSuite(t, testsuite.WithAccounts(snapshotcreator.AccountDetails{
		// Nil address will be replaced with the address generated from genesis seed.
		Address: nil,
		// Set an amount enough to cover storage deposit and more issuer keys.
		Amount: mock.MinIssuerAccountAmount * 10,
		Mana:   0,
		// AccountID is derived from this field, so this must be set uniquely for each account.
		IssuerKey: oldGenesisOutputKey,
		// Expiry Slot is the slot index at which the account expires.
		ExpirySlot: iotago.MaxSlotIndex,
		// BlockIssuanceCredits on this account is custom because it never needs to issue.
		BlockIssuanceCredits: iotago.BlockIssuanceCredits(123),
	}),
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(200, testsuite.DefaultSlotDurationInSeconds),
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
	wallet := ts.AddGenesisWallet("default", node1, iotago.MaxBlockIssuanceCredits/2)

	ts.Run(true)

	// check that the accounts added in the genesis snapshot were added to account manager correctly.
	// genesis account.
	genesisAccount := ts.AccountOutput("Genesis:1")
	genesisAccountOutput := genesisAccount.Output().(*iotago.AccountOutput)
	ts.AssertAccountData(&accounts.AccountData{
		ID:              genesisAccountOutput.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.BlockIssuanceCredits(123), 0),
		OutputID:        genesisAccount.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
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
		ValidatorStake:  mock.MinValidatorAccountAmount,
	}, ts.Nodes()...)
	// default wallet block issuer account.
	blockIssuerAccountOutput := ts.AccountOutput("Genesis:3")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              wallet.BlockIssuer.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.MaxBlockIssuanceCredits/2, 0),
		OutputID:        blockIssuerAccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: wallet.BlockIssuer.BlockIssuerKeys(),
	}, ts.Nodes()...)

	// MODIFY EXISTING GENESIS ACCOUNT
	newGenesisOutputKey := utils.RandBlockIssuerKey()
	var block1Slot iotago.SlotIndex = 1
	// set the expiry of the genesis account to be the block slot + max committable age.
	newExpirySlot := block1Slot + ts.API.ProtocolParameters().MaxCommittableAge()

	tx1 := ts.DefaultWallet().TransitionAccount(
		"TX1",
		"Genesis:1",
		mock.WithAddBlockIssuerKey(newGenesisOutputKey),
		mock.WithBlockIssuerExpirySlot(newExpirySlot),
	)

	// default block issuer issues a block containing the transaction in slot 1.
	genesisCommitment := iotago.NewEmptyCommitment(ts.API)
	genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
	block1 := ts.IssueBasicBlockAtSlotWithOptions("block1", block1Slot, ts.DefaultWallet(), tx1, mock.WithSlotCommitment(genesisCommitment))
	latestParent := ts.CommitUntilSlot(ts.BlockID("block1").Slot(), block1)

	// assert diff of the genesis account, it should have a new output ID, new expiry slot and a new block issuer key.
	ts.AssertAccountDiff(genesisAccountOutput.AccountID, block1Slot, &model.AccountDiff{
		BICChange:              0,
		PreviousUpdatedSlot:    0,
		PreviousExpirySlot:     iotago.MaxSlotIndex,
		NewExpirySlot:          newExpirySlot,
		NewOutputID:            ts.DefaultWallet().Output("TX1:0").OutputID(),
		PreviousOutputID:       genesisAccount.OutputID(),
		BlockIssuerKeysRemoved: iotago.NewBlockIssuerKeys(),
		BlockIssuerKeysAdded:   iotago.NewBlockIssuerKeys(newGenesisOutputKey),
	}, false, ts.Nodes()...)

	ts.AssertAccountData(&accounts.AccountData{
		ID:              genesisAccountOutput.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.BlockIssuanceCredits(123), 0),
		OutputID:        ts.DefaultWallet().Output("TX1:0").OutputID(),
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(oldGenesisOutputKey, newGenesisOutputKey),
		ExpirySlot:      newExpirySlot,
	}, ts.Nodes()...)

	// DESTROY GENESIS ACCOUNT
	// commit until the expiry slot of the transitioned genesis account plus one.
	latestParent = ts.CommitUntilSlot(newExpirySlot+1, latestParent)

	// issue the block containing the transaction in the same slot as the latest parent block.
	block2Slot := latestParent.ID().Slot()
	// create a transaction which destroys the genesis account.
	tx2 := ts.DefaultWallet().DestroyAccount("TX2", "TX1:0", block2Slot)
	block2 := ts.IssueBasicBlockAtSlotWithOptions("block2", block2Slot, ts.DefaultWallet(), tx2, mock.WithStrongParents(latestParent.ID()))
	latestParent = ts.CommitUntilSlot(block2Slot, block2)

	// assert diff of the destroyed account.
	ts.AssertAccountDiff(genesisAccountOutput.AccountID, block2Slot, &model.AccountDiff{
		BICChange:              -iotago.BlockIssuanceCredits(123),
		PreviousUpdatedSlot:    0,
		NewExpirySlot:          0,
		PreviousExpirySlot:     newExpirySlot,
		NewOutputID:            iotago.EmptyOutputID,
		PreviousOutputID:       ts.DefaultWallet().Output("TX1:0").OutputID(),
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
				0,
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
	wallet := ts.AddGenesisWallet("default", node1, iotago.MaxBlockIssuanceCredits/2)

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
		ValidatorStake:  mock.MinValidatorAccountAmount,
	}, ts.Nodes()...)
	// default wallet block issuer account.
	blockIssuerAccountOutput := ts.AccountOutput("Genesis:2")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              wallet.BlockIssuer.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.MaxBlockIssuanceCredits/2, 0),
		OutputID:        blockIssuerAccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: wallet.BlockIssuer.BlockIssuerKeys(),
	}, ts.Nodes()...)

	//CREATE NEW ACCOUNT WITH BLOCK ISSUER AND STAKING FEATURES FROM BASIC UTXO
	newAccountBlockIssuerKey := utils.RandBlockIssuerKey()
	// set the expiry slot of the transitioned genesis account to the latest committed + MaxCommittableAge
	newAccountExpirySlot := node1.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Slot() + ts.API.ProtocolParameters().MaxCommittableAge()

	var block1Slot iotago.SlotIndex = 1
	tx1 := ts.DefaultWallet().CreateAccountFromInput(
		"TX1",
		"Genesis:0",
		ts.DefaultWallet(),
		block1Slot,
		mock.WithBlockIssuerFeature(iotago.BlockIssuerKeys{newAccountBlockIssuerKey}, newAccountExpirySlot),
		mock.WithStakingFeature(10000, 421, 0, 10),
		mock.WithAccountAmount(mock.MinIssuerAccountAmount),
	)

	genesisCommitment := iotago.NewEmptyCommitment(ts.API)
	genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
	block1 := ts.IssueBasicBlockAtSlotWithOptions("block1", block1Slot, ts.DefaultWallet(), tx1)
	latestParent := ts.CommitUntilSlot(block1Slot, block1)

	newAccount := ts.DefaultWallet().AccountOutput("TX1:0")
	newAccountOutput := newAccount.Output().(*iotago.AccountOutput)

	ts.AssertAccountDiff(newAccountOutput.AccountID, block1Slot, &model.AccountDiff{
		BICChange:              0,
		PreviousUpdatedSlot:    0,
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
	block2Slot := latestParent.ID().Slot()
	tx2 := ts.DefaultWallet().CreateDelegationFromInput(
		"TX2",
		"TX1:1",
		block2Slot,
		mock.WithDelegatedValidatorAddress(&accountAddress),
		mock.WithDelegationStartEpoch(1),
	)
	block2 := ts.IssueBasicBlockAtSlotWithOptions("block2", block2Slot, ts.DefaultWallet(), tx2, mock.WithStrongParents(latestParent.ID()))

	latestParent = ts.CommitUntilSlot(block2Slot, block2)
	delegatedAmount := ts.DefaultWallet().Output("TX1:1").BaseTokenAmount()

	ts.AssertAccountDiff(newAccountOutput.AccountID, block2Slot, &model.AccountDiff{
		BICChange:              0,
		PreviousUpdatedSlot:    0,
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

	// transition a delegation output to a delayed claiming state
	block3Slot := latestParent.ID().Slot()
	tx3 := ts.DefaultWallet().DelayedClaimingTransition("TX3", "TX2:0", block3Slot, 0)
	block3 := ts.IssueBasicBlockAtSlotWithOptions("block3", block3Slot, ts.DefaultWallet(), tx3, mock.WithStrongParents(latestParent.ID()))

	latestParent = ts.CommitUntilSlot(block3Slot, block3)

	// Transitioning to delayed claiming effectively removes the delegation, so we expect a negative delegation stake change.
	ts.AssertAccountDiff(newAccountOutput.AccountID, block3Slot, &model.AccountDiff{
		BICChange:              0,
		PreviousUpdatedSlot:    0,
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
				0,
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
	wallet := ts.AddGenesisWallet("default", node1, iotago.MaxBlockIssuanceCredits/2)

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
		ValidatorStake:  mock.MinValidatorAccountAmount,
	}, ts.Nodes()...)
	// default wallet block issuer account.
	blockIssuerAccountOutput := ts.AccountOutput("Genesis:2")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              wallet.BlockIssuer.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.MaxBlockIssuanceCredits/2, 0),
		OutputID:        blockIssuerAccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: wallet.BlockIssuer.BlockIssuerKeys(),
	}, ts.Nodes()...)

	// CREATE IMPLICIT ACCOUNT FROM GENESIS BASIC UTXO, SENT TO A NEW USER WALLET.
	// this wallet is not registered in the ledger yet.
	newUserWallet := mock.NewWallet(ts.Testing, "newUser", node1)
	// a default wallet, already registered in the ledger, will issue the transaction and block.
	tx1 := ts.DefaultWallet().CreateImplicitAccountFromInput(
		"TX1",
		"Genesis:0",
		newUserWallet,
	)
	var block1Slot iotago.SlotIndex = 1
	block1 := ts.IssueBasicBlockAtSlotWithOptions("block1", block1Slot, ts.DefaultWallet(), tx1)
	latestParent := ts.CommitUntilSlot(block1Slot, block1)

	implicitAccountOutput := newUserWallet.Output("TX1:0")
	implicitAccountOutputID := implicitAccountOutput.OutputID()
	implicitAccountID := iotago.AccountIDFromOutputID(implicitAccountOutputID)
	var implicitBlockIssuerKey iotago.BlockIssuerKey = iotago.Ed25519PublicKeyHashBlockIssuerKeyFromImplicitAccountCreationAddress(newUserWallet.ImplicitAccountCreationAddress())
	// the new implicit account should now be registered in the accounts ledger.
	ts.AssertAccountData(&accounts.AccountData{
		ID:              implicitAccountID,
		Credits:         accounts.NewBlockIssuanceCredits(0, block1Slot),
		ExpirySlot:      iotago.MaxSlotIndex,
		OutputID:        implicitAccountOutputID,
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(implicitBlockIssuerKey),
	}, ts.Nodes()...)

	// TRANSITION IMPLICIT ACCOUNT TO ACCOUNT OUTPUT.
	// USE IMPLICIT ACCOUNT AS BLOCK ISSUER.
	fullAccountBlockIssuerKey := utils.RandBlockIssuerKey()

	block2Slot := latestParent.ID().Index()
	tx2 := newUserWallet.TransitionImplicitAccountToAccountOutput(
		"TX2",
		"TX1:0",
		block2Slot,
		mock.WithBlockIssuerFeature(
			iotago.BlockIssuerKeys{fullAccountBlockIssuerKey},
			iotago.MaxSlotIndex,
		),
		mock.WithAccountAmount(mock.MinIssuerAccountAmount),
	)
	block2Commitment := node1.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment()
	block2 := ts.IssueBasicBlockAtSlotWithOptions("block2", block2Slot, newUserWallet, tx2, mock.WithStrongParents(latestParent.ID()))
	latestParent = ts.CommitUntilSlot(block2Slot, block2)

	fullAccountOutputID := newUserWallet.Output("TX2:0").OutputID()
	allotted := iotago.BlockIssuanceCredits(tx2.Transaction.Allotments.Get(implicitAccountID))
	burned := iotago.BlockIssuanceCredits(block2.WorkScore()) * iotago.BlockIssuanceCredits(block2Commitment.ReferenceManaCost)
	// the implicit account should now have been transitioned to a full account in the accounts ledger.
	ts.AssertAccountDiff(implicitAccountID, block2Slot, &model.AccountDiff{
		BICChange:              allotted - burned,
		PreviousUpdatedSlot:    block1Slot,
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
		Credits:         accounts.NewBlockIssuanceCredits(allotted-burned, block2Slot),
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
