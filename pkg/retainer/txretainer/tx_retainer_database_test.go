package txretainer_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/retainer/txretainer"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func TestTransactionRetainerDatabase(t *testing.T) {
	clonableSQLiteDB := newClonableSQLiteDatabase(t)
	defer clonableSQLiteDB.Shutdown()

	txRetainerDatabase := txretainer.NewTransactionRetainerDB(clonableSQLiteDB.ExecDBFunc())

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
	txMeta, err := txRetainerDatabase.TransactionMetadataByID(txID1)
	require.NoError(t, err)
	require.Nil(t, txMeta)

	// add the transaction metadata
	err = txRetainerDatabase.InsertTxMetadata(newTxMeta1)
	require.NoError(t, err)

	// check that the transaction metadata exists
	txMeta, err = txRetainerDatabase.TransactionMetadataByID(txID1)
	require.NoError(t, err)
	require.Equal(t, newTxMeta1, txMeta)

	// update the transaction metadata
	err = txRetainerDatabase.InsertTxMetadata(newTxMeta1_2)
	require.NoError(t, err)

	// check that the transaction metadata has been updated
	txMeta, err = txRetainerDatabase.TransactionMetadataByID(txID1)
	require.NoError(t, err)
	require.Equal(t, newTxMeta1_2, txMeta)

	// add the transaction metadata
	err = txRetainerDatabase.InsertTxMetadata(newTxMeta2)
	require.NoError(t, err)

	// add the transaction metadata
	err = txRetainerDatabase.InsertTxMetadata(newTxMeta3)
	require.NoError(t, err)

}
