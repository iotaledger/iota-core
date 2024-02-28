package tests

import (
	"testing"

	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Test_MempoolInvalidSignatures attempts to recreate the bug in https://github.com/iotaledger/iota-core/issues/765
func Test_MempoolInvalidSignatures(t *testing.T) {

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

	// create the account2, from implicit to full account from TX1:1 with wallet "second"
	// generated (block2, TX3), (block3, TX4)
	ts.AddWallet("second", node1, iotago.EmptyAccountID)
	transitionAccountWithInvalidSignature(ts)

}

func transitionAccountWithInvalidSignature(ts *testsuite.TestSuite) iotago.AccountID {
	node1 := ts.Node("node1")
	newUserWallet := ts.Wallet("second")

	// CREATE IMPLICIT ACCOUNT FROM GENESIS BASIC UTXO, SENT TO A NEW USER WALLET.
	// a default wallet, already registered in the ledger, will issue the transaction and block.
	tx3 := ts.DefaultWallet().CreateImplicitAccountAndBasicOutputFromInput(
		"TX3",
		"TX1:0",
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
	block3Slot := ts.CurrentSlot()
	tx4 := newUserWallet.TransitionImplicitAccountToAccountOutput(
		"TX4",
		[]string{"TX3:0", "TX3:1"},
		mock.WithBlockIssuerFeature(
			iotago.BlockIssuerKeys{implicitBlockIssuerKey},
			iotago.MaxSlotIndex,
		),
	)
	// replace the first unlock with an empty signature unlock
	_, is := tx4.Unlocks[0].(*iotago.SignatureUnlock)
	if !is {
		panic("expected signature unlock as first unlock")
	}
	tx4.Unlocks[0] = &iotago.SignatureUnlock{
		Signature: &iotago.Ed25519Signature{},
	}

	block2Commitment := node1.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment()
	block3 := ts.IssueBasicBlockWithOptions("block3", newUserWallet, tx4, mock.WithStrongParents(latestParents...))
	latestParents = ts.CommitUntilSlot(block3Slot, block3.ID())

	burned := iotago.BlockIssuanceCredits(block3.WorkScore()) * iotago.BlockIssuanceCredits(block2Commitment.ReferenceManaCost)
	// the implicit account transition should fail, so the burned amount should be deducted from BIC, but no allotment made.
	ts.AssertAccountDiff(implicitAccountID, block3Slot, &model.AccountDiff{
		BICChange:             -burned,
		PreviousUpdatedSlot:   block2Slot,
		NewOutputID:           implicitAccountOutputID,
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
		Credits:         accounts.NewBlockIssuanceCredits(-burned, block3Slot),
		ExpirySlot:      iotago.MaxSlotIndex,
		OutputID:        implicitAccountOutputID,
		BlockIssuerKeys: iotago.NewBlockIssuerKeys(implicitBlockIssuerKey),
	}, ts.Nodes()...)

	return implicitAccountID
}
