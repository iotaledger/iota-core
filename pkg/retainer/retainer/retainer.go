package retainer

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/retainer"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

type (
	//nolint:revive
	RetainerFunc            func(iotago.SlotIndex) *slotstore.Retainer
	LatestCommittedSlotFunc func() iotago.SlotIndex
	FinalizedSlotFunc       func() iotago.SlotIndex
)

// Retainer keeps and resolves all the information needed in the API and INX.
type Retainer struct {
	store                   RetainerFunc
	latestCommittedSlotFunc LatestCommittedSlotFunc
	finalizedSlotFunc       FinalizedSlotFunc
	errorHandler            func(error)

	workers *workerpool.Group

	module.Module
}

func New(workers *workerpool.Group, retainerFunc RetainerFunc, latestCommittedSlotFunc LatestCommittedSlotFunc, finalizedSlotFunc FinalizedSlotFunc, errorHandler func(error)) *Retainer {
	return &Retainer{
		workers:                 workers,
		store:                   retainerFunc,
		latestCommittedSlotFunc: latestCommittedSlotFunc,
		finalizedSlotFunc:       finalizedSlotFunc,
		errorHandler:            errorHandler,
	}
}

// NewProvider creates a new Retainer provider.
func NewProvider() module.Provider[*engine.Engine, retainer.Retainer] {
	return module.Provide(func(e *engine.Engine) retainer.Retainer {
		r := New(e.Workers.CreateGroup("Retainer"),
			e.Storage.Retainer,
			e.Storage.Settings().LatestCommitment().Index,
			e.Storage.Settings().LatestFinalizedSlot,
			e.ErrorHandler("retainer"))

		asyncOpt := event.WithWorkerPool(r.workers.CreatePool("Retainer", 1))

		e.Events.BlockDAG.BlockAttached.Hook(func(b *blocks.Block) {
			if err := r.onBlockAttached(b.ID()); err != nil {
				r.errorHandler(ierrors.Wrap(err, "failed to store on BlockAttached in retainer"))
			}
		}, asyncOpt)

		e.Events.BlockGadget.BlockAccepted.Hook(func(b *blocks.Block) {
			if err := r.onBlockAccepted(b.ID()); err != nil {
				r.errorHandler(ierrors.Wrap(err, "failed to store on BlockAccepted in retainer"))
			}
		}, asyncOpt)

		e.Events.BlockGadget.BlockConfirmed.Hook(func(b *blocks.Block) {
			if err := r.onBlockConfirmed(b.ID()); err != nil {
				r.errorHandler(ierrors.Wrap(err, "failed to store on BlockConfirmed in retainer"))
			}
		}, asyncOpt)

		e.Events.Scheduler.BlockDropped.Hook(func(b *blocks.Block, err error) {
			r.RetainBlockFailure(b.ID(), apimodels.BlockFailureDroppedDueToCongestion)
		})

		e.HookInitialized(func() {
			e.Ledger.OnTransactionAttached(func(transactionMetadata mempool.TransactionMetadata) {

				// transaction is not included yet, thus EarliestIncludedAttachment is not set.
				if err := r.onTransactionAttached(transactionMetadata.Attachments()[0]); err != nil {
					r.errorHandler(ierrors.Wrap(err, "failed to store on TransactionAttached in retainer"))
				}

				transactionMetadata.OnConflicting(func() {
					// transaction is not included yet, thus EarliestIncludedAttachment is not set.
					r.RetainTransactionFailure(transactionMetadata.Attachments()[0], iotago.ErrTxConflicting)
				})

				transactionMetadata.OnInvalid(func(err error) {
					// transaction is not included yet, thus EarliestIncludedAttachment is not set.
					r.RetainTransactionFailure(transactionMetadata.Attachments()[0], err)
				})

				transactionMetadata.OnAccepted(func() {
					attachmentID := transactionMetadata.EarliestIncludedAttachment()
					if slotIndex := attachmentID.Index(); slotIndex > 0 {
						if err := r.onTransactionAccepted(attachmentID); err != nil {
							r.errorHandler(ierrors.Wrap(err, "failed to store on TransactionAccepted in retainer"))
						}
					}
				})

				transactionMetadata.OnEarliestIncludedAttachmentUpdated(func(prevBlock, newBlock iotago.BlockID) {
					// if prevBlock is genesis, we do not need to update anything, bc the tx is included in the block we attached to at start.
					if prevBlock.Index() == 0 {
						return
					}

					if err := r.onAttachmentUpdated(prevBlock, newBlock, transactionMetadata.IsAccepted()); err != nil {
						r.errorHandler(ierrors.Wrap(err, "failed to delete/store on AttachmentUpdated in retainer"))
					}
				})

			})
		})

		r.TriggerInitialized()

		return r
	})
}

