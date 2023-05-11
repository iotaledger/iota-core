package mempoolv1

import (
	"testing"

	"github.com/stretchr/testify/require"

	mempooltests "github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/tests"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestAttachments(t *testing.T) {
	blockIDs := map[string]iotago.BlockID{
		"1": iotago.SlotIdentifierRepresentingData(1, []byte("block1")),
		"2": iotago.SlotIdentifierRepresentingData(2, []byte("block2")),
	}

	attachments, err := NewTransactionWithMetadata(mempooltests.NewTransaction(2))
	require.NoError(t, err)
	require.True(t, attachments.addAttachment(blockIDs["1"]))
	require.True(t, attachments.addAttachment(blockIDs["2"]))

	require.False(t, attachments.addAttachment(blockIDs["1"]))

	var earliestInclusionIndex, earliestInclusionIndex1, earliestInclusionIndex2 iotago.SlotIndex

	attachments.OnEarliestIncludedSlotUpdated(func(_, includedIndex iotago.SlotIndex) {
		earliestInclusionIndex = includedIndex
	})
	require.Equal(t, iotago.SlotIndex(0), earliestInclusionIndex)

	attachments.markAttachmentIncluded(blockIDs["2"])
	require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex)
	attachments.markAttachmentIncluded(blockIDs["1"])
	require.Equal(t, iotago.SlotIndex(1), earliestInclusionIndex)

	attachments.OnEarliestIncludedSlotUpdated(func(_, includedIndex iotago.SlotIndex) {
		earliestInclusionIndex1 = includedIndex
	})

	require.True(t, attachments.markAttachmentOrphaned(blockIDs["1"]))
	require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex)
	require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex1)

	require.False(t, attachments.markAttachmentOrphaned(blockIDs["1"]))
	require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex)
	require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex1)

	attachments.markAttachmentOrphaned(blockIDs["2"])
	require.Equal(t, iotago.SlotIndex(0), earliestInclusionIndex)
	require.Equal(t, iotago.SlotIndex(0), earliestInclusionIndex1)

	attachments.OnEarliestIncludedSlotUpdated(func(_, includedIndex iotago.SlotIndex) {
		earliestInclusionIndex2 = includedIndex
	})
	require.Equal(t, iotago.SlotIndex(0), earliestInclusionIndex2)
}
