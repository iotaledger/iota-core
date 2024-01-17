package tests

import (
	"testing"

	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/depositcalculator"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
	"github.com/stretchr/testify/require"
)

func createFullAccount(ts *testsuite.TestSuite) iotago.AccountID {
	node1 := ts.Node("node1")

	newUserWallet := ts.AddWallet("first", node1, iotago.EmptyAccountID)
	// CREATE NEW ACCOUNT WITH BLOCK ISSUER AND STAKING FEATURES FROM BASIC UTXO
	newAccountBlockIssuerKey := tpkg.RandBlockIssuerKey()
	// set the expiry slot of the transitioned genesis account to the latest committed + MaxCommittableAge
	newAccountExpirySlot := node1.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Slot() + ts.API.ProtocolParameters().MaxCommittableAge()

	stakedAmount := iotago.BaseToken(10000)

	validatorAccountAmount, err := depositcalculator.MinDeposit(ts.API.ProtocolParameters(), iotago.OutputAccount,
		depositcalculator.WithAddress(&iotago.Ed25519Address{}),
		depositcalculator.WithBlockIssuerKeys(1),
		depositcalculator.WithStakedAmount(stakedAmount),
	)
	require.NoError(ts.Testing, err)

	tx1 := ts.DefaultWallet().CreateAccountFromInput(
		"TX2",
		"TX1:0",
		newUserWallet,
		mock.WithBlockIssuerFeature(iotago.BlockIssuerKeys{newAccountBlockIssuerKey}, newAccountExpirySlot),
		mock.WithStakingFeature(stakedAmount, 421, 0, 10),
		mock.WithAccountAmount(validatorAccountAmount),
		mock.WithAccountMana(mock.MaxBlockManaCost(ts.DefaultWallet().Node.Protocol.CommittedAPI().ProtocolParameters())),
	)

	block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1)
	var block1Slot iotago.SlotIndex = block1.ID().Slot()

	ts.CommitUntilSlot(block1Slot, block1.ID())

	newAccount := newUserWallet.AccountOutput("TX2:0")
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

	return newAccountOutput.AccountID
}

func createImplicitToFullAccount(ts *testsuite.TestSuite) iotago.AccountID {
	node1 := ts.Node("node1")

	// CREATE IMPLICIT ACCOUNT FROM GENESIS BASIC UTXO, SENT TO A NEW USER WALLET.
	// this wallet is not registered in the ledger yet.
	newUserWallet := ts.AddWallet("second", node1, iotago.EmptyAccountID)
	// a default wallet, already registered in the ledger, will issue the transaction and block.
	tx3 := ts.DefaultWallet().CreateImplicitAccountFromInput(
		"TX3",
		"TX1:1",
		newUserWallet,
	)
	block2 := ts.IssueBasicBlockWithOptions("block2", ts.DefaultWallet(), tx3)
	block2Slot := block2.ID().Slot()
	latestParents := ts.CommitUntilSlot(block2Slot, block2.ID())

	implicitAccountOutput := newUserWallet.Output("TX3:0")
	implicitAccountOutputID := implicitAccountOutput.OutputID()
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
	// USE IMPLICIT ACCOUNT AS BLOCK ISSUER.
	fullAccountBlockIssuerKey := newUserWallet.BlockIssuerKey()

	block3Slot := ts.CurrentSlot()
	tx4 := newUserWallet.TransitionImplicitAccountToAccountOutput(
		"TX4",
		"TX3:0",
		mock.WithBlockIssuerFeature(
			iotago.BlockIssuerKeys{fullAccountBlockIssuerKey},
			iotago.MaxSlotIndex,
		),
		mock.WithAccountAmount(mock.MinIssuerAccountAmount(ts.API.ProtocolParameters())),
	)
	block2Commitment := node1.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment()
	block3 := ts.IssueBasicBlockWithOptions("block3", newUserWallet, tx4, mock.WithStrongParents(latestParents...))
	latestParents = ts.CommitUntilSlot(block3Slot, block3.ID())

	fullAccountOutputID := newUserWallet.Output("TX4:0").OutputID()
	allotted := iotago.BlockIssuanceCredits(tx4.Transaction.Allotments.Get(implicitAccountID))
	burned := iotago.BlockIssuanceCredits(block3.WorkScore()) * iotago.BlockIssuanceCredits(block2Commitment.ReferenceManaCost)
	// the implicit account should now have been transitioned to a full account in the accounts ledger.
	ts.AssertAccountDiff(implicitAccountID, block3Slot, &model.AccountDiff{
		BICChange:              allotted - burned,
		PreviousUpdatedSlot:    block2Slot,
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
		Credits:         accounts.NewBlockIssuanceCredits(allotted-burned, block3Slot),
		ExpirySlot:      iotago.MaxSlotIndex,
		OutputID:        fullAccountOutputID,
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(fullAccountBlockIssuerKey),
	}, ts.Nodes()...)

	return implicitAccountID
}

