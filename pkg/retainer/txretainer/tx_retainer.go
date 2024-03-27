package txretainer

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/retainer"
	"github.com/iotaledger/iota-core/pkg/storage"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

var (
	// ErrEntryNotFound is returned when a transaction metadata entry is not found.
	ErrEntryNotFound = ierrors.New("entry not found")

	// code guard to ensure that the TransactionRetainer implements the retainer.TransactionRetainer interface.
	_ retainer.TransactionRetainer = &TransactionRetainer{}
)

// TransactionMetadata represents the metadata of a transaction in the SQL database and the cache.
// The ID is the primary key of the database table. We need this ID because we want to be able to store multiple
// metadata entries for the same transaction ID, but with different earliest attachment slots.
// This is necessary because we want to be able to rollback the transaction retainer to a specific slot by deleting
// all entries with an earliest attachment slot greater than the target slot.
type TransactionMetadata struct {
	ID                     uint64           `gorm:"autoIncrement"`
	TransactionID          []byte           `gorm:"notnull;index:transaction_ids"`
	ValidSignature         bool             `gorm:"notnull"`
	EarliestAttachmentSlot iotago.SlotIndex `gorm:"notnull"`
	State                  byte             `gorm:"notnull"`
	FailureReason          byte             `gorm:"notnull"`
	ErrorMsg               *string
}

// Equal compares everything except the Database ID.
func (m *TransactionMetadata) Equal(other *TransactionMetadata) bool {
	if m == nil || other == nil {
		return m == other
	}

	if !bytes.Equal(m.TransactionID, other.TransactionID) {
		return false
	}

	if m.ValidSignature != other.ValidSignature {
		return false
	}

	if m.EarliestAttachmentSlot != other.EarliestAttachmentSlot {
		return false
	}

	if m.State != other.State {
		return false
	}

	if m.FailureReason != other.FailureReason {
		return false
	}

	if (m.ErrorMsg == nil || other.ErrorMsg == nil) && m.ErrorMsg != other.ErrorMsg {
		return false
	}

	if (m.ErrorMsg != nil && other.ErrorMsg != nil) && *m.ErrorMsg != *other.ErrorMsg {
		return false
	}

	return true
}

func (m *TransactionMetadata) String() string {
	return fmt.Sprintf("tx metadata => TxID: %s, CreatedAtSlot: %d, ValidSignature: %t, State: %d, Error: %d, ErrorMsg: \"%s\"", hex.EncodeToString(m.TransactionID), m.EarliestAttachmentSlot, m.ValidSignature, m.State, m.FailureReason, *m.ErrorMsg)
}

type (
	SlotFunc             func() iotago.SlotIndex
	txMetadataUpdateFunc func(oldTxMeta *TransactionMetadata, newTxMeta *TransactionMetadata) (*TransactionMetadata, bool, error)
)

// TransactionRetainer keeps and resolves all the transaction-related metadata needed in the API and INX.
type TransactionRetainer struct {
	events                  *retainer.TransactionRetainerEvents
	workerPool              *workerpool.WorkerPool
	txRetainerDatabase      *transactionRetainerDatabase
	latestCommittedSlotFunc SlotFunc
	finalizedSlotFunc       SlotFunc
	errorHandler            func(error)

	txRetainerCache *transactionRetainerCache

	storeDebugErrorMessages bool

	module.Module
}

// WithDebugStoreErrorMessages configures the TransactionRetainer to store debug error messages.
func WithDebugStoreErrorMessages(store bool) options.Option[TransactionRetainer] {
	return func(r *TransactionRetainer) {
		r.storeDebugErrorMessages = store
	}
}

func New(subModule module.Module, workersGroup *workerpool.Group, dbExecFunc storage.SQLDatabaseExecFunc, latestCommittedSlotFunc SlotFunc, finalizedSlotFunc SlotFunc, errorHandler func(error), opts ...options.Option[TransactionRetainer]) *TransactionRetainer {
	return options.Apply(&TransactionRetainer{
		Module:                  subModule,
		events:                  retainer.NewTransactionRetainerEvents(),
		workerPool:              workersGroup.CreatePool("TxRetainer", workerpool.WithWorkerCount(1)),
		txRetainerCache:         NewTransactionRetainerCache(),
		txRetainerDatabase:      NewTransactionRetainerDB(dbExecFunc),
		latestCommittedSlotFunc: latestCommittedSlotFunc,
		finalizedSlotFunc:       finalizedSlotFunc,
		errorHandler:            errorHandler,
	}, opts, func(r *TransactionRetainer) {
		r.ShutdownEvent().OnTrigger(r.shutdown)

		r.ConstructedEvent().Trigger()
	})
}

