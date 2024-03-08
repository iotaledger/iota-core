package tests

import (
	"testing"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

// Test_AccountStateTransition follows the account state transition flow described in:
// https://github.com/iotaledger/iota-core/issues/660#issuecomment-1892596243
func Test_AccountStateTransition(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(200, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				testsuite.DefaultSlotsPerEpochExponent,
			),
		),
	)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddValidatorNode("node2")
	wallet := ts.AddDefaultWallet(node1)

	ts.Run(true)

	// split genesis output into 4 outputs for the further usage.
	tx1 := wallet.CreateBasicOutputsEquallyFromInput("TX1", 4, "Genesis:0")
	ts.IssueBasicBlockWithOptions("block0", wallet, tx1)

	// Issue some more blocks to make transaction accepted
	{
		ts.IssueValidationBlockWithHeaderOptions("vblock0", node2, mock.WithStrongParents(ts.BlockID("block0")))
		ts.IssueValidationBlockWithHeaderOptions("vblock1", node1, mock.WithStrongParents(ts.BlockID("vblock0")))
		ts.IssueValidationBlockWithHeaderOptions("vblock2", node2, mock.WithStrongParents(ts.BlockID("vblock1")))

		ts.AssertTransactionsInCacheAccepted(wallet.Transactions("TX1"), true, node1, node2)
	}

	// create the account1 from TX1:0 with wallet "first"
	// generated (block1, TX2)
	ts.AddWallet("first", node1, iotago.EmptyAccountID)
	createFullAccount(ts)

	// create the account2, from implicit to full account from TX1:1 with wallet "second"
	// generated (block2, TX3), (block3, TX4)
	ts.AddWallet("second", node1, iotago.EmptyAccountID)
	account2ID := createImplicitToFullAccount(ts)

	// send funds to account2, with TX1:2
	// generated (block4, TX5)
	sendFunds(ts)

	// allot 1000 mana to account2 with TX1:3
	// generated (block5, TX6)
	allotManaTo(ts, account2ID)

	// create native token from "TX5:0" and account2 (TX4:0)
	// generated (block6, TX7)
	createNativetoken(ts)
}

func createFullAccount(ts *testsuite.TestSuite) iotago.AccountID {
	node1 := ts.Node("node1")
	newUserWallet := ts.Wallet("first")

	// CREATE NEW ACCOUNT WITH BLOCK ISSUER FROM BASIC UTXO
	newAccountBlockIssuerKey := tpkg.RandBlockIssuerKey()
	// set the expiry slot of the transitioned genesis account to the latest committed + MaxCommittableAge
	newAccountExpirySlot := node1.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Slot() + ts.API.ProtocolParameters().MaxCommittableAge()

	tx1 := ts.DefaultWallet().CreateAccountFromInput(
		"TX2",
		"TX1:0",
		newUserWallet,
		mock.WithBlockIssuerFeature(iotago.BlockIssuerKeys{newAccountBlockIssuerKey}, newAccountExpirySlot),
		mock.WithAccountAmount(mock.MinIssuerAccountAmount(ts.API.ProtocolParameters())),
		mock.WithAccountMana(mock.MaxBlockManaCost(ts.DefaultWallet().Client.CommittedAPI().ProtocolParameters())),
	)

	block1 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1))
	var block1Slot iotago.SlotIndex = block1.ID().Slot()

	ts.CommitUntilSlot(block1Slot, block1.ID())

	newAccount := newUserWallet.AccountOutputData("TX2:0")
	newAccountOutput := newAccount.Output.(*iotago.AccountOutput)

	ts.AssertAccountDiff(newAccountOutput.AccountID, block1Slot, &model.AccountDiff{
		BICChange:              0,
		PreviousUpdatedSlot:    0,
		NewExpirySlot:          newAccountExpirySlot,
		PreviousExpirySlot:     0,
		NewOutputID:            newAccount.ID,
		PreviousOutputID:       iotago.EmptyOutputID,
		BlockIssuerKeysAdded:   iotago.NewBlockIssuerKeys(newAccountBlockIssuerKey),
		BlockIssuerKeysRemoved: iotago.NewBlockIssuerKeys(),
		ValidatorStakeChange:   0,
		StakeEndEpochChange:    0,
		FixedCostChange:        0,
		DelegationStakeChange:  0,
	}, false, ts.Nodes()...)

	ts.AssertAccountData(&accounts.AccountData{
		ID:              newAccountOutput.AccountID,
		Credits:         accounts.NewBlockIssuanceCredits(0, block1Slot),
		ExpirySlot:      newAccountExpirySlot,
		OutputID:        newAccount.ID,
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(newAccountBlockIssuerKey),
	}, ts.Nodes()...)

	return newAccountOutput.AccountID
}

