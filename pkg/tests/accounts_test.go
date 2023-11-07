package tests

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
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
	// Add a non-validator node to the network. This will not add any accounts to the snapshot.
	_ = ts.AddNode("node2")
	// Add a default block issuer to the network. This will add another block issuer account to the snapshot.
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
	genesisCommitment := iotago.NewEmptyCommitment(ts.API.ProtocolParameters().Version())
	genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
	block1 := ts.IssueBasicBlockAtSlotWithOptions("block1", block1Slot, ts.DefaultWallet(), tx1, mock.WithSlotCommitment(genesisCommitment))
	latestParent := ts.CommitUntilSlot(ts.BlockID("block1").Slot(), block1)

	// assert diff of the genesis account, it should have a new output ID, new expiry slot and a new block issuer key.
	ts.AssertAccountDiff(genesisAccountOutput.AccountID, block1Slot, &model.AccountDiff{
		BICChange:              0,
		PreviousUpdatedTime:    0,
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
		PreviousUpdatedTime:    0,
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
	newAccountExpirySlot := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Slot() + ts.API.ProtocolParameters().MaxCommittableAge()

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

	genesisCommitment := iotago.NewEmptyCommitment(ts.API.ProtocolParameters().Version())
	genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
	block1 := ts.IssueBasicBlockAtSlotWithOptions("block1", block1Slot, ts.DefaultWallet(), tx1)
	latestParent := ts.CommitUntilSlot(block1Slot, block1)

	newAccount := ts.DefaultWallet().AccountOutput("TX1:0")
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

	// transition a delegation output to a delayed claiming state
	block3Slot := latestParent.ID().Slot()
	tx3 := ts.DefaultWallet().DelayedClaimingTransition("TX3", "TX2:0", block3Slot, 0)
	block3 := ts.IssueBasicBlockAtSlotWithOptions("block3", block3Slot, ts.DefaultWallet(), tx3, mock.WithStrongParents(latestParent.ID()))

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
	block2Commitment := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()
	block2 := ts.IssueBasicBlockAtSlotWithOptions("block2", block2Slot, newUserWallet, tx2, mock.WithStrongParents(latestParent.ID()))
	latestParent = ts.CommitUntilSlot(block2Slot, block2)

	fullAccountOutputID := newUserWallet.Output("TX2:0").OutputID()
	allotted := iotago.BlockIssuanceCredits(tx2.Transaction.Allotments.Get(implicitAccountID))
	burned := iotago.BlockIssuanceCredits(block2.WorkScore()) * iotago.BlockIssuanceCredits(block2Commitment.ReferenceManaCost)
	// the implicit account should now have been transitioned to a full account in the accounts ledger.
	ts.AssertAccountDiff(implicitAccountID, block2Slot, &model.AccountDiff{
		BICChange:              allotted - burned,
		PreviousUpdatedTime:    block1Slot,
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

func Test_NegativeBIC(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
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
	node2 := ts.AddNode("node2")

	validatorBIC := iotago.MaxBlockIssuanceCredits / 2
	// Add a default block issuer to the network. This will add another block issuer account to the snapshot.
	walletBIC := iotago.BlockIssuanceCredits(499)
	wallet := ts.AddGenesisWallet("default", node2, walletBIC)

	ts.Run(false)

	// check that the accounts added in the genesis snapshot were added to the account manager correctly.
	// validator node account.
	validatorAccountOutput := ts.AccountOutput("Genesis:1")
	ts.AssertAccountData(&accounts.AccountData{
		ID:              node1.Validator.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(validatorBIC, 0),
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
		Credits:         accounts.NewBlockIssuanceCredits(walletBIC, 0),
		OutputID:        blockIssuerAccountOutput.OutputID(),
		ExpirySlot:      iotago.MaxSlotIndex,
		BlockIssuerKeys: wallet.BlockIssuer.BlockIssuerKeys(),
	}, ts.Nodes()...)

	// MODIFY EXISTING GENESIS ACCOUNT
	var block1Slot iotago.SlotIndex = 1
	var latestParent *blocks.Block
	// Issue one block from each of the two block-issuers - one will go negative and the other has enough BICs.
	{
		block1Commitment := iotago.NewEmptyCommitment(ts.API.ProtocolParameters().Version())
		block1Commitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
		block11 := ts.IssueBasicBlockAtSlotWithOptions("block1.1", block1Slot, ts.Wallet("node1"), &iotago.TaggedData{}, mock.WithSlotCommitment(block1Commitment))
		block12 := ts.IssueBasicBlockAtSlotWithOptions("block1.2", block1Slot, wallet, &iotago.TaggedData{}, mock.WithStrongParents(block11.ID()), mock.WithSlotCommitment(block1Commitment))

		// Commit BIC burns and check account states.
		latestParent = ts.CommitUntilSlot(ts.BlockID("block1.2").Slot(), block12)

		burned := iotago.BlockIssuanceCredits(block11.WorkScore()) * iotago.BlockIssuanceCredits(ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost)

		validatorBIC -= burned
		walletBIC -= burned

		ts.AssertAccountData(&accounts.AccountData{
			ID:              node1.Validator.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(validatorBIC, block1Slot),
			OutputID:        validatorAccountOutput.OutputID(),
			ExpirySlot:      iotago.MaxSlotIndex,
			BlockIssuerKeys: node1.Validator.BlockIssuerKeys(),
			StakeEndEpoch:   iotago.MaxEpochIndex,
			ValidatorStake:  mock.MinValidatorAccountAmount,
		}, ts.Nodes()...)
		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(walletBIC, block1Slot),
			ExpirySlot:      iotago.MaxSlotIndex,
			OutputID:        blockIssuerAccountOutput.OutputID(),
			BlockIssuerKeys: wallet.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)
	}

	block2Slot := latestParent.ID().Slot()

	// Try to issue more blocks from each of the issuers - one succeeds in issuing a block,
	// the other has the block rejected in the CommitmentFilter as his account has negative BIC value.
	{

		block21 := ts.IssueBasicBlockAtSlotWithOptions("block2.1", block2Slot, ts.Wallet("node1"), &iotago.TaggedData{})

		block22 := ts.IssueBasicBlockAtSlotWithOptions("block2.2", block2Slot, wallet, &iotago.TaggedData{}, mock.WithStrongParents(ts.BlockID("block2.1")))

		ts.AssertBlockFiltered([]*blocks.Block{block22}, iotago.ErrNegativeBIC, wallet.Node)

		latestParent = ts.CommitUntilSlot(ts.BlockID("block2.1").Slot(), block21)

		burned := iotago.BlockIssuanceCredits(block21.WorkScore()) * iotago.BlockIssuanceCredits(ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost)

		validatorBIC -= burned

		ts.AssertAccountData(&accounts.AccountData{
			ID:              node1.Validator.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(validatorBIC, block2Slot),
			OutputID:        validatorAccountOutput.OutputID(),
			ExpirySlot:      iotago.MaxSlotIndex,
			BlockIssuerKeys: node1.Validator.BlockIssuerKeys(),
			StakeEndEpoch:   iotago.MaxEpochIndex,
			ValidatorStake:  mock.MinValidatorAccountAmount,
			LatestSupportedProtocolVersionAndHash: model.VersionAndHash{
				Version: ts.API.ProtocolParameters().Version(),
				Hash:    lo.PanicOnErr(ts.API.ProtocolParameters().Hash()),
			},
		}, ts.Nodes()...)
		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(walletBIC, block1Slot),
			ExpirySlot:      iotago.MaxSlotIndex,
			OutputID:        blockIssuerAccountOutput.OutputID(),
			BlockIssuerKeys: wallet.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)
	}

	block3Slot := latestParent.ID().Slot()

	// Allot some mana to the locked account to unlock it.
	{
		allottedBIC := iotago.BlockIssuanceCredits(500)
		tx1, err := wallet.CreateSignedTransactionWithOptions(
			"TX1",
			mock.WithInputs(utxoledger.Outputs{wallet.Output("Genesis:0")}),
			mock.WithOutputs(iotago.Outputs[iotago.Output]{wallet.Output("Genesis:0").Output()}),
			mock.WithSlotCreated(block3Slot),
			mock.WithAllotments(iotago.Allotments{&iotago.Allotment{
				AccountID: wallet.BlockIssuer.AccountID,
				Mana:      iotago.Mana(allottedBIC),
			}}),
		)
		require.NoError(t, err)

		block3Commitment := node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()
		block31 := ts.IssueBasicBlockAtSlotWithOptions("block3.1", block3Slot, ts.Wallet("node1"), tx1, mock.WithStrongParents(latestParent.ID()), mock.WithSlotCommitment(block3Commitment))

		latestParent = ts.CommitUntilSlot(ts.BlockID("block3.1").Slot(), block31)

		burned := iotago.BlockIssuanceCredits(block31.WorkScore()) * iotago.BlockIssuanceCredits(ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost)

		validatorBIC -= burned
		walletBIC += allottedBIC

		ts.AssertAccountData(&accounts.AccountData{
			ID:              node1.Validator.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(validatorBIC, block3Slot),
			OutputID:        validatorAccountOutput.OutputID(),
			ExpirySlot:      iotago.MaxSlotIndex,
			BlockIssuerKeys: node1.Validator.BlockIssuerKeys(),
			StakeEndEpoch:   iotago.MaxEpochIndex,
			ValidatorStake:  mock.MinValidatorAccountAmount,
			LatestSupportedProtocolVersionAndHash: model.VersionAndHash{
				Version: ts.API.ProtocolParameters().Version(),
				Hash:    lo.PanicOnErr(ts.API.ProtocolParameters().Hash()),
			},
		}, ts.Nodes()...)
		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(walletBIC, block3Slot),
			ExpirySlot:      iotago.MaxSlotIndex,
			OutputID:        blockIssuerAccountOutput.OutputID(),
			BlockIssuerKeys: wallet.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)
	}

	block4Slot := latestParent.ID().Slot()

	// Issue block from the unlocked account to make sure that it's actually unlocked.
	{

		block4 := ts.IssueBasicBlockAtSlotWithOptions("block4", block4Slot, wallet, &iotago.TaggedData{}, mock.WithStrongParents(latestParent.ID()))

		latestParent = ts.CommitUntilSlot(ts.BlockID("block4").Slot(), block4)

		burned := iotago.BlockIssuanceCredits(block4.WorkScore()) * iotago.BlockIssuanceCredits(ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost)

		walletBIC -= burned

		ts.AssertAccountData(&accounts.AccountData{
			ID:              node1.Validator.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(validatorBIC, block3Slot),
			OutputID:        validatorAccountOutput.OutputID(),
			ExpirySlot:      iotago.MaxSlotIndex,
			BlockIssuerKeys: node1.Validator.BlockIssuerKeys(),
			StakeEndEpoch:   iotago.MaxEpochIndex,
			ValidatorStake:  mock.MinValidatorAccountAmount,
			LatestSupportedProtocolVersionAndHash: model.VersionAndHash{
				Version: ts.API.ProtocolParameters().Version(),
				Hash:    lo.PanicOnErr(ts.API.ProtocolParameters().Hash()),
			},
		}, ts.Nodes()...)
		ts.AssertAccountData(&accounts.AccountData{
			ID:              wallet.BlockIssuer.AccountID,
			Credits:         accounts.NewBlockIssuanceCredits(walletBIC, block4Slot),
			ExpirySlot:      iotago.MaxSlotIndex,
			OutputID:        blockIssuerAccountOutput.OutputID(),
			BlockIssuerKeys: wallet.BlockIssuer.BlockIssuerKeys(),
		}, ts.Nodes()...)
	}
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
