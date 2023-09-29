package mempoolv1

import (
	"testing"
)

func TestAttachments(t *testing.T) {
	//blockIDs := map[string]iotago.BlockID{
	//	"1": iotago.SlotIdentifierRepresentingData(1, []byte("block1")),
	//	"2": iotago.SlotIdentifierRepresentingData(2, []byte("block2")),
	//}
	//
	//transactionMetadata, err := NewTransactionMetadata(mempooltests.NewTransaction(2))
	//require.NoError(t, err)
	//
	//signedTransactionMetadata, err := NewSignedTransactionMetadata(mempooltests.NewSignedTransaction(transactionMetadata.Transaction()), transactionMetadata)
	//
	//require.True(t, attachments.addAttachment(blockIDs["1"]))
	//require.True(t, attachments.addAttachment(blockIDs["2"]))
	//
	//require.False(t, attachments.addAttachment(blockIDs["1"]))
	//
	//var earliestInclusionIndex, earliestInclusionIndex1, earliestInclusionIndex2 iotago.SlotIndex
	//
	//attachments.OnEarliestIncludedAttachmentUpdated(func(_, includedBlock iotago.BlockID) {
	//	earliestInclusionIndex = includedBlock.Slot()
	//})
	//require.Equal(t, iotago.SlotIndex(0), earliestInclusionIndex)
	//
	//attachments.markAttachmentIncluded(blockIDs["2"])
	//require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex)
	//attachments.markAttachmentIncluded(blockIDs["1"])
	//require.Equal(t, iotago.SlotIndex(1), earliestInclusionIndex)
	//
	//attachments.OnEarliestIncludedAttachmentUpdated(func(_, includedBlock iotago.BlockID) {
	//	earliestInclusionIndex1 = includedBlock.Slot()
	//})
	//
	//require.True(t, attachments.markAttachmentOrphaned(blockIDs["1"]))
	//require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex)
	//require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex1)
	//
	//require.False(t, attachments.markAttachmentOrphaned(blockIDs["1"]))
	//require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex)
	//require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex1)
	//
	//attachments.markAttachmentOrphaned(blockIDs["2"])
	//require.Equal(t, iotago.SlotIndex(0), earliestInclusionIndex)
	//require.Equal(t, iotago.SlotIndex(0), earliestInclusionIndex1)
	//
	//attachments.OnEarliestIncludedAttachmentUpdated(func(_, includedBlock iotago.BlockID) {
	//	earliestInclusionIndex2 = includedBlock.Slot()
	//})
	//require.Equal(t, iotago.SlotIndex(0), earliestInclusionIndex2)
}
