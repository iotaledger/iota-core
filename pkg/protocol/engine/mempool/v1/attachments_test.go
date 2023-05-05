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
	require.True(t, attachments.Add(blockIDs["1"]))
	require.True(t, attachments.Add(blockIDs["2"]))

	require.False(t, attachments.Add(blockIDs["1"]))

	var earliestInclusionIndex, earliestInclusionIndex1, earliestInclusionIndex2 iotago.SlotIndex

	attachments.OnEarliestIncludedSlotUpdated(func(includedIndex iotago.SlotIndex) {
		earliestInclusionIndex = includedIndex
	})
	require.Equal(t, iotago.SlotIndex(0), earliestInclusionIndex)

	attachments.MarkIncluded(blockIDs["2"])
	require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex)
	attachments.MarkIncluded(blockIDs["1"])
	require.Equal(t, iotago.SlotIndex(1), earliestInclusionIndex)

	attachments.OnEarliestIncludedSlotUpdated(func(includedIndex iotago.SlotIndex) {
		earliestInclusionIndex1 = includedIndex
	})

	require.True(t, attachments.MarkOrphaned(blockIDs["1"]))
	require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex)
	require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex1)

	require.False(t, attachments.MarkOrphaned(blockIDs["1"]))
	require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex)
	require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex1)

	attachments.MarkOrphaned(blockIDs["2"])
	require.Equal(t, iotago.SlotIndex(0), earliestInclusionIndex)
	require.Equal(t, iotago.SlotIndex(0), earliestInclusionIndex1)

	attachments.OnEarliestIncludedSlotUpdated(func(includedIndex iotago.SlotIndex) {
		earliestInclusionIndex2 = includedIndex
	})
	require.Equal(t, iotago.SlotIndex(0), earliestInclusionIndex2)
}
