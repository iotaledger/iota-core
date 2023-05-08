package mempoolv1

import (
	"testing"

	"github.com/stretchr/testify/require"

	iotago "github.com/iotaledger/iota.go/v4"
)

func TestAttachments(t *testing.T) {
	blockIDs := map[string]iotago.BlockID{
		"1": iotago.SlotIdentifierRepresentingData(1, []byte("block1")),
		"2": iotago.SlotIdentifierRepresentingData(2, []byte("block2")),
	}

	attachments := NewAttachments()
	attachments.Add(blockIDs["1"])
	attachments.Add(blockIDs["2"])

	var earliestInclusionIndex, earliestInclusionIndex1, earliestInclusionIndex2 iotago.SlotIndex

	attachments.OnEarliestIncludedAttachmentUpdated(func(includedBlock iotago.BlockID) {
		earliestInclusionIndex = includedBlock.Index()
	})
	require.Equal(t, iotago.SlotIndex(0), earliestInclusionIndex)

	attachments.MarkIncluded(blockIDs["2"])
	require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex)
	attachments.MarkIncluded(blockIDs["1"])
	require.Equal(t, iotago.SlotIndex(1), earliestInclusionIndex)

	attachments.OnEarliestIncludedAttachmentUpdated(func(includedBlock iotago.BlockID) {
		earliestInclusionIndex1 = includedBlock.Index()
	})

	attachments.MarkOrphaned(blockIDs["1"])
	require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex)
	require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex1)
	attachments.MarkOrphaned(blockIDs["2"])
	require.Equal(t, iotago.SlotIndex(0), earliestInclusionIndex)
	require.Equal(t, iotago.SlotIndex(0), earliestInclusionIndex1)

	attachments.OnEarliestIncludedAttachmentUpdated(func(includedBlock iotago.BlockID) {
		earliestInclusionIndex2 = includedBlock.Index()
	})
	require.Equal(t, iotago.SlotIndex(0), earliestInclusionIndex2)
}
