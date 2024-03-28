package tests

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
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
	ts.AddValidatorNode("node2")
	wallet := ts.AddDefaultWallet(node1)

	ts.Run(true)

	// Create a standard basic output transfer transaction.
	validTX := wallet.CreateBasicOutputsEquallyFromInput("TX", 1, "Genesis:0")
	invalidTX := validTX.Clone().(*iotago.SignedTransaction)

	// Make validTX invalid by replacing the first unlock with an empty signature unlock.
	_, is := invalidTX.Unlocks[0].(*iotago.SignatureUnlock)
	if !is {
		panic("expected signature unlock as first unlock")
	}
	invalidTX.Unlocks[0] = &iotago.SignatureUnlock{
		Signature: &iotago.Ed25519Signature{},
	}

	// Issue the invalid attachment of the transaction.
	err := lo.Return2(ts.IssueBasicBlockWithOptions("block1", wallet, invalidTX))
	require.NoError(t, err)

	// Ensure that the attachment is seen as pending (tx) and failed (signed tx).
	ts.AssertTransactionsExist([]*iotago.Transaction{invalidTX.Transaction}, true, ts.Nodes()...)
	ts.AssertTransactionsInCacheAccepted([]*iotago.Transaction{invalidTX.Transaction}, false, ts.Nodes()...)
	ts.AssertTransactionsInCachePending([]*iotago.Transaction{invalidTX.Transaction}, true, ts.Nodes()...)
	ts.AssertTransactionFailure(invalidTX.MustID(), iotago.ErrDirectUnlockableAddressUnlockInvalid, ts.Nodes()...)

	// Issue the valid attachment and another invalid attachment on top.
	block2, err := ts.IssueBasicBlockWithOptions("block2", wallet, validTX)
	require.NoError(t, err)

	block3, err := ts.IssueBasicBlockWithOptions("block3", wallet, invalidTX, mock.WithStrongParents(block2.ID()))
	require.NoError(t, err)

	// Accept block2 and block3.
	ts.IssueBlocksAtSlots("accept-block3", []iotago.SlotIndex{block3.ID().Slot()}, 4, "block3", ts.Nodes(), false, true)

	// Ensure that the valid attachment exists and got accepted,
	// while the invalid attachment did not override the previous valid attachment.
	ts.AssertTransactionsExist([]*iotago.Transaction{validTX.Transaction}, true, ts.Nodes()...)
	ts.AssertTransactionsInCacheAccepted([]*iotago.Transaction{validTX.Transaction}, true, ts.Nodes()...)

	// Both block should be accepted.
	ts.AssertBlocksInCacheAccepted(ts.Blocks("block2"), true, ts.Nodes()...)
	ts.AssertBlocksInCacheAccepted(ts.Blocks("block3"), true, ts.Nodes()...)
}
