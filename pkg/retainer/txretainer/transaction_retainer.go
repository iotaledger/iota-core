package txretainer

import (
	"gorm.io/gorm"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/retainer"
	"github.com/iotaledger/iota-core/pkg/storage"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type Status struct {
	ID              uint `gorm:"primaryKey;notnull"`
	CommittedSlot   iotago.SlotIndex
	NetworkName     string
	DatabaseVersion uint32
}

var (
	_ retainer.TransactionRetainer = &TransactionRetainer{}
)

type (
	DatabaseFunc            func(func(*gorm.DB) error) error
	LatestCommittedSlotFunc func() iotago.SlotIndex
	FinalizedSlotFunc       func() iotago.SlotIndex
)

// TransactionRetainer keeps and resolves all the information needed in the API and INX.
type TransactionRetainer struct {
	workerPool              *workerpool.WorkerPool
	dbExecFunc              storage.SQLDatabaseExecFunc
	latestCommittedSlotFunc LatestCommittedSlotFunc
	finalizedSlotFunc       FinalizedSlotFunc
	errorHandler            func(error)

	storeDebugErrorMessages bool

	module.Module
}

// WithDebugStoreErrorMessages configures the TransactionRetainer to store debug error messages.
func WithDebugStoreErrorMessages(store bool) options.Option[TransactionRetainer] {
	return func(r *TransactionRetainer) {
		r.storeDebugErrorMessages = store
	}
}

func New(workersGroup *workerpool.Group, dbExecFunc storage.SQLDatabaseExecFunc, latestCommittedSlotFunc LatestCommittedSlotFunc, finalizedSlotFunc FinalizedSlotFunc, errorHandler func(error), opts ...options.Option[TransactionRetainer]) *TransactionRetainer {
	return options.Apply(&TransactionRetainer{
		workerPool:              workersGroup.CreatePool("TxRetainer", workerpool.WithWorkerCount(1)),
		dbExecFunc:              dbExecFunc,
		latestCommittedSlotFunc: latestCommittedSlotFunc,
		finalizedSlotFunc:       finalizedSlotFunc,
		errorHandler:            errorHandler,
	}, opts,
		(*TransactionRetainer).TriggerConstructed,
		(*TransactionRetainer).TriggerInitialized,
	)
}

// NewProvider creates a new TransactionRetainer provider.
func NewProvider(opts ...options.Option[TransactionRetainer]) module.Provider[*engine.Engine, retainer.TransactionRetainer] {
	return module.Provide(func(e *engine.Engine) retainer.TransactionRetainer {
		r := New(e.Workers.CreateGroup("TransactionRetainer"),
			e.Storage.TransactionRetainerDatabaseExecFunc(),
			func() iotago.SlotIndex {
				// use settings in case SyncManager is not constructed yet.
				if e.SyncManager == nil {
					return e.Storage.Settings().LatestCommitment().Slot()
				}

				return e.SyncManager.LatestCommitment().Slot()
			},
			func() iotago.SlotIndex {
				// use settings in case SyncManager is not constructed yet.
				if e.SyncManager == nil {
					return e.Storage.Settings().LatestFinalizedSlot()
				}

				return e.SyncManager.LatestFinalizedSlot()
			},
			e.ErrorHandler("txRetainer"),
			opts...,
		)

		asyncOpt := event.WithWorkerPool(r.workerPool)

		getTxErrMsg := func(err error) *string {
			if err == nil {
				return nil
			}
			errMsg := err.Error()
			return &errMsg
		}

		getEarliestIncludedAttachmentSlot := func(signedTransactionMetadata mempool.SignedTransactionMetadata) iotago.SlotIndex {
			return signedTransactionMetadata.TransactionMetadata().EarliestIncludedAttachment().Slot()
		}

		e.Initialized.OnTrigger(func() {
			// attaching the transaction failed for some reason => store the error
			// HINT: we treat the transaction as unsigned here, because we don't know if it was signed or not.
			// This should not be a problem, because the error reason will still be stored and visible to the user,
			// it might get overwritten by an invalid attachment later, but that's fine.
			e.Ledger.MemPool().OnAttachTransactionFailed(func(txID iotago.TransactionID, blockID iotago.BlockID, err error) {
				r.updateTxMetadata(txID, &txMetadata{
					TransactionID:          txID[:],
					ValidSignature:         false,
					EarliestAttachmentSlot: blockID.Slot(),
					State:                  byte(api.TransactionStateFailed),
					FailureReason:          byte(api.DetermineTransactionFailureReason(err)),
					ErrorMsg:               getTxErrMsg(err),
				})
			}, asyncOpt)

			// transaction was successfully attached to the tangle
			e.Ledger.MemPool().OnTransactionAttached(func(transactionMetadata mempool.TransactionMetadata) {
				e.LogTrace("TxRetainer.TransactionAttached", "tx", transactionMetadata.ID())

				// store that we saw the transaction
				txID := transactionMetadata.ID()
				r.updateTxMetadata(txID, &txMetadata{
					TransactionID:          txID[:],
					ValidSignature:         false,
					EarliestAttachmentSlot: transactionMetadata.EarliestIncludedAttachment().Slot(),
					State:                  byte(api.TransactionStatePending),
					FailureReason:          byte(api.TxFailureNone),
					ErrorMsg:               nil,
				})

				/*
					confirmed and finalized logic need to be done in the getter
					with the slot of the earliest included attachment
						first check if the earliest included attachment slot is finalized
						if not, check if the attachment itself was confirmed
					/*
						// TransactionStateFailed indicates that the transaction has not been executed by the node due to a failure during processing.
						TransactionStateFailed
				*/

				transactionMetadata.OnAccepted(func() {
					e.LogTrace("TxRetainer.TransactionAccepted", "tx", transactionMetadata.ID())

					// transaction was accepted
					txID := transactionMetadata.ID()
					r.updateTxMetadata(txID, &txMetadata{
						TransactionID:          txID[:],
						ValidSignature:         true, // if the transaction was accepted, the signature is valid
						EarliestAttachmentSlot: transactionMetadata.EarliestIncludedAttachment().Slot(),
						State:                  byte(api.TransactionStateAccepted),
						FailureReason:          0,
						ErrorMsg:               nil,
					})
				})

				// a conflict that lost => double spent
				transactionMetadata.OnRejected(func() {
					e.LogTrace("TxRetainer.TransactionRejected", "tx", transactionMetadata.ID())

					err := iotago.ErrInputAlreadySpent

					// transaction was rejected
					txID := transactionMetadata.ID()
					r.updateTxMetadata(txID, &txMetadata{
						TransactionID:          txID[:],
						ValidSignature:         true, // the other transaction in the conflict won, so this transaction can never be accepted, so we can set this value to true, so it does not get overwritten
						EarliestAttachmentSlot: transactionMetadata.EarliestIncludedAttachment().Slot(),
						State:                  byte(api.TransactionStateFailed),
						FailureReason:          byte(api.DetermineTransactionFailureReason(err)),
						ErrorMsg:               getTxErrMsg(err),
					})
				})

				// on invalid is when the transaction execution failed
				transactionMetadata.OnInvalid(func(err error) {
					e.LogTrace("TxRetainer.TransactionInvalid", "tx", transactionMetadata.ID(), "err", err)

					// if it's not the ErrRequestFailed, the error comes from the transaction execution
					// and therefore must have had a valid signature. If it's ErrRequestFailed, the signature
					// check is not done yet, but we are fine storing the error reason as invalid signature.
					isMempoolRequestFailedError := ierrors.Is(err, mempool.ErrRequestFailed)

					// store that the transaction is either invalid or the inputs could not be solidified.
					txID := transactionMetadata.ID()
					r.updateTxMetadata(txID, &txMetadata{
						TransactionID:          txID[:],
						ValidSignature:         !isMempoolRequestFailedError,
						EarliestAttachmentSlot: transactionMetadata.EarliestIncludedAttachment().Slot(),
						State:                  byte(api.TransactionStateFailed),
						FailureReason:          byte(api.DetermineTransactionFailureReason(err)),
						ErrorMsg:               getTxErrMsg(err),
					})
				})

				// 1. attached to the wrong place in the tangle, was not accepted within minCommitmentAge
				// 2. one of the inputs is orphaned, so the other transaction that produced that input got orphaned
				// 3. a conflicting tx was commited before this one, so this one gets orphaned
				transactionMetadata.OnOrphanedSlotUpdated(func(slot iotago.SlotIndex) {
					e.LogTrace("TxRetainer.TransactionOrphanedSlotUpdated", "tx", transactionMetadata.ID(), "slot", slot)

					//validSignature is unclear
					// TODO: wrong error reason
					err := iotago.ErrInputAlreadySpent

					// store that the transaction is either invalid or the inputs could not be solidified.
					txID := transactionMetadata.ID()
					r.updateTxMetadata(txID, &txMetadata{
						TransactionID:          txID[:],
						ValidSignature:         false,
						EarliestAttachmentSlot: transactionMetadata.EarliestIncludedAttachment().Slot(),
						State:                  byte(api.TransactionStateFailed),
						FailureReason:          byte(api.DetermineTransactionFailureReason(err)),
						ErrorMsg:               getTxErrMsg(err),
					})
				})
			})

			// is fired when an attachment of a transaction is detected
			e.Ledger.MemPool().OnSignedTransactionAttached(func(signedTransactionMetadata mempool.SignedTransactionMetadata) {

				// attachment with invalid signature detected
				// store that there is an attachment with an invalid signature.
				signedTransactionMetadata.OnSignaturesInvalid(func(err error) {
					txID := signedTransactionMetadata.TransactionMetadata().ID()

					r.updateTxMetadata(txID, &txMetadata{
						TransactionID:          txID[:],
						ValidSignature:         false,
						EarliestAttachmentSlot: getEarliestIncludedAttachmentSlot(signedTransactionMetadata),
						State:                  byte(api.TransactionStateFailed),
						FailureReason:          byte(api.DetermineTransactionFailureReason(err)),
						ErrorMsg:               getTxErrMsg(err),
					})
				})
			})
		})

		//e.Events.Retainer.LinkTo(r.events)

		r.TriggerInitialized()

		return r
	})
}

func (r *TransactionRetainer) Shutdown() {
	r.workerPool.Shutdown()
}

// Rollback rolls back the component state as if the last committed slot was targetSlot.
func (r *TransactionRetainer) Rollback(targetSlot iotago.SlotIndex) error {
	return r.deleteTxMetadataWithSlotGreaterThan(targetSlot)
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (r *TransactionRetainer) Reset() {
	if err := r.deleteTxMetadataWithSlotGreaterThan(r.latestCommittedSlotFunc()); err != nil {
		r.errorHandler(ierrors.Wrap(err, "failed to reset transaction retainer"))
	}
}

// Prune prunes the component state as if the last pruned slot was targetSlot.
func (r *TransactionRetainer) Prune(targetSlot iotago.SlotIndex) error {
	return r.deleteTxMetadataWithSlotSmallerOrEqualThan(targetSlot)
}

func (r *TransactionRetainer) UpdateTransactionMetadata(txID iotago.TransactionID, validSignature bool, earliestAttachmentSlot iotago.SlotIndex, state api.TransactionState, txErr error) {
	txFailureReason := api.TxFailureNone
	var txErrorMsg *string
	if txErr != nil {
		txFailureReason = api.DetermineTransactionFailureReason(txErr)
		errorMsg := txErr.Error()
		txErrorMsg = &errorMsg
	}

	r.updateTxMetadata(txID,
		&txMetadata{
			TransactionID:          txID[:],
			ValidSignature:         validSignature,
			EarliestAttachmentSlot: earliestAttachmentSlot,
			State:                  byte(state),
			FailureReason:          byte(txFailureReason),
			ErrorMsg:               txErrorMsg,
		},
	)
}

// TransactionMetadata returns the metadata of a transaction.
func (r *TransactionRetainer) TransactionMetadata(txID iotago.TransactionID) (*api.TransactionMetadataResponse, error) {
	txMeta := r.transactionMetadataByID(txID)
	if txMeta == nil {
		return nil, ErrEntryNotFound
	}

	txFailureDetails := ""
	if txMeta.ErrorMsg != nil {
		txFailureDetails = *txMeta.ErrorMsg
	}

	return &api.TransactionMetadataResponse{
		TransactionID:             txID,
		TransactionState:          api.TransactionState(txMeta.State),
		EarliestAttachmentSlot:    txMeta.EarliestAttachmentSlot,
		TransactionFailureReason:  api.TransactionFailureReason(txMeta.FailureReason),
		TransactionFailureDetails: txFailureDetails,
	}, nil
}
