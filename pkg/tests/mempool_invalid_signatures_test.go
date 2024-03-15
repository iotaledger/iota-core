package tests

import (
	"testing"

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

	transitionBasicOutputWithInvalidSignature(ts)
}

func transitionBasicOutputWithInvalidSignature(ts *testsuite.TestSuite) {
	wallet := ts.DefaultWallet()

	// Create a standard basic output transfer transaction.
	tx3 := wallet.CreateBasicOutputsEquallyFromInput("TX3", 1, "TX1:0")
	tx4 := tx3.Clone().(*iotago.SignedTransaction)

	// Make tx3 invalid by replacing the first unlock with an empty signature unlock.
	_, is := tx3.Unlocks[0].(*iotago.SignatureUnlock)
	if !is {
		panic("expected signature unlock as first unlock")
	}
	tx3.Unlocks[0] = &iotago.SignatureUnlock{
		Signature: &iotago.Ed25519Signature{},
	}

	// Issue the invalid attachment of the transaction.
	ts.IssueBasicBlockWithOptions("block2", wallet, tx3)

	ts.Wait(ts.Nodes()...)

	// Ensure that the attachment is seen as invalid.
	ts.AssertTransactionsExist([]*iotago.Transaction{tx3.Transaction}, true, ts.Nodes()...)
	// TODO: This fails. The TX is still pending. Is this fine? The corresponding signed tx on the other hand is
	// marked as failed/invalid as asserted below.
	// ts.AssertTransactionsInCacheInvalid([]*iotago.Transaction{tx3.Transaction}, true, ts.Nodes()...)
	signedTx3ID := tx3.MustID()
	ts.AssertTransactionFailure(signedTx3ID, iotago.ErrDirectUnlockableAddressUnlockInvalid, ts.Nodes()...)

	// Issue the valid attachment and another invalid attachment on top.
	block3 := ts.IssueBasicBlockWithOptions("block3", wallet, tx4)
	block4 := ts.IssueBasicBlockWithOptions("block4", wallet, tx3, mock.WithStrongParents(block3.ID()))

	ts.CommitUntilSlot(block4.ID().Slot(), block4.ID())

	// Ensure that the valid attachment exists and got accepted,
	// while the invalid attachment did not override the previous valid attachment.
	ts.AssertTransactionsExist([]*iotago.Transaction{tx4.Transaction}, true, ts.Nodes()...)
	ts.AssertTransactionsInCacheAccepted([]*iotago.Transaction{tx4.Transaction}, true, ts.Nodes()...)

	// TODO: Fails with "block BlockID(block3:1) is root block". Do we even need to assert this?
	// ts.AssertBlocksInCacheAccepted(ts.Blocks("block3"), true, ts.Nodes()...)
	// ts.AssertBlocksInCacheInvalid(ts.Blocks("block3"), false, ts.Nodes()...)

	// ts.AssertBlocksInCacheAccepted(ts.Blocks("block4"), false, ts.Nodes()...)
	// ts.AssertBlocksInCacheInvalid(ts.Blocks("block4"), true, ts.Nodes()...)
}
