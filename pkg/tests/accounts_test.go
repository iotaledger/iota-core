package tests

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/depositcalculator"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/pkg/testsuite/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

func Test_TransitionAndDestroyAccount(t *testing.T) {
	oldGenesisOutputKey := utils.RandBlockIssuerKey()

	ts := testsuite.NewTestSuite(t,
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
				120,
			),
		),
	)
	defer ts.Shutdown()

	ts.AddGenesisAccount(snapshotcreator.AccountDetails{
		// Nil address will be replaced with the address generated from genesis seed.
		Address: nil,
		// Set an amount enough to cover storage deposit and more issuer keys.
		Amount: mock.MinIssuerAccountAmount(ts.API.ProtocolParameters()) * 10,
		Mana:   0,
		// AccountID is derived from this field, so this must be set uniquely for each account.
		IssuerKey: oldGenesisOutputKey,
		// Expiry Slot is the slot index at which the account expires.
		ExpirySlot: iotago.MaxSlotIndex,
		// BlockIssuanceCredits on this account is custom because it never needs to issue.
		BlockIssuanceCredits: iotago.BlockIssuanceCredits(123),
	})

	// Add a validator node to the network. This will add a validator account to the snapshot.
	node1 := ts.AddValidatorNode("node1")
	// Add a non-validator node to the network. This will not add any accounts to the snapshot.
	_ = ts.AddNode("node2")
	// Add a default block issuer to the network. This will add another block issuer account to the snapshot.
	wallet := ts.AddDefaultWallet(node1)

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
		ValidatorStake:  mock.MinValidatorAccountAmount(ts.API.ProtocolParameters()),
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
	ts.SetCurrentSlot(block1Slot)
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

	// DESTROY GENESIS ACCOUNT
	// commit until the expiry slot of the transitioned genesis account plus one.
	latestParents = ts.CommitUntilSlot(newExpirySlot+1, latestParents...)

	// issue the block containing the transaction in the same slot as the latest parent block.
	block2Slot := ts.CurrentSlot()
	// create a transaction which destroys the genesis account.
	tx2 := ts.DefaultWallet().DestroyAccount("TX2", "TX1:0")
	block2 := ts.IssueBasicBlockWithOptions("block2", ts.DefaultWallet(), tx2, mock.WithStrongParents(latestParents...))
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
				120,
			),
		),
	)
	defer ts.Shutdown()

	// Add a validator node to the network. This will add a validator account to the snapshot.
	node1 := ts.AddValidatorNode("node1")
	// Add a non-validator node to the network. This will not add any accounts to the snapshot.
	_ = ts.AddNode("node2")
	// Add a default block issuer to the network. This will add another block issuer account to the snapshot.
	wallet := ts.AddDefaultWallet(node1)

	ts.Run(true)

	// Assert validator and block issuer accounts in genesis snapshot.
	// Validator node account.
	validatorAccountOutput := ts.AccountOutput("Genesis:1")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              node1.Validator.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.MaxBlockIssuanceCredits/2, 0),
		OutputID:        validatorAccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: node1.Validator.BlockIssuerKeys(),
		StakeEndEpoch:   iotago.MaxEpochIndex,
		ValidatorStake:  mock.MinValidatorAccountAmount(ts.API.ProtocolParameters()),
	}, ts.Nodes()...)

	// Default wallet block issuer account.
	blockIssuerAccountOutput := ts.AccountOutput("Genesis:2")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              wallet.BlockIssuer.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.MaxBlockIssuanceCredits/2, 0),
		OutputID:        blockIssuerAccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: wallet.BlockIssuer.BlockIssuerKeys(),
	}, ts.Nodes()...)

	// CREATE NEW ACCOUNT WITH BLOCK ISSUER AND STAKING FEATURES FROM BASIC UTXO
	newAccountBlockIssuerKey := utils.RandBlockIssuerKey()
	// set the expiry slot of the transitioned genesis account to the latest committed + MaxCommittableAge
	newAccountExpirySlot := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Slot() + ts.API.ProtocolParameters().MaxCommittableAge()

	stakedAmount := iotago.BaseToken(10000)

	validatorAccountAmount, err := depositcalculator.MinDeposit(ts.API.ProtocolParameters(), iotago.OutputAccount,
		depositcalculator.WithAddress(&iotago.Ed25519Address{}),
		depositcalculator.WithBlockIssuerKeys(1),
		depositcalculator.WithStakedAmount(stakedAmount),
	)
	require.NoError(t, err)

	var block1Slot iotago.SlotIndex = 1
	tx1 := ts.DefaultWallet().CreateAccountFromInput(
		"TX1",
		"Genesis:0",
		ts.DefaultWallet(),
		mock.WithBlockIssuerFeature(iotago.BlockIssuerKeys{newAccountBlockIssuerKey}, newAccountExpirySlot),
		mock.WithStakingFeature(stakedAmount, 421, 0, 10), // match amount and staked amount to simplify the tests
		mock.WithAccountAmount(validatorAccountAmount),
	)

	genesisCommitment := iotago.NewEmptyCommitment(ts.API)
	genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
	ts.SetCurrentSlot(block1Slot)
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
		ValidatorStakeChange:   int64(stakedAmount),
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
		ValidatorStake:  stakedAmount,
	}, ts.Nodes()...)

	// CREATE DELEGATION TO NEW ACCOUNT FROM BASIC UTXO
	accountAddress := iotago.AccountAddress(newAccountOutput.AccountID)
	block2Slot := ts.CurrentSlot()
	tx2 := ts.DefaultWallet().CreateDelegationFromInput(
		"TX2",
		"TX1:1",
		mock.WithDelegatedValidatorAddress(&accountAddress),
		mock.WithDelegationStartEpoch(1),
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
		ValidatorStake:  stakedAmount,
	}, ts.Nodes()...)

	// transition a delegation output to a delayed claiming state
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
		ValidatorStake:  stakedAmount,
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
				120,
			),
		),
	)
	defer ts.Shutdown()

	// Add a validator node to the network. This will add a validator account to the snapshot.
	node1 := ts.AddValidatorNode("node1")
	// Add a non-validator node to the network. This will not add any accounts to the snapshot.
	_ = ts.AddNode("node2")
	// Add a default block issuer to the network. This will add another block issuer account to the snapshot.
	wallet := ts.AddDefaultWallet(node1)

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
		ValidatorStake:  mock.MinValidatorAccountAmount(ts.API.ProtocolParameters()),
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

	// TRANSITION IMPLICIT ACCOUNT TO ACCOUNT OUTPUT.
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
		mock.WithAccountAmount(mock.MinIssuerAccountAmount(ts.API.ProtocolParameters())),
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