func createImplicitToFullAccount(ts *testsuite.TestSuite) iotago.AccountID {
	node1 := ts.Node("node1")
	newUserWallet := ts.Wallet("second")

	// CREATE IMPLICIT ACCOUNT FROM GENESIS BASIC UTXO, SENT TO A NEW USER WALLET.
	// a default wallet, already registered in the ledger, will issue the transaction and block.
	tx3 := ts.DefaultWallet().CreateImplicitAccountAndBasicOutputFromInput(
		"TX3",
		"TX1:1",
		newUserWallet,
	)
	block2 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block2", ts.DefaultWallet(), tx3))
	block2Slot := block2.ID().Slot()
	latestParents := ts.CommitUntilSlot(block2Slot, block2.ID())

	implicitAccountOutput := newUserWallet.OutputData("TX3:0")
	implicitAccountOutputID := implicitAccountOutput.ID
	implicitAccountID := iotago.AccountIDFromOutputID(implicitAccountOutputID)
	var implicitBlockIssuerKey iotago.BlockIssuerKey = iotago.Ed25519PublicKeyHashBlockIssuerKeyFromImplicitAccountCreationAddress(newUserWallet.ImplicitAccountCreationAddress())

	// the new implicit account should now be registered in the accounts ledger.
	ts.AssertAccountData(&accounts.AccountData{
		ID:              implicitAccountID,
		Credits:         accounts.NewBlockIssuanceCredits(0, block2Slot),
		ExpirySlot:      iotago.MaxSlotIndex,
		OutputID:        implicitAccountOutputID,
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(implicitBlockIssuerKey),
	}, ts.Nodes()...)

	// TRANSITION IMPLICIT ACCOUNT TO ACCOUNT OUTPUT.
	block3Slot := ts.CurrentSlot()
	tx4 := newUserWallet.TransitionImplicitAccountToAccountOutput(
		"TX4",
		[]string{"TX3:0", "TX3:1"},
		mock.WithBlockIssuerFeature(
			iotago.BlockIssuerKeys{implicitBlockIssuerKey},
			iotago.MaxSlotIndex,
		),
	)
	block2Commitment := node1.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment()
	block3 := lo.PanicOnErr(ts.IssueBasicBlockWithOptions("block3", newUserWallet, tx4, mock.WithStrongParents(latestParents...)))
	latestParents = ts.CommitUntilSlot(block3Slot, block3.ID())

	fullAccountOutputID := newUserWallet.OutputData("TX4:0").ID
	allotted := iotago.BlockIssuanceCredits(tx4.Transaction.Allotments.Get(implicitAccountID))
	burned := iotago.BlockIssuanceCredits(block3.WorkScore()) * iotago.BlockIssuanceCredits(block2Commitment.ReferenceManaCost)
	// the implicit account should now have been transitioned to a full account in the accounts ledger.
	ts.AssertAccountDiff(implicitAccountID, block3Slot, &model.AccountDiff{
		BICChange:             allotted - burned,
		PreviousUpdatedSlot:   block2Slot,
		NewOutputID:           fullAccountOutputID,
		PreviousOutputID:      implicitAccountOutputID,
		PreviousExpirySlot:    iotago.MaxSlotIndex,
		NewExpirySlot:         iotago.MaxSlotIndex,
		ValidatorStakeChange:  0,
		StakeEndEpochChange:   0,
		FixedCostChange:       0,
		DelegationStakeChange: 0,
	}, false, ts.Nodes()...)

	ts.AssertAccountData(&accounts.AccountData{
		ID:              implicitAccountID,
		Credits:         accounts.NewBlockIssuanceCredits(allotted-burned, block3Slot),
		ExpirySlot:      iotago.MaxSlotIndex,
		OutputID:        fullAccountOutputID,
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(implicitBlockIssuerKey),
	}, ts.Nodes()...)

	return implicitAccountID
}