func allotManaTo(ts *testsuite.TestSuite, to iotago.AccountID) {
	wallet := ts.DefaultWallet()
	node1 := ts.Node("node1")
	node2 := ts.Node("node2")

	tx6 := wallet.AllotManaFromInputs("TX5",
		iotago.Allotments{&iotago.Allotment{
			AccountID: to,
			Mana:      iotago.Mana(1000),
		}}, "TX1:2")
	commitment := node1.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment()
	ts.IssueBasicBlockWithOptions("block4", wallet, tx6, mock.WithSlotCommitment(commitment))

	// Issue some more blocks to make transaction accepted
	{
		ts.IssueValidationBlockWithHeaderOptions("vblock6", node2, mock.WithStrongParents(ts.BlockID("block4")))
		ts.IssueValidationBlockWithHeaderOptions("vblock7", node1, mock.WithStrongParents(ts.BlockID("vblock6")))
		ts.IssueValidationBlockWithHeaderOptions("vblock8", node2, mock.WithStrongParents(ts.BlockID("vblock7")))

		ts.AssertTransactionsInCacheAccepted(wallet.Transactions("TX5"), true, node1, node2)
	}
}

func sendfunds(ts *testsuite.TestSuite) {
	node1 := ts.Node("node1")
	node2 := ts.Node("node2")
	wallet := ts.DefaultWallet()
	secondWallet := ts.Wallet("second")

	tx := wallet.SendFundsToWallet("TX6", secondWallet, "TX1:3")
	ts.IssueBasicBlockWithOptions("block5", wallet, tx)

	ts.AssertTransactionsExist(wallet.Transactions("TX6"), true, node1)
	ts.AssertTransactionsInCacheBooked(wallet.Transactions("TX6"), true, node1)

	// Issue some more blocks to make transaction accepted
	{
		ts.IssueValidationBlockWithHeaderOptions("vblock9", node2, mock.WithStrongParents(ts.BlockID("block5")))
		ts.IssueValidationBlockWithHeaderOptions("vblock10", node1, mock.WithStrongParents(ts.BlockID("vblock9")))
		ts.IssueValidationBlockWithHeaderOptions("vblock11", node2, mock.WithStrongParents(ts.BlockID("vblock10")))

		ts.AssertTransactionsInCacheAccepted(wallet.Transactions("TX5"), true, node1, node2)
	}

}

func createNativetoken(ts *testsuite.TestSuite, acc iotago.AccountID) {
	wallet := ts.Wallet("second")
	node1 := ts.Node("node1")

	accOutput := ts.Wallet("second").Output("TX4:0")
	tx := wallet.CreateNativeTokenFromInput("TX7", "TX6:0", accOutput)
	ts.IssueBasicBlockWithOptions("block5", wallet, tx)

	ts.AssertTransactionsExist(wallet.Transactions("TX7"), true, node1)
	ts.AssertTransactionsInCacheBooked(wallet.Transactions("TX7"), true, node1)
}

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

	// Add a validator node to the network. This will add a validator account to the snapshot.
	node1 := ts.AddValidatorNode("node1")
	node2 := ts.AddValidatorNode("node2")
	// Add a default block issuer to the network. This will add another block issuer account to the snapshot.
	wallet := ts.AddDefaultWallet(node1)

	ts.Run(true)

	node1.Protocol.SetLogLevel(log.LevelTrace)

	// split genesis output into two outputs for the 2 accounts creation, 1 as faucet funds
	tx1 := wallet.CreateBasicOutputsEquallyFromInput("TX1", 4, "Genesis:0")
	ts.IssueBasicBlockWithOptions("block0", wallet, tx1)

	// Issue some more blocks to make transaction accepted
	{
		ts.IssueValidationBlockWithHeaderOptions("vblock0", node2, mock.WithStrongParents(ts.BlockID("block0")))
		ts.IssueValidationBlockWithHeaderOptions("vblock1", node1, mock.WithStrongParents(ts.BlockID("vblock0")))
		ts.IssueValidationBlockWithHeaderOptions("vblock2", node2, mock.WithStrongParents(ts.BlockID("vblock1")))

		ts.AssertTransactionsInCacheAccepted(wallet.Transactions("TX1"), true, node1, node2)
	}

	// create the first account with TX1:0
	// generated (block1, TX2)
	createFullAccount(ts)

	// create the second account, from implicit to full account with TX1:1
	// generated (block2, TX3), (block3, TX4)
	implicitAccID := createImplicitToFullAccount(ts)

	// allot 1000 mana to implicit account with TX1:2
	// generated (block4, TX5)
	allotManaTo(ts, implicitAccID)

	// send funds to account 2, with TX1:3
	sendfunds(ts)

	// create native token
	createNativetoken(ts, implicitAccID)
}
