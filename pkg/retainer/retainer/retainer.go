package retainer

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/retainer"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/models"
)

type (
	BlockRetrieveFunc func(iotago.BlockID) (*model.Block, bool)
	BlockDiskFunc     func(iotago.SlotIndex) *prunable.Blocks
	FinalizedSlotFunc func() iotago.SlotIndex
	RetainerFunc      func(iotago.SlotIndex) *prunable.Retainer
)

// Retainer keeps and resolves all the information needed in the API and INX.
type Retainer struct {
	blockRetrieveFunc BlockRetrieveFunc
	blockDiskFunc     BlockDiskFunc
	finalizedSlotFunc FinalizedSlotFunc
	retainerFunc      RetainerFunc
	errorHandler      func(error)

	module.Module
}

// the retainer should store the "confirmed" flag of blocks between they got committed and finalized.
// this storage should have buckets and the confirmed info should be stored by simply setting the blockid.

// several intervals to prune => triggered by the pruning manager
//
//	=> the confirmed flag until it got finalized (is this always the same interval?)
//	=> the info about conflicting blocks (maybe 1 - 2 epochs)
//
// maybe also store the orphaned block there as well?

// always get metadata through blockID
// txconfirmed store (blockID)
// txPending store (error codes)

// get block status: go through stores to check status
// get tx status: confirmed store -> check slot index finalized? finalized -> pending store (error codes)

func New(retainerFunc RetainerFunc, blockRetrieveFunc BlockRetrieveFunc, blockDiskFunc BlockDiskFunc, finalizedSlotIndex FinalizedSlotFunc, errorHandler func(error)) *Retainer {
	return &Retainer{
		blockRetrieveFunc: blockRetrieveFunc,
		blockDiskFunc:     blockDiskFunc,
		finalizedSlotFunc: finalizedSlotIndex,
		retainerFunc:      retainerFunc,
		errorHandler:      errorHandler,
	}
}

// NewProvider creates a new SyncManager provider.
func NewProvider() module.Provider[*engine.Engine, retainer.Retainer] {
	return module.Provide(func(e *engine.Engine) retainer.Retainer {
		r := New(e.Storage.Retainer, e.Block, e.Storage.Blocks, e.Storage.Settings().LatestFinalizedSlot, e.ErrorHandler("retainer"))
		asyncOpt := event.WithWorkerPool(e.Workers.CreatePool("Retainer", 1))

		e.Events.BlockDAG.BlockAttached.Hook(func(b *blocks.Block) {
			_, isTx := b.Transaction()
			if err := r.onBlockAttached(b.ID(), isTx); err != nil {
				r.errorHandler(ierrors.Wrap(err, "failed to store on Block Attached in retainer"))
			}
		}, asyncOpt)

		e.Events.BlockGadget.BlockAccepted.Hook(func(b *blocks.Block) {
			if err := r.onBlockAccepted(b.ID()); err != nil {
				r.errorHandler(ierrors.Wrap(err, "failed to store on Block Accepted in retainer"))
			}
		}, asyncOpt)

		e.Events.BlockGadget.BlockConfirmed.Hook(func(b *blocks.Block) {
			if err := r.onBlockConfirmed(b.ID()); err != nil {
				r.errorHandler(ierrors.Wrap(err, "failed to store on Block Confirmed in retainer"))
			}
		}, asyncOpt)

		e.HookInitialized(func() {
			e.Ledger.OnTransactionAttached(func(transactionMetadata mempool.TransactionMetadata) {
				attachmentID := transactionMetadata.EarliestIncludedAttachment()

				if err := r.onTransactionAttached(attachmentID); err != nil {
					r.errorHandler(ierrors.Wrap(err, "failed to store on Transaction Attached in retainer"))
				}

				transactionMetadata.OnAccepted(func() {
					if slotIndex := attachmentID.Index(); slotIndex > 0 {
						if err := r.onTransactionConfirmed(attachmentID); err != nil {
							r.errorHandler(ierrors.Wrap(err, "failed to store on Transaction Accepted in retainer"))
						}
					}
				})

				transactionMetadata.OnEarliestIncludedAttachmentUpdated(func(prevBlock, newBlock iotago.BlockID) {
					if newBlock.Index() != prevBlock.Index() {
						if err := r.onAttachmentUpdated(prevBlock, newBlock); err != nil {
							r.errorHandler(ierrors.Wrap(err, "failed to delete/store on Attachment Updated in retainer"))
						}
					}
				})
			})
		})

		r.TriggerInitialized()

		return r
	})
}

func (r *Retainer) Shutdown() {
	r.retainerFunc = nil
}

func (r *Retainer) Block(blockID iotago.BlockID) (*model.Block, error) {
	block, _ := r.blockRetrieveFunc(blockID)
	if block == nil {
		return nil, ierrors.Errorf("block not found: %s", blockID.ToHex())
	}

	return block, nil
}