func Test_NegativeBIC_BlockIssuerLocked(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				iotago.SlotIndex(0),
				testsuite.GenesisTimeWithOffsetBySlots(iotago.SlotIndex(200), testsuite.DefaultSlotDurationInSeconds),
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

	// Add a validator node to the network. This will add a validator account to the snapshot.
	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddNode("node2")

	wallet1BIC := iotago.BlockIssuanceCredits(100000)
	wallet2BIC := iotago.BlockIssuanceCredits(0)

	wallet1 := ts.AddGenesisWallet("wallet 1", node2, testsuite.WithWalletBlockIssuanceCredits(wallet1BIC))
	wallet2 := ts.AddGenesisWallet("wallet 2", node2, testsuite.WithWalletBlockIssuanceCredits(wallet2BIC))

	ts.Run(false)

	// check that the accounts added in the genesis snapshot were added to the account manager correctly.

	// wallet 1 block issuer account.
	wallet1OutputID := ts.AccountOutput("Genesis:2").OutputID()
	ts.AssertAccountData(&accounts.AccountData{
		ID:              wallet1.BlockIssuer.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(wallet1BIC, 0),
		OutputID:        wallet1OutputID,
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: wallet1.BlockIssuer.BlockIssuerKeys(),
	}, ts.Nodes()...)

	// wallet 2 block issuer account.
	wallet2OutputID := ts.AccountOutput("Genesis:3").OutputID()
	ts.AssertAccountData(&accounts.AccountData{
		ID:              wallet2.BlockIssuer.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(wallet2BIC, 0),
		OutputID:        wallet2OutputID,
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: wallet2.BlockIssuer.BlockIssuerKeys(),
	}, ts.Nodes()...)

	// MODIFY EXISTING GENESIS ACCOUNT
	var block1Slot iotago.SlotIndex = 1
	var latestParents []iotago.BlockID
	// Issue one block from each of the two block-issuers - one will go negative and the other has enough BICs.
	{
		block1Commitment := iotago.NewEmptyCommitment(ts.API)
		block1Commitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
		ts.SetCurrentSlot(block1Slot)
		block11 := ts.IssueBasicBlockWithOptions("block1.1", wallet1, &iotago.TaggedData{}, mock.WithSlotCommitment(block1Commitment))
		block12 := ts.IssueBasicBlockWithOptions("block1.2", wallet2, &iotago.TaggedData{}, mock.WithStrongParents(block11.ID()), mock.WithSlotCommitment(block1Commitment))

		// Commit BIC burns and check account states.
		latestParents = ts.CommitUntilSlot(ts.BlockID("block1.2").Slot(), block12.ID())

		burned := iotago.BlockIssuanceCredits(block11.WorkScore()) * iotago.BlockIssuanceCredits(ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost)

		wallet1BIC -= burned
		wallet2BIC -= burned

		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet1.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet1BIC, block1Slot),
			OutputID:        wallet1OutputID,
			ExpirySlot:      iotago.MaxSlotIndex,
			BlockIssuerKeys: wallet1.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)

		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet2.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet2BIC, block1Slot),
			ExpirySlot:      iotago.MaxSlotIndex,
			OutputID:        wallet2OutputID,
			BlockIssuerKeys: wallet2.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)
	}

	block2Slot := ts.CurrentSlot()

	// Try to issue more blocks from each of the issuers - one succeeds in issuing a block,
	// the other has the block rejected in the CommitmentFilter as his account has negative BIC value.
	{
		block2Commitment := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()

		block21 := ts.IssueBasicBlockWithOptions("block2.1", wallet1, &iotago.TaggedData{}, mock.WithSlotCommitment(block2Commitment))

		block22 := ts.IssueBasicBlockWithOptions("block2.2", wallet2, &iotago.TaggedData{}, mock.WithStrongParents(ts.BlockID("block2.1")), mock.WithSlotCommitment(block2Commitment))

		ts.AssertBlockFiltered([]*blocks.Block{block22}, iotago.ErrNegativeBIC, wallet2.Node)

		latestParents = ts.CommitUntilSlot(ts.BlockID("block2.1").Slot(), block21.ID())

		burned := iotago.BlockIssuanceCredits(block21.WorkScore()) * iotago.BlockIssuanceCredits(ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost)

		wallet1BIC -= burned

		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet1.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet1BIC, block2Slot),
			OutputID:        wallet1OutputID,
			ExpirySlot:      iotago.MaxSlotIndex,
			BlockIssuerKeys: wallet1.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)

		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet2.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet2BIC, block1Slot),
			ExpirySlot:      iotago.MaxSlotIndex,
			OutputID:        wallet2OutputID,
			BlockIssuerKeys: wallet2.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)
	}

	block3Slot := ts.CurrentSlot()

	// Allot some mana to the locked account to unlock it.
	// The locked wallet 2 is preparing and signs the transaction, but it's issued by wallet 1 whose account is not locked.
	{
		allottedBIC := iotago.BlockIssuanceCredits(1000)
		tx1 := wallet2.AllotManaFromInputs("TX1",
			iotago.Allotments{&iotago.Allotment{
				AccountID: wallet2.BlockIssuer.AccountID,
				Mana:      iotago.Mana(allottedBIC),
			}}, "Genesis:0")

		block3Commitment := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()
		// Wallet 1 whose account is not locked is issuing the block to unlock the account of wallet 2.
		block31 := ts.IssueBasicBlockWithOptions("block3.1", wallet1, tx1, mock.WithStrongParents(latestParents...), mock.WithSlotCommitment(block3Commitment))

		latestParents = ts.CommitUntilSlot(ts.BlockID("block3.1").Slot(), block31.ID())

		burned := iotago.BlockIssuanceCredits(block31.WorkScore()) * iotago.BlockIssuanceCredits(ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost)

		wallet1BIC -= burned
		wallet2BIC += allottedBIC

		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet1.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet1BIC, block3Slot),
			OutputID:        wallet1OutputID,
			ExpirySlot:      iotago.MaxSlotIndex,
			BlockIssuerKeys: wallet1.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)

		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet2.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet2BIC, block3Slot),
			ExpirySlot:      iotago.MaxSlotIndex,
			OutputID:        wallet2OutputID,
			BlockIssuerKeys: wallet2.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)
	}

	block4Slot := ts.CurrentSlot()

	// Issue block from the unlocked account of wallet 2 to make sure that it's actually unlocked.
	{
		block4Commitment := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()

		block4 := ts.IssueBasicBlockWithOptions("block4", wallet2, &iotago.TaggedData{}, mock.WithStrongParents(latestParents...), mock.WithSlotCommitment(block4Commitment))

		latestParents = ts.CommitUntilSlot(ts.BlockID("block4").Slot(), block4.ID())

		burned := iotago.BlockIssuanceCredits(block4.WorkScore()) * iotago.BlockIssuanceCredits(ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost)

		wallet2BIC -= burned

		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet1.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet1BIC, block3Slot),
			OutputID:        wallet1OutputID,
			ExpirySlot:      iotago.MaxSlotIndex,
			BlockIssuerKeys: wallet1.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)

		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet2.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet2BIC, block4Slot),
			ExpirySlot:      iotago.MaxSlotIndex,
			OutputID:        wallet2OutputID,
			BlockIssuerKeys: wallet2.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)
	}
}

