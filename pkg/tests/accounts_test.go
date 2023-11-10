package tests

import (
	"fmt"
	"testing"

	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/pkg/testsuite/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Starts with an account already existing in snapshot (default wallet).
// 1. Change expiry slot to expire soon + add new block issuer key.
// 2. Destroy account.
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

	// 1. MODIFY EXISTING GENESIS ACCOUNT TO HAVE NEW EXPIRY SLOT AND NEW BLOCK ISSUER KEY.
	newGenesisOutputKey := utils.RandBlockIssuerKey()
	var block1Slot iotago.SlotIndex = 1
	ts.SetCurrentSlot(block1Slot)
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
	block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1, mock.WithSlotCommitment(genesisCommitment))
	latestParents := ts.CommitUntilSlot(ts.BlockID("block1").Slot(), block1.ID())

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

	// 2. DESTROY GENESIS ACCOUNT
	// commit until the expiry slot of the transitioned genesis account plus one.
	latestParents = ts.CommitUntilSlot(newExpirySlot+1, latestParents...)

	// create a transaction which destroys the genesis account.
	tx2 := ts.DefaultWallet().DestroyAccount("TX2", "TX1:0")
	block2 := ts.IssueBasicBlockWithOptions("block2", ts.DefaultWallet(), tx2, mock.WithStrongParents(latestParents...))
	block2Slot := ts.CurrentSlot()
	latestParents = ts.CommitUntilSlot(block2Slot, block2.ID())

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

// Starts with an account already existing in snapshot (default wallet).
// 1. Use default wallet to create a new account with staking feature from genesis basic output.
// 2. Use default wallet to create a delegation to the new account from a basic output.
// 3. Use default wallet to transition the delegation to delayed claiming.
func Test_StakeAndDelegate(t *testing.T) {
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
	fmt.Println(wallet.BlockIssuer.AccountID)
	ts.AssertAccountData(&accounts.AccountData{
		ID:              wallet.BlockIssuer.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.MaxBlockIssuanceCredits/2, 0),
		OutputID:        blockIssuerAccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: wallet.BlockIssuer.BlockIssuerKeys(),
	}, ts.Nodes()...)

	// 1. CREATE NEW ACCOUNT WITH BLOCK ISSUER AND STAKING FEATURES FROM BASIC UTXO
	newAccountBlockIssuerKey := utils.RandBlockIssuerKey()
	// set the expiry slot of the new account to note expire
	newAccountExpirySlot := iotago.MaxSlotIndex

	var block1Slot iotago.SlotIndex = 1
	ts.SetCurrentSlot(block1Slot)
	tx1 := ts.DefaultWallet().CreateAccountFromInput(
		"TX1",
		"Genesis:0",
		ts.DefaultWallet(),
		mock.WithBlockIssuerFeature(iotago.BlockIssuerKeys{newAccountBlockIssuerKey}, newAccountExpirySlot),
		mock.WithStakingFeature(10000, 421, 0, 10),
		mock.WithAccountAmount(mock.MinIssuerAccountAmount),
	)

	genesisCommitment := iotago.NewEmptyCommitment(ts.API)
	genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
	block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1)
	latestParents := ts.CommitUntilSlot(block1Slot, block1.ID())

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

	// 2. CREATE DELEGATION TO NEW ACCOUNT FROM BASIC UTXO
	accountAddress := iotago.AccountAddress(newAccountOutput.AccountID)
	block2Slot := ts.CurrentSlot()
	delegationStartEpoch := iotago.EpochIndex(1)

	tx2 := ts.DefaultWallet().CreateDelegationFromInput(
		"TX2",
		"TX1:1",
		mock.WithDelegatedValidatorAddress(&accountAddress),
		mock.WithDelegationStartEpoch(delegationStartEpoch),
	)
	block2 := ts.IssueBasicBlockWithOptions("block2", ts.DefaultWallet(), tx2, mock.WithStrongParents(latestParents...))

	latestParents = ts.CommitUntilSlot(block2Slot, block2.ID())
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

	// 3. TRANSITION DELEGATION TO DELAYED CLAIMING
	block3Slot := ts.CurrentSlot()
	tx3 := ts.DefaultWallet().DelayedClaimingTransition("TX3", "TX2:0", 0)
	block3 := ts.IssueBasicBlockWithOptions("block3", ts.DefaultWallet(), tx3, mock.WithStrongParents(latestParents...))

	latestParents = ts.CommitUntilSlot(block3Slot, block3.ID())

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

	// 4. WAIT 2 EPOCHS, THEN CLAIM REWARDS AND DESTROY DELEGATION.
	//latestParent = ts.CommitUntilSlot(ts.API.TimeProvider().EpochStart(delegationStartEpoch+2), latestParent)

	// create a transaction which claims rewards and destroys the delegation.
	//tx4 := delegatorWallet.ClaimDelegatorRewards("TX4", "TX3:0")
}