// NewProvider creates a new TransactionRetainer provider.
func NewProvider(opts ...options.Option[TransactionRetainer]) module.Provider[*engine.Engine, retainer.TransactionRetainer] {
	return module.Provide(func(e *engine.Engine) retainer.TransactionRetainer {
		r := New(e.NewSubModule("TransactionRetainer"),
			e.Workers.CreateGroup("TransactionRetainer"),
			e.Storage.TransactionRetainerDatabaseExecFunc(),
			func() iotago.SlotIndex {
				return e.SyncManager.LatestCommitment().Slot()
			},
			func() iotago.SlotIndex {
				return e.SyncManager.LatestFinalizedSlot()
			},
			e.ErrorHandler("txRetainer"),
			opts...,
		)

		asyncOpt := event.WithWorkerPool(r.workerPool)

		e.ConstructedEvent().OnTrigger(func() {
			// attaching the transaction failed for some reason => store the error
			// HINT: we treat the transaction as unsigned here, because we don't know if it was signed or not.
			// This should not be a problem, because the error reason will still be stored and visible to the user,
			// it might get overwritten by an invalid attachment later, but that's fine.
			e.Ledger.MemPool().OnAttachTransactionFailed(func(txID iotago.TransactionID, blockID iotago.BlockID, err error) {
				r.UpdateTransactionMetadata(txID, false, blockID.Slot(), api.TransactionStateFailed, err)
			}, asyncOpt)

			// transaction was successfully attached to the tangle
			e.Ledger.MemPool().OnTransactionAttached(func(transactionMetadata mempool.TransactionMetadata) {
				e.LogTrace("TxRetainer.TransactionAttached", "tx", transactionMetadata.ID())

				// store that we saw the transaction.
				// we don't know the validity of the signature yet, but it is not important at this point.
				// if there is already an entry with a valid signature, it should not be overwritten,
				// therefore we use false for the "validSignature" argument.
				r.UpdateTransactionMetadata(transactionMetadata.ID(), false, transactionMetadata.EarliestIncludedAttachment().Slot(), api.TransactionStatePending, nil)

				r.events.TransactionRetained.Trigger(transactionMetadata.ID())

				// the transaction was accepted
				transactionMetadata.OnAccepted(func() {
					e.LogTrace("TxRetainer.TransactionAccepted", "tx", transactionMetadata.ID())

					// if the transaction was accepted, the signature is valid
					r.UpdateTransactionMetadata(transactionMetadata.ID(), true, transactionMetadata.EarliestIncludedAttachment().Slot(), api.TransactionStateAccepted, nil)
				})

				// the earliest attachment slot of the transaction was updated
				transactionMetadata.OnEarliestIncludedAttachmentUpdated(func(prevID, newID iotago.BlockID) {
					e.LogTrace("TxRetainer.TransactionEarliestIncludedAttachmentUpdated", "tx", transactionMetadata.ID(), "previous slot", prevID.Slot(), "new slot", newID.Slot())

					// update the earliest attachment slot
					r.txRetainerCache.UpdateEarliestAttachmentSlot(transactionMetadata.ID(), newID.Slot())
				})

				// transaction was rejected
				transactionMetadata.OnRejected(func() {
					e.LogTrace("TxRetainer.TransactionRejected", "tx", transactionMetadata.ID())

					// this transaction is a conflict that lost => double spent
					// since the other transaction in the conflict won, this transaction can never be accepted,
					// therefore we can set signed to true, so it doesn't get overwritten later.
					r.UpdateTransactionMetadata(transactionMetadata.ID(), true, transactionMetadata.EarliestIncludedAttachment().Slot(), api.TransactionStateFailed, iotago.ErrTxConflictRejected)
				})

				// the transaction was marked as invalid
				transactionMetadata.OnInvalid(func(err error) {
					e.LogTrace("TxRetainer.TransactionInvalid", "tx", transactionMetadata.ID(), "err", err)

					// if it's not the ErrInputSolidificationRequestFailed, the error comes from the transaction execution
					// and therefore must have had a valid signature. If it's ErrInputSolidificationRequestFailed, the signature
					// check is not done yet, but we are fine storing the error reason as invalid signature.
					isMempoolRequestFailedError := ierrors.Is(err, mempool.ErrInputSolidificationRequestFailed)

					// store that the transaction is either invalid or the inputs could not be solidified.
					r.UpdateTransactionMetadata(transactionMetadata.ID(), !isMempoolRequestFailedError, transactionMetadata.EarliestIncludedAttachment().Slot(), api.TransactionStateFailed, err)
				})

				// the transaction was marked as orphaned
				transactionMetadata.OnOrphanedSlotUpdated(func(slot iotago.SlotIndex) {
					e.LogTrace("TxRetainer.TransactionOrphanedSlotUpdated", "tx", transactionMetadata.ID(), "slot", slot)

					// transaction was orphaned for one of the following reasons:
					// 	- attached to the wrong place in the tangle, was not accepted within minCommittableAge
					// 	- one of the inputs is orphaned, so the other transaction that produced that input got orphaned as well
					// 	- a conflicting tx was committed before this one, so this one gets orphaned
					// we can use false for the "validSignature" argument, because we don't know here if the signature is valid or not,
					// and it was neither invalid nor accepted nor rejected before, so there was no entry with
					// "validSignature" set to true anyway.
					r.UpdateTransactionMetadata(transactionMetadata.ID(), false, transactionMetadata.EarliestIncludedAttachment().Slot(), api.TransactionStateFailed, ierrors.WithMessagef(iotago.ErrTxOrphaned, "transaction orphaned in slot %d", slot))
				})
			}, asyncOpt)

			// this event is fired when an attachment of a transaction is detected
			e.Ledger.MemPool().OnSignedTransactionAttached(func(signedTransactionMetadata mempool.SignedTransactionMetadata) {
				// attachment with invalid signature detected
				signedTransactionMetadata.OnSignaturesInvalid(func(err error) {
					transactionMetadata := signedTransactionMetadata.TransactionMetadata()

					// store that there is an attachment with an invalid signature.
					// TODO: is it even correct to use the earliest attachment slot here?
					r.UpdateTransactionMetadata(transactionMetadata.ID(), false, transactionMetadata.EarliestIncludedAttachment().Slot(), api.TransactionStateFailed, err)
				})
			}, asyncOpt)

			// this event is fired when a new commitment is detected
			e.Events.Notarization.LatestCommitmentUpdated.Hook(func(commitment *model.Commitment) {
				if err := r.CommitSlot(commitment.Slot()); err != nil {
					panic(err)
				}
			}, asyncOpt)

			// this event is fired when the storage successfully pruned an epoch
			e.Storage.Pruned.Hook(func(epoch iotago.EpochIndex) {
				epochEndSlot := e.CommittedAPI().TimeProvider().EpochEnd(epoch)

				// pruning should be done until and including the last slot of the pruned epoch
				if err := r.txRetainerDatabase.Prune(epochEndSlot); err != nil {
					r.errorHandler(err)
				}
			}, asyncOpt)

			e.Events.TransactionRetainer.TransactionRetained.LinkTo(r.events.TransactionRetained)

			r.InitializedEvent().Trigger()
		})

		return r
	})
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (r *TransactionRetainer) Reset(targetSlot iotago.SlotIndex) {
	// In the TransactionRetainer, we rely on the fact that "Reset" is always called
	// when a new engine is initialized (even during chain switching).
	// This is the cleanest way to rollback the state on disc to the
	// last committed slot without the need to initialize temporary
	// components in the "ForkAtSlot" method.
	//
	// we need to rollback the transaction retainer to the target slot
	// to delete all transactions that were committed after the target slot.
	if err := r.txRetainerDatabase.Rollback(targetSlot); err != nil {
		panic(ierrors.Wrap(err, "failed to rollback transaction retainer"))
	}

	// reset the cache
	r.txRetainerCache.Reset()
}

// Prune prunes the component state as if the last pruned slot was targetSlot.
func (r *TransactionRetainer) Prune(targetSlot iotago.SlotIndex) error {
	// we do not prune the data from the cache, because it was not committed yet.
	// it will be committed and written to the database eventually, and then it can be pruned.

	if err := r.txRetainerDatabase.Prune(targetSlot); err != nil {
		return ierrors.Wrap(err, "failed to prune transaction retainer")
	}

	return nil
}

// CommitSlot applies all uncommitted changes of a slot from the cache to the database and deletes them from the cache.
func (r *TransactionRetainer) CommitSlot(slot iotago.SlotIndex) error {
	uncommittedChanges := r.txRetainerCache.DeleteAndReturnTxMetadataChangesBySlot(slot)
	if len(uncommittedChanges) == 0 {
		return nil
	}

	if err := r.txRetainerDatabase.ApplyTxMetadataChanges(uncommittedChanges); err != nil {
		return ierrors.Wrapf(err, "failed to commit slot: %d", slot)
	}

	return nil
}

// UpdateTransactionMetadata updates the metadata of a transaction.
// The changes will be applied to the cache and later to the database by "CommitSlot".
func (r *TransactionRetainer) UpdateTransactionMetadata(txID iotago.TransactionID, validSignature bool, earliestAttachmentSlot iotago.SlotIndex, state api.TransactionState, txErr error) {
	txFailureReason := api.TxFailureNone
	var txErrorMsg *string
	if txErr != nil {
		txFailureReason = api.DetermineTransactionFailureReason(txErr)

		if r.storeDebugErrorMessages {
			errorMsg := txErr.Error()
			txErrorMsg = &errorMsg
		}
	}

	txMeta := &TransactionMetadata{
		TransactionID:          txID[:],
		ValidSignature:         validSignature,
		EarliestAttachmentSlot: earliestAttachmentSlot,
		State:                  byte(state),
		FailureReason:          byte(txFailureReason),
		ErrorMsg:               txErrorMsg,
	}

	if err := r.txRetainerCache.UpdateTxMetadata(txMeta); err != nil {
		r.errorHandler(err)
	}
}

// TransactionMetadata returns the metadata of a transaction.
func (r *TransactionRetainer) TransactionMetadata(txID iotago.TransactionID) (*api.TransactionMetadataResponse, error) {
	// first check the cache, if there is an entry and it is accepted, we can return it without checking the database
	txMeta, cacheHit := r.txRetainerCache.TransactionMetadataByID(txID)
	if !cacheHit || txMeta.State != byte(api.TransactionStateAccepted) {
		// if the transaction is not in the cache or is not accepted, we need to check the database as well
		txMetaDatabase, err := r.txRetainerDatabase.TransactionMetadataByID(txID)
		if err != nil {
			return nil, err
		}

		// there was an entry in the database
		if txMetaDatabase != nil {
			// if one of the following conditions is met, we use the entry from the database:
			// - there was no entry in the cache
			// - the entry in the database is accepted
			// - the entry in the database has a valid signature and the entry in the cache does not
			if !cacheHit || txMetaDatabase.State == byte(api.TransactionStateAccepted) || (!txMeta.ValidSignature && txMetaDatabase.ValidSignature) {
				txMeta = txMetaDatabase
			}
		}
	}

	if txMeta == nil {
		return nil, ErrEntryNotFound
	}

	txFailureDetails := ""
	if txMeta.ErrorMsg != nil {
		txFailureDetails = *txMeta.ErrorMsg
	}

	response := &api.TransactionMetadataResponse{
		TransactionID:             txID,
		TransactionState:          api.TransactionState(txMeta.State),
		EarliestAttachmentSlot:    txMeta.EarliestAttachmentSlot,
		TransactionFailureReason:  api.TransactionFailureReason(txMeta.FailureReason),
		TransactionFailureDetails: txFailureDetails,
	}

	// for TransactionStateConfirmed and TransactionStateFinalized we need to check if
	// the slot of the earliest attachment is already confirmed or finalized
	if response.TransactionState == api.TransactionStateAccepted {
		switch {
		case response.EarliestAttachmentSlot <= r.finalizedSlotFunc():
			response.TransactionState = api.TransactionStateFinalized
		case response.EarliestAttachmentSlot <= r.latestCommittedSlotFunc():
			response.TransactionState = api.TransactionStateCommitted
		}
	}

	return response, nil
}

// Shutdown shuts down the TransactionRetainer.
func (r *TransactionRetainer) shutdown() {
	r.workerPool.Shutdown()

	r.StoppedEvent().Trigger()
}
