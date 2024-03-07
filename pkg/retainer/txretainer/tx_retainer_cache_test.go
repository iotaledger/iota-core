package txretainer_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/retainer/txretainer"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func TestTransactionRetainerCache(t *testing.T) {
	txMetadataUpdateFunc := func(oldTxMeta *txretainer.TransactionMetadata, newTxMeta *txretainer.TransactionMetadata) (*txretainer.TransactionMetadata, bool, error) {
		// always return the new transaction metadata in the tests
		return newTxMeta, true, nil
	}

	txRetainerCache := txretainer.NewTransactionRetainerCache(txretainer.WithTxRetainerCacheTxMetadataUpdateFunc(txMetadataUpdateFunc))

	txID1 := tpkg.RandTransactionID()
	txID2 := tpkg.RandTransactionID()
	txID3 := tpkg.RandTransactionID()

	newTxMeta1 := &txretainer.TransactionMetadata{
		TransactionID:          txID1[:],
		ValidSignature:         false,
		EarliestAttachmentSlot: 20,
		State:                  0,
		FailureReason:          0,
		ErrorMsg:               nil,
	}

	errMsg := "error message"
	newTxMeta1_2 := &txretainer.TransactionMetadata{
		TransactionID:          txID1[:],
		ValidSignature:         true,
		EarliestAttachmentSlot: 10,
		State:                  1,
		FailureReason:          1,
		ErrorMsg:               &errMsg,
	}

	newTxMeta2 := &txretainer.TransactionMetadata{
		TransactionID:          txID2[:],
		ValidSignature:         true,
		EarliestAttachmentSlot: 10,
		State:                  1,
		FailureReason:          1,
		ErrorMsg:               &errMsg,
	}

	newTxMeta3 := &txretainer.TransactionMetadata{
		TransactionID:          txID3[:],
		ValidSignature:         true,
		EarliestAttachmentSlot: 15,
		State:                  1,
		FailureReason:          1,
		ErrorMsg:               &errMsg,
	}

	// check that the transaction metadata doesn't exist
	txMeta, exists := txRetainerCache.TransactionMetadataByID(txID1)
	require.False(t, exists)
	require.Nil(t, txMeta)

	// add the transaction metadata
	err := txRetainerCache.UpdateTxMetadata(newTxMeta1)
	require.NoError(t, err)

	// check that the transaction metadata exists
	txMeta, exists = txRetainerCache.TransactionMetadataByID(txID1)
	require.True(t, exists)
	require.Equal(t, newTxMeta1, txMeta)

	// update the transaction metadata
	err = txRetainerCache.UpdateTxMetadata(newTxMeta1_2)
	require.NoError(t, err)

	// check that the transaction metadata has been updated
	txMeta, exists = txRetainerCache.TransactionMetadataByID(txID1)
	require.True(t, exists)
	require.Equal(t, newTxMeta1_2, txMeta)

	// update the earliest attachment slot of the transaction
	txRetainerCache.UpdateEarliestAttachmentSlot(txID1, 5)

	// check that the earliest attachment slot has been updated
	txMeta, exists = txRetainerCache.TransactionMetadataByID(txID1)
	require.True(t, exists)
	require.Equal(t, iotago.SlotIndex(5), txMeta.EarliestAttachmentSlot)

	// check that the transaction is now part of slot 5
	txIDs := txRetainerCache.DeleteAndReturnTxMetadataChangesBySlot(5)
	require.Len(t, txIDs, 1)
	require.Contains(t, txIDs, txID1)

	// check that the transaction is not part of slot 5 anymore
	txIDs = txRetainerCache.DeleteAndReturnTxMetadataChangesBySlot(5)
	require.Len(t, txIDs, 0)

	// check that the transaction doesn't exist anymore
	txMeta, exists = txRetainerCache.TransactionMetadataByID(txID1)
	require.False(t, exists)
	require.Nil(t, txMeta)

	// check that the transaction is not part of slot 10
	txIDs = txRetainerCache.DeleteAndReturnTxMetadataChangesBySlot(10)
	require.Len(t, txIDs, 0)

	// check that the transaction is not part of slot 20
	txIDs = txRetainerCache.DeleteAndReturnTxMetadataChangesBySlot(20)
	require.Len(t, txIDs, 0)

	// add the transaction metadata again
	err = txRetainerCache.UpdateTxMetadata(newTxMeta1)
	require.NoError(t, err)

	// add another one with another txID
	err = txRetainerCache.UpdateTxMetadata(newTxMeta2)
	require.NoError(t, err)

	// add another one with another txID
	err = txRetainerCache.UpdateTxMetadata(newTxMeta3)
	require.NoError(t, err)

	// update the earliest attachment slot of the transaction
	txRetainerCache.UpdateEarliestAttachmentSlot(txID1, 5)

	// check that two transactions get returned by a higher slot than the two, but lower than the third
	txIDs = txRetainerCache.DeleteAndReturnTxMetadataChangesBySlot(12)
	require.Len(t, txIDs, 2)
	require.Contains(t, txIDs, txID1)
	require.Contains(t, txIDs, txID2)

	// check that the two transaction metadata don't exist anymore
	txMeta, exists = txRetainerCache.TransactionMetadataByID(txID1)
	require.False(t, exists)
	require.Nil(t, txMeta)

	txMeta, exists = txRetainerCache.TransactionMetadataByID(txID2)
	require.False(t, exists)
	require.Nil(t, txMeta)

	// check that the third transaction metadata still exists
	txMeta, exists = txRetainerCache.TransactionMetadataByID(txID3)
	require.True(t, exists)
	require.Equal(t, newTxMeta3, txMeta)

	// reset the cache
	txRetainerCache.Reset()

	// check that the transaction metadata doesn't exist anymore
	txMeta, exists = txRetainerCache.TransactionMetadataByID(txID3)
	require.False(t, exists)
	require.Nil(t, txMeta)
}