func Test_NegativeBIC_AccountOutput(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
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

	// Add a validator node to the network. This will add a validator account to the snapshot.
	node1 := ts.AddValidatorNode("node1")
	_ = ts.AddNode("node2")

	wallet1BIC := iotago.BlockIssuanceCredits(-1)
	wallet2BIC := iotago.MaxBlockIssuanceCredits / 2

	// Add a default block issuer to the network. This will add another block issuer account to the snapshot.
	minDeposit, err := depositcalculator.MinDeposit(ts.API.ProtocolParameters(), iotago.OutputAccount,
		depositcalculator.WithAddress(&iotago.Ed25519Address{}),
		depositcalculator.WithBlockIssuerKeys(2),
	)
	require.NoError(t, err)

	wallet1 := ts.AddGenesisWallet("wallet 1", node1, testsuite.WithWalletAmount(minDeposit), testsuite.WithWalletBlockIssuanceCredits(wallet1BIC))
	wallet2 := ts.AddGenesisWallet("wallet 2", node1, testsuite.WithWalletBlockIssuanceCredits(wallet2BIC))

	ts.Run(true)

	// check that the accounts added in the genesis snapshot were added to the account manager correctly.
	// validator node account.
	validatorAccountOutput := ts.AccountOutput("Genesis:1")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              node1.Validator.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.MaxBlockIssuanceCredits/2, 0),
		OutputID:        validatorAccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: node1.Validator.BlockIssuerKeys(),
		StakeEndEpoch:   iotago.MaxEpochIndex,
		ValidatorStake:  mock.MinValidatorAccountAmount(ts.API.ProtocolParameters()),
	}, ts.Nodes()...)

	// default wallet block issuer account.
	wallet1AccountOutput := ts.AccountOutput("Genesis:2")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              wallet1.BlockIssuer.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(-1, 0),
		OutputID:        wallet1AccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: wallet1.BlockIssuer.BlockIssuerKeys(),
	}, ts.Nodes()...)

	wallet2AccountOutput := ts.AccountOutput("Genesis:3")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              wallet2.BlockIssuer.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.MaxBlockIssuanceCredits/2, 0),
		OutputID:        wallet2AccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: wallet2.BlockIssuer.BlockIssuerKeys(),
	}, ts.Nodes()...)

	// MODIFY EXISTING GENESIS ACCOUNT
	newWallet1IssuerKey := utils.RandBlockIssuerKey()
	var block1Slot iotago.SlotIndex = 1
	var latestParents []iotago.BlockID
	ts.SetCurrentSlot(block1Slot)

	// set the expiry of the genesis account to be the block slot + max committable age.
	newExpirySlot := block1Slot + ts.API.ProtocolParameters().MaxCommittableAge()
	{
		// Prepare a transaction that will try to spend an AccountOutput of a locked account.
		tx1 := wallet1.TransitionAccount(
			"TX1",
			"Genesis:2",
			mock.WithAddBlockIssuerKey(newWallet1IssuerKey),
			mock.WithBlockIssuerExpirySlot(newExpirySlot),
		)

		// default block issuer issues a block containing the transaction in slot 1.
		genesisCommitment := iotago.NewEmptyCommitment(ts.API)
		genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost

		// Wallet 2, which has non-negative BIC issues the block.
		ts.IssueBasicBlockWithOptions("block1", wallet2, tx1, mock.WithSlotCommitment(genesisCommitment))

		ts.AssertTransactionsInCacheInvalid([]*iotago.Transaction{tx1.Transaction}, true, node1)

		latestParents = ts.CommitUntilSlot(block1Slot, ts.BlockIDs("Genesis")...)

		// The outputID of wallet1 and wallet2 account should remain the same as neither was successfully spent.
		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet1.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet1BIC, 0),
			OutputID:        wallet1AccountOutput.OutputID(),
			ExpirySlot:      iotago.MaxSlotIndex,
			BlockIssuerKeys: wallet1.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)

		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet2.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet2BIC, 0),
			OutputID:        wallet2AccountOutput.OutputID(),
			ExpirySlot:      iotago.MaxSlotIndex,
			BlockIssuerKeys: wallet2.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)
	}

	block2Slot := ts.CurrentSlot()

	// Allot some mana to the locked account to unlock it.
	// The locked wallet 1 is preparing and signs the transaction, but it's issued by wallet 2 whose account is not locked.
	{
		allottedBIC := iotago.BlockIssuanceCredits(10001)
		tx2 := wallet1.AllotManaFromInputs("TX2",
			iotago.Allotments{&iotago.Allotment{
				AccountID: wallet1.BlockIssuer.AccountID,
				Mana:      iotago.Mana(allottedBIC),
			}}, "Genesis:0")

		block2Commitment := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()
		// Wallet 2 whose account is not locked is issuing the block to unlock the account of wallet 1.
		block2 := ts.IssueBasicBlockWithOptions("block2", wallet2, tx2, mock.WithStrongParents(latestParents...), mock.WithSlotCommitment(block2Commitment))

		latestParents = ts.CommitUntilSlot(block2Slot, block2.ID())

		wallet1BIC += allottedBIC
		wallet2BIC -= iotago.BlockIssuanceCredits(block2.WorkScore()) * iotago.BlockIssuanceCredits(ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost)

		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet1.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet1BIC, block2Slot),
			OutputID:        wallet1AccountOutput.OutputID(),
			ExpirySlot:      iotago.MaxSlotIndex,
			BlockIssuerKeys: wallet1.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)

		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet2.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet2BIC, block2Slot),
			ExpirySlot:      iotago.MaxSlotIndex,
			OutputID:        wallet2AccountOutput.OutputID(),
			BlockIssuerKeys: wallet2.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)
	}

	block3Slot := ts.CurrentSlot()
	newExpirySlot = block3Slot + ts.API.ProtocolParameters().MaxCommittableAge()
	{
		// Prepare a transaction that will try to spend an AccountOutput of an already unlocked account.
		tx3 := wallet1.TransitionAccount(
			"TX3",
			"Genesis:2",
			mock.WithAddBlockIssuerKey(newWallet1IssuerKey),
			mock.WithBlockIssuerExpirySlot(newExpirySlot),
		)

		block3Commitment := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()

		// Wallet 1, which already has non-negative BIC issues the block.
		block3 := ts.IssueBasicBlockWithOptions("block3", wallet1, tx3, mock.WithStrongParents(latestParents...), mock.WithSlotCommitment(block3Commitment))
		latestParents = ts.CommitUntilSlot(block3Slot, block3.ID())

		wallet1BIC -= iotago.BlockIssuanceCredits(block3.WorkScore()) * iotago.BlockIssuanceCredits(ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost)

		// The outputID of wallet1 and wallet2 account should remain the same as neither was successfully spent.
		// The mana on wallet2 account should be subtracted
		// because it issued the block with a transaction that didn't mutate the ledger.
		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet1.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet1BIC, block3Slot),
			OutputID:        wallet1.AccountOutput("TX3:0").OutputID(),
			ExpirySlot:      newExpirySlot,
			BlockIssuerKeys: iotago.NewBlockIssuerKeys(wallet1.BlockIssuer.BlockIssuerKeys()[0], newWallet1IssuerKey),
		}, ts.Nodes()...)

		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet2.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet2BIC, block2Slot),
			OutputID:        wallet2AccountOutput.OutputID(),
			ExpirySlot:      iotago.MaxSlotIndex,
			BlockIssuerKeys: wallet2.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)
	}

	// DESTROY WALLET 1 ACCOUNT
	{
		// commit until the expiry slot of the transitioned genesis account plus one.
		latestParents = ts.CommitUntilSlot(newExpirySlot+1, latestParents...)

		block4Slot := ts.CurrentSlot()

		// create a transaction which destroys the genesis account.

		tx4 := wallet1.DestroyAccount("TX4", "TX3:0")
		block4Commitment := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()

		block4 := ts.IssueBasicBlockWithOptions("block4", wallet2, tx4, mock.WithStrongParents(latestParents...), mock.WithSlotCommitment(block4Commitment))
		latestParents = ts.CommitUntilSlot(block4Slot, block4.ID())

		// assert diff of the destroyed account.
		ts.AssertAccountDiff(wallet1.BlockIssuer.AccountID, block4Slot, &model.AccountDiff{
			BICChange:              -iotago.BlockIssuanceCredits(9500),
			PreviousUpdatedSlot:    21,
			NewExpirySlot:          0,
			PreviousExpirySlot:     newExpirySlot,
			NewOutputID:            iotago.EmptyOutputID,
			PreviousOutputID:       wallet1.Output("TX3:0").OutputID(),
			BlockIssuerKeysAdded:   iotago.NewBlockIssuerKeys(),
			BlockIssuerKeysRemoved: iotago.NewBlockIssuerKeys(wallet1.BlockIssuer.BlockIssuerKeys()[0], newWallet1IssuerKey),
			ValidatorStakeChange:   0,
			StakeEndEpochChange:    0,
			FixedCostChange:        0,
			DelegationStakeChange:  0,
		}, true, ts.Nodes()...)
	}
}

