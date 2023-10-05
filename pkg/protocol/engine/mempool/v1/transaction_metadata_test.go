package mempoolv1

import (
	"testing"

	"github.com/stretchr/testify/require"

	mempooltests "github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/tests"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestAttachments(t *testing.T) {
	blockIDs := map[string]iotago.BlockID{
		"1": iotago.BlockIDRepresentingData(1, []byte("block1")),
		"2": iotago.BlockIDRepresentingData(2, []byte("block2")),
	}

	transactionMetadata, err := NewTransactionMetadata(mempooltests.NewTransaction(2), nil)
	require.NoError(t, err)

	signedTransactionMetadata, err := NewSignedTransactionMetadata(mempooltests.NewSignedTransaction(transactionMetadata.Transaction()), transactionMetadata)

	require.True(t, signedTransactionMetadata.addAttachment(blockIDs["1"]))
	require.True(t, signedTransactionMetadata.addAttachment(blockIDs["2"]))

	require.False(t, signedTransactionMetadata.addAttachment(blockIDs["1"]))

	var earliestInclusionIndex iotago.SlotIndex

	signedTransactionMetadata.transactionMetadata.OnEarliestIncludedAttachmentUpdated(func(_, includedBlock iotago.BlockID) {
		earliestInclusionIndex = includedBlock.Slot()
	})
	require.Equal(t, iotago.SlotIndex(0), earliestInclusionIndex)

	signedTransactionMetadata.transactionMetadata.markAttachmentIncluded(blockIDs["2"])
	require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex)
	signedTransactionMetadata.transactionMetadata.markAttachmentIncluded(blockIDs["1"])
	require.Equal(t, iotago.SlotIndex(1), earliestInclusionIndex)
}