// Starts with an account already existing in snapshot (default wallet).
// 1. Use default wallet to send an implicit account output to a new user wallet.
// 2. Use implicit account itself to transition the implicit account output to a full account.
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

	// 1. CREATE IMPLICIT ACCOUNT FROM GENESIS BASIC UTXO, SENT TO A NEW USER WALLET.
	// this wallet is not registered in the ledger yet.
	newUserWallet := mock.NewWallet(ts.Testing, "newUser", node1)
	// a default wallet, already registered in the ledger, will issue the transaction and block.
	tx1 := ts.DefaultWallet().CreateImplicitAccountFromInput(
		"TX1",
		"Genesis:0",
		newUserWallet,
	)
	var block1Slot iotago.SlotIndex = 1
	ts.SetCurrentSlot(block1Slot)
	block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1)
	latestParents := ts.CommitUntilSlot(block1Slot, block1.ID())

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

	// 2. TRANSITION IMPLICIT ACCOUNT TO ACCOUNT OUTPUT.
	// USE IMPLICIT ACCOUNT AS BLOCK ISSUER.
	fullAccountBlockIssuerKey := utils.RandBlockIssuerKey()

	block2Slot := ts.CurrentSlot()
	tx2 := newUserWallet.TransitionImplicitAccountToAccountOutput(
		"TX2",
		"TX1:0",
		mock.WithBlockIssuerFeature(
			iotago.BlockIssuerKeys{fullAccountBlockIssuerKey},
			iotago.MaxSlotIndex,
		),
		mock.WithAccountAmount(mock.MinIssuerAccountAmount),
	)
	block2Commitment := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()
	block2 := ts.IssueBasicBlockWithOptions("block2", newUserWallet, tx2, mock.WithStrongParents(latestParents...))
	latestParents = ts.CommitUntilSlot(block2Slot, block2.ID())

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
Testcases:

- Test that Reward Inputs can point to an account id that does not exist or has no rewards in the rewards data.
When fetching rewards for accounts that don't exist (or have no rewards) the node should return 0 rather than an error.
This is because reward inputs are required for removing a Staking Feature from an Account or for destroying a Delegation Output.
Otherwise, if I delegated to some address that doesn't exist, I could not destroy my delegation output because the reward would error.
(I haven't verified whether this is already implemented as-is or not, but it should be tested regardless).

- Test that Reward Inputs can only point to Accounts & Delegation Outputs.
I tried to quickly find that this is checked somewhere but couldn't, so perhaps it's missing.

- Test that a transaction fails whenever the condition for claiming is not given.
That is, coming up with scenarios where claiming is not allowed and try to claim (remove staking feature, destroy delegation output).

- Test delegation stuff
// Step 1

Create Delegation Output A delegating to X

// Step 2 (separate epoch)

Create Account X with Staking Feature
Create Delegation Output B delegating to X
Create Delegation Output C delegating to X
Create Delegation Output D delegating to X

In this test X is never selected as a validator. Any Delegation Output that is destroyed should claim 0 rewards.

Assert expectations: X's total stake should be X + A + B + C + D. (I.e. make sure that A is counted even though it was created before X created its staking feature).

// Step 3

Destroy A
Create Delegation Output E delegating to X

Assert expectations: X's total stake should be X + B + C + D + E.

// Step 4

Transition B to Delayed Claiming.
Destroy C.

Assert expectations: X's total stake should be X + D + E.

// Step 5

Destroy Account.

Assert expectations: X should be removed from the "staking tracking data" (not sure what it's called exactly).

// Step 6

Destroy B, D & E.

Assert expectations.


1. Create account w/out BIC from normal UTXO.
2. Create account w/ BIC from normal UTXO.
3. Transition non-BIC account to BIC account.
4. Transition BIC account to non-BIC account.
5. Transition BIC account to BIC account changing amount/keys/expiry.
6. Destroy account w/out BIC feature.
7. Destroy account w/ BIC feature.
*/