func (r *Retainer) Shutdown() {
	r.workers.Shutdown()
}

func (r *Retainer) BlockMetadata(blockID iotago.BlockID) (*retainer.BlockMetadata, error) {
	blockStatus, blockFailureReason := r.blockStatus(blockID)
	if blockStatus == apimodels.BlockStateUnknown {
		return nil, ierrors.Errorf("block %s not found", blockID.ToHex())
	}

	// we do not expose accepted flag
	if blockStatus == apimodels.BlockStateAccepted {
		blockStatus = apimodels.BlockStatePending
	}

	txStatus, txFailureReason := r.transactionStatus(blockID)

	return &retainer.BlockMetadata{
		BlockID:            blockID,
		BlockState:         blockStatus,
		BlockFailureReason: blockFailureReason,
		TxState:            txStatus,
		TxFailureReason:    txFailureReason,
	}, nil
}

func (r *Retainer) RetainBlockFailure(blockID iotago.BlockID, failureCode apimodels.BlockFailureReason) {
	if err := r.store(blockID.Index()).StoreBlockFailure(blockID, failureCode); err != nil {
		r.errorHandler(ierrors.Wrap(err, "failed to store block failure in retainer"))
	}
}

func (r *Retainer) RetainTransactionFailure(blockID iotago.BlockID, err error) {
	if err := r.store(blockID.Index()).StoreTransactionFailure(blockID, determineTxFailureReason(err)); err != nil {
		r.errorHandler(ierrors.Wrap(err, "failed to store transaction failure in retainer"))
	}
}

func (r *Retainer) blockStatus(blockID iotago.BlockID) (apimodels.BlockState, apimodels.BlockFailureReason) {
	blockData, exists := r.store(blockID.Index()).GetBlock(blockID)
	if !exists {
		return apimodels.BlockStateUnknown, apimodels.BlockFailureNone
	}
	switch blockData.State {
	case apimodels.BlockStatePending:
		if blockID.Index() <= r.latestCommittedSlotFunc() {
			return apimodels.BlockStateRejected, blockData.FailureReason
		}
	case apimodels.BlockStateAccepted, apimodels.BlockStateConfirmed:
		if blockID.Index() <= r.finalizedSlotFunc() {
			return apimodels.BlockStateFinalized, apimodels.BlockFailureNone
		}
	}

	return blockData.State, blockData.FailureReason
}

func (r *Retainer) transactionStatus(blockID iotago.BlockID) (apimodels.TransactionState, apimodels.TransactionFailureReason) {
	txData, exists := r.store(blockID.Index()).GetTransaction(blockID)
	if !exists {
		return apimodels.TransactionStateNoTransaction, apimodels.TxFailureNone
	}

	// for confirmed and finalized we need to check for the block status
	if txData.State == apimodels.TransactionStateAccepted {
		blockState, _ := r.blockStatus(blockID)

		switch blockState {
		case apimodels.BlockStateConfirmed:
			return apimodels.TransactionStateConfirmed, apimodels.TxFailureNone
		case apimodels.BlockStateFinalized:
			return apimodels.TransactionStateFinalized, apimodels.TxFailureNone
		}
	}

	return txData.State, txData.FailureReason
}

func (r *Retainer) onBlockAttached(blockID iotago.BlockID) error {
	return r.store(blockID.Index()).StoreBlockAttached(blockID)
}

func (r *Retainer) onBlockAccepted(blockID iotago.BlockID) error {
	return r.store(blockID.Index()).StoreBlockAccepted(blockID)
}

func (r *Retainer) onBlockConfirmed(blockID iotago.BlockID) error {
	return r.store(blockID.Index()).StoreBlockConfirmed(blockID)
}

func (r *Retainer) onTransactionAttached(blockID iotago.BlockID) error {
	return r.store(blockID.Index()).StoreTransactionNoFailureStatus(blockID, apimodels.TransactionStatePending)
}

func (r *Retainer) onTransactionAccepted(blockID iotago.BlockID) error {
	return r.store(blockID.Index()).StoreTransactionNoFailureStatus(blockID, apimodels.TransactionStateAccepted)
}

func (r *Retainer) onAttachmentUpdated(prevID, newID iotago.BlockID, accepted bool) error {
	if err := r.store(prevID.Index()).DeleteTransactionData(prevID); err != nil {
		return err
	}

	if accepted {
		return r.store(newID.Index()).StoreTransactionNoFailureStatus(newID, apimodels.TransactionStateAccepted)
	}

	return r.store(newID.Index()).StoreTransactionNoFailureStatus(newID, apimodels.TransactionStatePending)
}
