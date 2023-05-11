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
	require.True(t, attachments.Add(blockIDs["1"]))
	require.True(t, attachments.Add(blockIDs["2"]))

	require.False(t, attachments.Add(blockIDs["1"]))

	var earliestInclusionIndex, earliestInclusionIndex1, earliestInclusionIndex2 iotago.SlotIndex

	attachments.OnEarliestIncludedAttachmentUpdated(func(_, includedBlock iotago.BlockID) {
		earliestInclusionIndex = includedBlock.Index()
	})
	require.Equal(t, iotago.SlotIndex(0), earliestInclusionIndex)

	attachments.MarkIncluded(blockIDs["2"])
	require.Equal(t, iotago.SlotIndex(2), earliestInclusionIndex)
	attachments.MarkIncluded(blockIDs["1"])
	require.Equal(t, iotago.SlotIndex(1), earliestInclusionIndex)

	attachments.OnEarliestIncludedAttachmentUpdated(func(_, includedBlock iotago.BlockID) {
		earliestInclusionIndex1 = includedBlock.Index()
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

	attachments.OnEarliestIncludedAttachmentUpdated(func(_, includedBlock iotago.BlockID) {
		earliestInclusionIndex2 = includedBlock.Index()
	})
	require.Equal(t, iotago.SlotIndex(0), earliestInclusionIndex2)
}
