package txretainer_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/retainer/txretainer"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type testTxMetadata struct {
	TransactionID          iotago.TransactionID
	ValidSignature         bool
	EarliestAttachmentSlot iotago.SlotIndex
	State                  api.TransactionState
	TransactionError       error
}

type test struct {
	name          string
	transactionID iotago.TransactionID
	newTxMetaFunc func() *testTxMetadata
	testFunc      func()
	resultFunc    func() *api.TransactionMetadataResponse
	targetErr     error
}

func (test *test) Run(t *testing.T, tr *txretainer.TransactionRetainer) {
	var newTxMeta *testTxMetadata
	if test.newTxMetaFunc != nil {
		newTxMeta = test.newTxMetaFunc()
	}

	if newTxMeta != nil {
		// update the transaction metadata in the retainer
		tr.UpdateTransactionMetadata(newTxMeta.TransactionID, newTxMeta.ValidSignature, newTxMeta.EarliestAttachmentSlot, newTxMeta.State, newTxMeta.TransactionError)
	}

	if test.testFunc != nil {
		test.testFunc()
	}

	// get the transaction metadata from the retainer
	txMeta, err := tr.TransactionMetadata(test.transactionID)
	if test.targetErr != nil {
		require.ErrorIs(t, err, test.targetErr, test.name)
		return
	}
	require.NoError(t, err, test.name)

	var targetTxMeta *api.TransactionMetadataResponse
	if test.resultFunc != nil {
		targetTxMeta = test.resultFunc()
	}
	require.Equal(t, targetTxMeta, txMeta, test.name)
}