func sendFunds(ts *testsuite.TestSuite) {
	node1 := ts.Node("node1")
	node2 := ts.Node("node2")
	wallet := ts.DefaultWallet()
	secondWallet := ts.Wallet("second")

	// send funds from defaultWallet to secondWallet
	tx := wallet.SendFundsToWallet("TX5", secondWallet, "TX1:2")
	ts.IssueBasicBlockWithOptions("block4", wallet, tx)

	ts.AssertTransactionsExist(wallet.Transactions("TX5"), true, node1)
	ts.AssertTransactionsInCacheBooked(wallet.Transactions("TX5"), true, node1)

	// Issue some more blocks to make transaction accepted
	{
		ts.IssueValidationBlockWithHeaderOptions("vblock9", node2, mock.WithStrongParents(ts.BlockID("block4")))
		ts.IssueValidationBlockWithHeaderOptions("vblock10", node1, mock.WithStrongParents(ts.BlockID("vblock9")))
		ts.IssueValidationBlockWithHeaderOptions("vblock11", node2, mock.WithStrongParents(ts.BlockID("vblock10")))

		ts.AssertTransactionsInCacheAccepted(wallet.Transactions("TX5"), true, node1, node2)
	}
}

func allotManaTo(ts *testsuite.TestSuite, to iotago.AccountID) {
	wallet := ts.DefaultWallet()
	node1 := ts.Node("node1")
	node2 := ts.Node("node2")

	tx6 := wallet.AllotManaFromInputs("TX6",
		iotago.Allotments{&iotago.Allotment{
			AccountID: to,
			Mana:      iotago.Mana(1000),
		}}, "TX1:3")
	commitment := node1.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment()
	ts.IssueBasicBlockWithOptions("block5", wallet, tx6, mock.WithSlotCommitment(commitment))

	// Issue some more blocks to make transaction accepted
	{
		ts.IssueValidationBlockWithHeaderOptions("vblock6", node2, mock.WithStrongParents(ts.BlockID("block5")))
		ts.IssueValidationBlockWithHeaderOptions("vblock7", node1, mock.WithStrongParents(ts.BlockID("vblock6")))
		ts.IssueValidationBlockWithHeaderOptions("vblock8", node2, mock.WithStrongParents(ts.BlockID("vblock7")))

		ts.AssertTransactionsInCacheAccepted(wallet.Transactions("TX6"), true, node1, node2)
	}
}

// createNativetoken creates a native token from the given input and account.
func createNativetoken(ts *testsuite.TestSuite) {
	wallet := ts.Wallet("second")
	node1 := ts.Node("node1")
	node2 := ts.Node("node2")

	tx := wallet.CreateNativeTokenFromInput("TX7", "TX5:0", "TX4:0", 5_000_000, 10_000_000_000)
	ts.IssueBasicBlockWithOptions("block6", wallet, tx)

	ts.AssertTransactionsExist(wallet.Transactions("TX7"), true, node1)
	ts.AssertTransactionsInCacheBooked(wallet.Transactions("TX7"), true, node1)

	// Issue some more blocks to make transaction accepted
	{
		ts.IssueValidationBlockWithHeaderOptions("vblock12", node2, mock.WithStrongParents(ts.BlockID("block6")))
		ts.IssueValidationBlockWithHeaderOptions("vblock13", node1, mock.WithStrongParents(ts.BlockID("vblock12")))
		ts.IssueValidationBlockWithHeaderOptions("vblock14", node2, mock.WithStrongParents(ts.BlockID("vblock13")))

		ts.AssertTransactionsInCacheAccepted(wallet.Transactions("TX7"), true, node1, node2)
	}
}