func Test_NegativeBIC_AccountOwnedBasicOutputLocked(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
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

	// Add a validator node to the network. This will add a validator account to the snapshot.
	node1 := ts.AddValidatorNode("node1")
	_ = ts.AddNode("node2")

	wallet1BIC := iotago.BlockIssuanceCredits(-1)
	wallet2BIC := iotago.MaxBlockIssuanceCredits / 2
	// Add a default block issuer to the network. This will add another block issuer account to the snapshot.
	wallet1 := ts.AddGenesisWallet("wallet 1", node1, testsuite.WithWalletBlockIssuanceCredits(wallet1BIC))
	wallet2 := ts.AddGenesisWallet("wallet 2", node1, testsuite.WithWalletBlockIssuanceCredits(wallet2BIC))

	ts.Run(true)

	// check that the accounts added in the genesis snapshot were added to the account manager correctly.
	// validator node account.
	validatorAccountOutput := ts.AccountOutput("Genesis:1")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              node1.Validator.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(iotago.MaxBlockIssuanceCredits/2, 0),
		OutputID:        validatorAccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: node1.Validator.BlockIssuerKeys(),
		StakeEndEpoch:   iotago.MaxEpochIndex,
		ValidatorStake:  mock.MinValidatorAccountAmount(ts.API.ProtocolParameters()),
	}, ts.Nodes()...)

	// default wallet block issuer account.
	wallet1AccountOutput := ts.AccountOutput("Genesis:2")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              wallet1.BlockIssuer.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(wallet1BIC, 0),
		OutputID:        wallet1AccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: wallet1.BlockIssuer.BlockIssuerKeys(),
	}, ts.Nodes()...)

	wallet2AccountOutput := ts.AccountOutput("Genesis:3")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              wallet2.BlockIssuer.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(wallet2BIC, 0),
		OutputID:        wallet2AccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: wallet2.BlockIssuer.BlockIssuerKeys(),
	}, ts.Nodes()...)

	var block1Slot iotago.SlotIndex = 1
	var latestParents []iotago.BlockID
	ts.SetCurrentSlot(block1Slot)

	genesisCommitment := iotago.NewEmptyCommitment(ts.API)
	genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost

	tx0 := wallet1.CreateBasicOutputsEquallyFromInput(
		"TX0",
		2,
		"Genesis:0",
	)
	block0 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx0, mock.WithSlotCommitment(genesisCommitment))

	// SEND A TRANSACTION TO AN ACCOUNT ADDRESS
	{
		// Prepare a transaction that will try to spend an AccountOutput of a locked account.
		tx1 := wallet1.SendFundsToAccount(
			"TX1",
			wallet1.BlockIssuer.AccountID,
			"TX0:0",
		)

		// default block issuer issues a block containing the transaction in slot 1.

		// Wallet 2, which has non-negative BIC issues the block.
		block1 := ts.IssueBasicBlockWithOptions("block1", wallet2, tx1, mock.WithStrongParents(block0.ID()), mock.WithSlotCommitment(genesisCommitment))

		ts.AssertTransactionsInCacheBooked([]*iotago.Transaction{tx1.Transaction}, true, node1)

		latestParents = ts.CommitUntilSlot(block1Slot, block1.ID())

		wallet2BIC -= iotago.BlockIssuanceCredits(block1.WorkScore()) * iotago.BlockIssuanceCredits(ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost)

		// The outputID of wallet1 and wallet2 account should remain the same as neither was successfully spent.
		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet1.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet1BIC, 0),
			OutputID:        wallet1AccountOutput.OutputID(),
			ExpirySlot:      iotago.MaxSlotIndex,
			BlockIssuerKeys: wallet1.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)

		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet2.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet2BIC, block1Slot),
			OutputID:        wallet2AccountOutput.OutputID(),
			ExpirySlot:      iotago.MaxSlotIndex,
			BlockIssuerKeys: wallet2.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)
	}

	block2Slot := ts.CurrentSlot()

	//TRY TO SPEND THE BASIC OUTPUT FROM AN ACCOUNT ADDRESS
	{
		block2Commitment := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()

		tx2 := wallet1.SendFundsFromAccount(
			"TX2",
			"Genesis:2",
			lo.PanicOnErr(block2Commitment.ID()),
			"TX1:0",
		)

		// Wallet 2, which has non-negative BIC issues the block.
		ts.IssueBasicBlockWithOptions("block2", wallet2, tx2, mock.WithStrongParents(latestParents...), mock.WithSlotCommitment(block2Commitment))

		ts.AssertTransactionsInCacheInvalid([]*iotago.Transaction{tx2.Transaction}, true, node1)

		latestParents = ts.CommitUntilSlot(block2Slot, latestParents...)

		// The outputID of wallet1 and wallet2 account should remain the same as neither was successfully spent.
		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet1.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet1BIC, 0),
			OutputID:        wallet1AccountOutput.OutputID(),
			ExpirySlot:      iotago.MaxSlotIndex,
			BlockIssuerKeys: wallet1.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)

		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet2.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet2BIC, block1Slot),
			OutputID:        wallet2AccountOutput.OutputID(),
			ExpirySlot:      iotago.MaxSlotIndex,
			BlockIssuerKeys: wallet2.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)
	}

	block3Slot := ts.CurrentSlot()

	// UNLOCK THE ACCOUNT
	// The locked wallet 2 is preparing and signs the transaction,
	// but it's issued by wallet 2 whose account is not locked.
	{
		allottedBIC := iotago.BlockIssuanceCredits(10001)
		tx3 := wallet1.AllotManaFromInputs("TX3",
			iotago.Allotments{&iotago.Allotment{
				AccountID: wallet1.BlockIssuer.AccountID,
				Mana:      iotago.Mana(allottedBIC),
			}}, "TX0:1")

		block3Commitment := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()

		// Wallet 2 whose account is not locked is issuing the block to unlock the account of wallet 1.
		block3 := ts.IssueBasicBlockWithOptions("block3", wallet2, tx3, mock.WithStrongParents(latestParents...), mock.WithSlotCommitment(block3Commitment))

		ts.AssertTransactionsInCacheBooked([]*iotago.Transaction{tx3.Transaction}, true, ts.Nodes()...)
		ts.AssertBlocksInCacheBooked([]*blocks.Block{block3}, true, ts.Nodes()...)

		latestParents = ts.CommitUntilSlot(block3Slot, block3.ID())

		wallet1BIC += allottedBIC
		wallet2BIC -= iotago.BlockIssuanceCredits(block3.WorkScore()) * iotago.BlockIssuanceCredits(ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost)
		require.GreaterOrEqual(t, wallet1BIC, iotago.BlockIssuanceCredits(0))

		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet1.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet1BIC, block3Slot),
			OutputID:        wallet1AccountOutput.OutputID(),
			ExpirySlot:      iotago.MaxSlotIndex,
			BlockIssuerKeys: wallet1.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)

		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet2.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet2BIC, block3Slot),
			ExpirySlot:      iotago.MaxSlotIndex,
			OutputID:        wallet2AccountOutput.OutputID(),
			BlockIssuerKeys: wallet2.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)
	}

	block4Slot := ts.CurrentSlot()
	// SPEND THE BASIC OUTPUT FROM AN ACCOUNT ADDRESS
	{
		block4Commitment := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()

		tx4 := wallet1.SendFundsFromAccount(
			"TX4",
			"Genesis:2",
			lo.PanicOnErr(block4Commitment.ID()),
			"TX1:0",
		)

		// Wallet 1, which has non-negative BIC issues the block.
		block4 := ts.IssueBasicBlockWithOptions("block4", wallet1, tx4, mock.WithStrongParents(latestParents...), mock.WithSlotCommitment(block4Commitment))

		ts.AssertTransactionsInCacheBooked([]*iotago.Transaction{tx4.Transaction}, true, node1)

		latestParents = ts.CommitUntilSlot(block4Slot, block4.ID())

		wallet1BIC -= iotago.BlockIssuanceCredits(block4.WorkScore()) * iotago.BlockIssuanceCredits(ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost)

		// The outputID of wallet1 and wallet2 account should remain the same as neither was successfully spent.
		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet1.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet1BIC, block4Slot),
			OutputID:        wallet1.Output("TX4:0").OutputID(),
			ExpirySlot:      iotago.MaxSlotIndex,
			BlockIssuerKeys: wallet1.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)

		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet2.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(wallet2BIC, block3Slot),
			OutputID:        wallet2AccountOutput.OutputID(),
			ExpirySlot:      iotago.MaxSlotIndex,
			BlockIssuerKeys: wallet2.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)
	}
}