func TestTransactionRetainer_UpdateMetadata(t *testing.T) {
	ts := newTestSuite(t)
	defer ts.Close()

	tr := ts.TxRetainer

	txID := tpkg.RandTransactionID()

	// IMPORTANT: all those tests modify the same transaction metadata and depend on each other
	for _, test := range []*test{
		// fail - transaction metadata not found because it was not added yet
		{
			name:          "fail - transaction metadata not found because it was not added yet",
			transactionID: txID,
			newTxMetaFunc: nil,
			testFunc:      nil,
			resultFunc: func() *api.TransactionMetadataResponse {
				return &api.TransactionMetadataResponse{
					TransactionID: txID,
				}
			},
			targetErr: txretainer.ErrEntryNotFound,
		},
		// ok - add attachment with invalid signature and no attachment slot
		{
			name:          "ok - add attachment with invalid signature and no attachment slot",
			transactionID: txID,
			newTxMetaFunc: func() *testTxMetadata {
				return &testTxMetadata{
					TransactionID:          txID,
					ValidSignature:         false,
					EarliestAttachmentSlot: 0,
					State:                  api.TransactionStateFailed,
					TransactionError:       iotago.ErrInputAlreadySpent,
				}
			},
			testFunc: nil,
			resultFunc: func() *api.TransactionMetadataResponse {
				return &api.TransactionMetadataResponse{
					TransactionID:             txID,
					TransactionState:          api.TransactionStateFailed,
					EarliestAttachmentSlot:    0,
					TransactionFailureReason:  api.DetermineTransactionFailureReason(iotago.ErrInputAlreadySpent),
					TransactionFailureDetails: iotago.ErrInputAlreadySpent.Error(),
				}
			},
			targetErr: nil,
		},
		// ok - add attachment with invalid signature and attachment slot set
		{
			name:          "ok - add attachment with invalid signature and attachment slot set",
			transactionID: txID,
			newTxMetaFunc: func() *testTxMetadata {
				return &testTxMetadata{
					TransactionID:          txID,
					ValidSignature:         false,
					EarliestAttachmentSlot: 10,
					State:                  api.TransactionStateAccepted,
					TransactionError:       nil,
				}
			},
			testFunc: nil,
			resultFunc: func() *api.TransactionMetadataResponse {
				return &api.TransactionMetadataResponse{
					TransactionID:             txID,
					TransactionState:          api.TransactionStateAccepted,
					EarliestAttachmentSlot:    10,
					TransactionFailureReason:  api.TxFailureNone,
					TransactionFailureDetails: "",
				}
			},
			targetErr: nil,
		},
		// ok - add attachment with invalid signature and higher attachment slot set
		{
			name:          "ok - add attachment with invalid signature and higher attachment slot set",
			transactionID: txID,
			newTxMetaFunc: func() *testTxMetadata {
				return &testTxMetadata{
					TransactionID:          txID,
					ValidSignature:         false,
					EarliestAttachmentSlot: 15,
					State:                  api.TransactionStateFailed,
					TransactionError:       iotago.ErrInputAlreadySpent,
				}
			},
			testFunc: nil,
			resultFunc: func() *api.TransactionMetadataResponse {
				return &api.TransactionMetadataResponse{
					TransactionID:             txID,
					TransactionState:          api.TransactionStateFailed,
					EarliestAttachmentSlot:    10,
					TransactionFailureReason:  api.DetermineTransactionFailureReason(iotago.ErrInputAlreadySpent),
					TransactionFailureDetails: iotago.ErrInputAlreadySpent.Error(),
				}
			},
			targetErr: nil,
		},
		// ok - add attachment with invalid signature and lower attachment slot set
		{
			name:          "ok - add attachment with invalid signature and lower attachment slot set",
			transactionID: txID,
			newTxMetaFunc: func() *testTxMetadata {
				return &testTxMetadata{
					TransactionID:          txID,
					ValidSignature:         false,
					EarliestAttachmentSlot: 5,
					State:                  api.TransactionStateFailed,
					TransactionError:       iotago.ErrInputAlreadySpent,
				}
			},
			testFunc: nil,
			resultFunc: func() *api.TransactionMetadataResponse {
				return &api.TransactionMetadataResponse{
					TransactionID:             txID,
					TransactionState:          api.TransactionStateFailed,
					EarliestAttachmentSlot:    5,
					TransactionFailureReason:  api.DetermineTransactionFailureReason(iotago.ErrInputAlreadySpent),
					TransactionFailureDetails: iotago.ErrInputAlreadySpent.Error(),
				}
			},
			targetErr: nil,
		},
		// ok - add attachment with valid signature and higher attachment slot than the invalid ones before set
		{
			name:          "ok - add attachment with valid signature and higher attachment slot than the invalid ones before set",
			transactionID: txID,
			newTxMetaFunc: func() *testTxMetadata {
				return &testTxMetadata{
					TransactionID:          txID,
					ValidSignature:         true,
					EarliestAttachmentSlot: 15,
					State:                  api.TransactionStatePending,
					TransactionError:       nil,
				}
			},
			testFunc: nil,
			resultFunc: func() *api.TransactionMetadataResponse {
				return &api.TransactionMetadataResponse{
					TransactionID:             txID,
					TransactionState:          api.TransactionStatePending,
					EarliestAttachmentSlot:    15,
					TransactionFailureReason:  api.TxFailureNone,
					TransactionFailureDetails: "",
				}
			},
			targetErr: nil,
		},
		// ok - add attachment with valid signature and higher attachment slot set
		{
			name:          "ok - add attachment with valid signature and higher attachment slot set",
			transactionID: txID,
			newTxMetaFunc: func() *testTxMetadata {
				return &testTxMetadata{
					TransactionID:          txID,
					ValidSignature:         true,
					EarliestAttachmentSlot: 20,
					State:                  api.TransactionStateAccepted,
					TransactionError:       nil,
				}
			},
			testFunc: nil,
			resultFunc: func() *api.TransactionMetadataResponse {
				return &api.TransactionMetadataResponse{
					TransactionID:             txID,
					TransactionState:          api.TransactionStateAccepted,
					EarliestAttachmentSlot:    15,
					TransactionFailureReason:  api.TxFailureNone,
					TransactionFailureDetails: "",
				}
			},
			targetErr: nil,
		},
		// ok - add attachment with valid signature and lower attachment slot set
		{
			name:          "ok - add attachment with valid signature and lower attachment slot set",
			transactionID: txID,
			newTxMetaFunc: func() *testTxMetadata {
				return &testTxMetadata{
					TransactionID:          txID,
					ValidSignature:         true,
					EarliestAttachmentSlot: 10,
					State:                  api.TransactionStatePending,
					TransactionError:       nil,
				}
			},
			testFunc: nil,
			resultFunc: func() *api.TransactionMetadataResponse {
				return &api.TransactionMetadataResponse{
					TransactionID:             txID,
					TransactionState:          api.TransactionStatePending,
					EarliestAttachmentSlot:    10,
					TransactionFailureReason:  api.TxFailureNone,
					TransactionFailureDetails: "",
				}
			},
			targetErr: nil,
		},
		// ok - add attachment with invalid signature and lower attachment slot set
		{
			name:          "ok - add attachment with invalid signature and lower attachment slot set",
			transactionID: txID,
			newTxMetaFunc: func() *testTxMetadata {
				return &testTxMetadata{
					TransactionID:          txID,
					ValidSignature:         false,
					EarliestAttachmentSlot: 5,
					State:                  api.TransactionStatePending,
					TransactionError:       nil,
				}
			},
			testFunc: nil,
			resultFunc: func() *api.TransactionMetadataResponse {
				return &api.TransactionMetadataResponse{
					TransactionID:             txID,
					TransactionState:          api.TransactionStatePending,
					EarliestAttachmentSlot:    10,
					TransactionFailureReason:  api.TxFailureNone,
					TransactionFailureDetails: "",
				}
			},
			targetErr: nil,
		},
	} {
		test.Run(t, tr)
	}
}