func (r *Retainer) BlockMetadata(blockID iotago.BlockID) (*retainer.BlockMetadata, error) {
	blockStatus, err := r.blockStatus(blockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get block status for %s", blockID.ToHex())
	}

	hasTx, txStatus, err := r.transactionStatus(blockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get transaction status for %s", blockID.ToHex())
	}

	// TODO: fill in blockReason, TxReason.

	return &retainer.BlockMetadata{
		BlockStatus:       blockStatus,
		HasTx:             hasTx,
		TransactionStatus: txStatus,
	}, nil
}

func (r *Retainer) blockStatus(blockID iotago.BlockID) (models.BlockState, error) {
	b, blockStorageErr := r.blockDiskFunc(blockID.Index()).Load(blockID)
	if blockStorageErr != nil {
		return models.BlockStateUnknown, ierrors.Wrap(blockStorageErr, "failed to get block state in retainer.")
	}

	// block was for sure accepted
	if b != nil {
		// check if finalized
		if blockID.Index() <= r.finalizedSlotFunc() {
			return models.BlockStateFinalized, nil
		}
		// check if confirmed
		if confirmed, err := r.retainerFunc(blockID.Index()).WasBlockConfirmed(blockID); err != nil {
			return models.BlockStateUnknown, ierrors.Wrap(err, "failed to get block confirmed state in retainer.")
		} else if confirmed {
			return models.BlockStateConfirmed, nil
		}
	}

	// orphaned (attached, but never accepeted)
	if orphaned, err := r.retainerFunc(blockID.Index()).WasBlockOrphaned(blockID); err != nil {
		return models.BlockStateUnknown, ierrors.Wrap(err, "failed to get block orphaned state in retainer.")
	} else if orphaned {
		return models.BlockStateOrphaned, nil
	}

	return models.BlockStateUnknown, ierrors.Errorf("failed to get block state in retainer.")
}

func (r *Retainer) transactionStatus(blockID iotago.BlockID) (bool, models.TransactionState, error) {
	b, blockStorageErr := r.blockDiskFunc(blockID.Index()).Load(blockID)
	if blockStorageErr != nil {
		return false, models.TransactionStateFailed, ierrors.Wrap(blockStorageErr, "failed to get transaction state in retainer.")
	}

	// block was for sure accepted
	if b != nil {
		// return if block does not have a transaction
		if _, hasTx := b.Transaction(); !hasTx {
			return false, models.TransactionStatePending, nil
		}

		if blockID.Index() <= r.finalizedSlotFunc() {
			return true, models.TransactionStateFinalized, nil
		}
		// check if confirmed
		if confirmed, err := r.retainerFunc(blockID.Index()).WasTransactionConfirmed(blockID); err != nil {
			return true, models.TransactionStateFailed, ierrors.Wrap(err, "failed to get transaction confirmed state in retainer.")
		} else if confirmed {
			return true, models.TransactionStateConfirmed, nil
		}
	}

	if pending, err := r.retainerFunc(blockID.Index()).WasTransactionPending(blockID); err != nil {
		return true, models.TransactionStateFailed, ierrors.Wrap(err, "failed to get transaction state in retainer.")
	} else if pending {
		return true, models.TransactionStatePending, nil
	}

	return true, models.TransactionStateFailed, ierrors.Errorf("failed to get transaction state in retainer.")
}

// TODO: trigger this if we have orphaned event
func (r *Retainer) onBlockAttached(blockID iotago.BlockID, isTx bool) error {
	retainerStore := r.retainerFunc(blockID.Index())
	return retainerStore.Store(blockID, isTx)
}

func (r *Retainer) onBlockAccepted(blockID iotago.BlockID) error {
	retainerStore := r.retainerFunc(blockID.Index())
	return retainerStore.StoreBlockAccepted(blockID)
}

func (r *Retainer) onBlockConfirmed(blockID iotago.BlockID) error {
	retainerStore := r.retainerFunc(blockID.Index())
	return retainerStore.StoreBlockConfirmed(blockID)
}

func (r *Retainer) onTransactionAttached(blockID iotago.BlockID) error {
	retainerStore := r.retainerFunc(blockID.Index())
	return retainerStore.StoreTransactionPending(blockID)
}

func (r *Retainer) onTransactionConfirmed(blockID iotago.BlockID) error {
	retainerStore := r.retainerFunc(blockID.Index())
	return retainerStore.StoreTransactionConfirmed(blockID)
}

func (r *Retainer) onAttachmentUpdated(prevID, newID iotago.BlockID) error {
	oldStore := r.retainerFunc(prevID.Index())
	err := oldStore.DeleteTransactionConfirmed(prevID)
	if err != nil {
		return err
	}

	newStore := r.retainerFunc(newID.Index())
	return newStore.StoreTransactionConfirmed(newID)
}