func TestTransactionRetainer_ResetAndPruning(t *testing.T) {
	ts := newTestSuite(t)
	defer ts.Close()

	tr := ts.TxRetainer

	txID := tpkg.RandTransactionID()

	// IMPORTANT: all those tests modify the same transaction metadata and depend on each other
	for _, test := range []*test{
		// ok - prune below the earliest attachment slot
		{
			name:          "ok - prune below the earliest attachment slot",
			transactionID: txID,
			newTxMetaFunc: func() *testTxMetadata {
				return &testTxMetadata{
					TransactionID:          txID,
					ValidSignature:         false,
					EarliestAttachmentSlot: 10,
					State:                  api.TransactionStateFailed,
					TransactionError:       iotago.ErrInputAlreadySpent,
				}
			},
			testFunc: func() {
				ts.SetLatestCommittedSlot(9)
				tr.Prune(9)
			},
			resultFunc: func() *api.TransactionMetadataResponse {
				return &api.TransactionMetadataResponse{
					TransactionID:             txID,
					TransactionState:          api.TransactionStateFailed,
					EarliestAttachmentSlot:    10,
					TransactionFailureReason:  api.DetermineTransactionFailureReason(iotago.ErrInputAlreadySpent),
					TransactionFailureDetails: iotago.ErrInputAlreadySpent.Error(),
				}
			},
			targetErr: nil,
		},
		// fail - prune at same slot as the earliest attachment slot
		{
			name:          "fail - prune at same slot as the earliest attachment slot",
			transactionID: txID,
			newTxMetaFunc: func() *testTxMetadata {
				return &testTxMetadata{
					TransactionID:          txID,
					ValidSignature:         false,
					EarliestAttachmentSlot: 10,
					State:                  api.TransactionStateFailed,
					TransactionError:       iotago.ErrInputAlreadySpent,
				}
			},
			testFunc: func() {
				ts.SetLatestCommittedSlot(10)
				tr.Prune(10)
			},
			resultFunc: nil,
			targetErr:  txretainer.ErrEntryNotFound,
		},
		// fail - prune a slot above the earliest attachment slot
		{
			name:          "fail - prune a slot above the earliest attachment slot",
			transactionID: txID,
			newTxMetaFunc: func() *testTxMetadata {
				return &testTxMetadata{
					TransactionID:          txID,
					ValidSignature:         false,
					EarliestAttachmentSlot: 10,
					State:                  api.TransactionStateFailed,
					TransactionError:       iotago.ErrInputAlreadySpent,
				}
			},
			testFunc: func() {
				ts.SetLatestCommittedSlot(11)
				tr.Prune(11)
			},
			resultFunc: nil,
			targetErr:  txretainer.ErrEntryNotFound,
		},
		// ok - reset a slot above the earliest attachment slot
		{
			name:          "ok - reset a slot above the earliest attachment slot",
			transactionID: txID,
			newTxMetaFunc: func() *testTxMetadata {
				return &testTxMetadata{
					TransactionID:          txID,
					ValidSignature:         false,
					EarliestAttachmentSlot: 10,
					State:                  api.TransactionStateFailed,
					TransactionError:       iotago.ErrInputAlreadySpent,
				}
			},
			testFunc: func() {
				ts.SetLatestCommittedSlot(11)
				tr.Reset(11)
			},
			resultFunc: func() *api.TransactionMetadataResponse {
				return &api.TransactionMetadataResponse{
					TransactionID:             txID,
					TransactionState:          api.TransactionStateFailed,
					EarliestAttachmentSlot:    10,
					TransactionFailureReason:  api.DetermineTransactionFailureReason(iotago.ErrInputAlreadySpent),
					TransactionFailureDetails: iotago.ErrInputAlreadySpent.Error(),
				}
			},
			targetErr: nil,
		},
		// ok - reset at same slot as the earliest attachment slot
		{
			name:          "ok - reset at same slot as the earliest attachment slot",
			transactionID: txID,
			newTxMetaFunc: func() *testTxMetadata {
				return &testTxMetadata{
					TransactionID:          txID,
					ValidSignature:         false,
					EarliestAttachmentSlot: 10,
					State:                  api.TransactionStateFailed,
					TransactionError:       iotago.ErrInputAlreadySpent,
				}
			},
			testFunc: func() {
				ts.SetLatestCommittedSlot(10)
				tr.Reset(10)
			},
			resultFunc: func() *api.TransactionMetadataResponse {
				return &api.TransactionMetadataResponse{
					TransactionID:             txID,
					TransactionState:          api.TransactionStateFailed,
					EarliestAttachmentSlot:    10,
					TransactionFailureReason:  api.DetermineTransactionFailureReason(iotago.ErrInputAlreadySpent),
					TransactionFailureDetails: iotago.ErrInputAlreadySpent.Error(),
				}
			},
			targetErr: nil,
		},
		// fail - reset below the earliest attachment slot
		{
			name:          "fail - reset below the earliest attachment slot",
			transactionID: txID,
			newTxMetaFunc: func() *testTxMetadata {
				return &testTxMetadata{
					TransactionID:          txID,
					ValidSignature:         false,
					EarliestAttachmentSlot: 10,
					State:                  api.TransactionStateFailed,
					TransactionError:       iotago.ErrInputAlreadySpent,
				}
			},
			testFunc: func() {
				ts.SetLatestCommittedSlot(9)
				tr.Reset(9)
			},
			resultFunc: nil,
			targetErr:  txretainer.ErrEntryNotFound,
		},
	} {
		test.Run(t, tr)
	}
}
