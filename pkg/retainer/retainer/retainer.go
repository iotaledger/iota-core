package retainer

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/retainer"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

type (
	//nolint:revive
	RetainerFunc            func(iotago.SlotIndex) *prunable.Retainer
	BlockFromCacheFunc      func(iotago.BlockID) *model.Block
	BlockFromStorageFunc    func(iotago.BlockID) (*model.Block, error)
	LatestCommittedSlotFunc func() iotago.SlotIndex
	FinalizedSlotFunc       func() iotago.SlotIndex
)

// Retainer keeps and resolves all the information needed in the API and INX.
type Retainer struct {
	retainerFunc            RetainerFunc
	blockFromCacheFunc      BlockFromCacheFunc
	blockFromStorageFunc    BlockFromStorageFunc
	latestCommittedSlotFunc LatestCommittedSlotFunc
	finalizedSlotFunc       FinalizedSlotFunc
	errorHandler            func(error)

	workers *workerpool.Group

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

func New(workers *workerpool.Group, retainerFunc RetainerFunc, blockFromCacheFunc BlockFromCacheFunc, blockFromStorageFunc BlockFromStorageFunc, latestCommittedSlotFunc LatestCommittedSlotFunc, finalizedSlotFunc FinalizedSlotFunc, errorHandler func(error)) *Retainer {
	return &Retainer{
		workers:                 workers,
		retainerFunc:            retainerFunc,
		blockFromCacheFunc:      blockFromCacheFunc,
		blockFromStorageFunc:    blockFromStorageFunc,
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
			func(blockID iotago.BlockID) *model.Block {
				block, exists := e.BlockFromCache(blockID)
				if !exists {
					return nil
				}

				return block.ModelBlock()
			},
			func(blockID iotago.BlockID) (*model.Block, error) {
				storage := e.Storage.Blocks(blockID.Index())
				if storage == nil {
					return nil, ierrors.Errorf("failed to get storage for block %s in retainer", blockID)
				}

				block, err := e.Storage.Blocks(blockID.Index()).Load(blockID)
				if err != nil {
					return nil, ierrors.Wrap(err, "failed to get block from storage in retainer")
				}

				if block == nil {
					return nil, ierrors.Errorf("failed to get block from storage in retainer, block not found: %s", blockID)
				}

				return block, nil
			}, e.Storage.Settings().LatestFinalizedSlot,
			e.Storage.Settings().LatestCommitment().Index,
			e.ErrorHandler("retainer"))

		asyncOpt := event.WithWorkerPool(r.workers.CreatePool("Retainer", 1))

		e.Events.BlockDAG.BlockAttached.Hook(func(b *blocks.Block) {
			_, isTx := b.Transaction()
			if err := r.onBlockAttached(b.ID(), isTx); err != nil {
				r.errorHandler(ierrors.Wrap(err, "failed to store on BlockAttached in retainer"))
			}
		}, asyncOpt)

		// TODO: if we hook and collect BlockFailure on blockDropped, then we should remove errors on acceptance, cause block can be dropped in one node but still accepted by the network
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

		e.HookInitialized(func() {
			e.Ledger.OnTransactionAttached(func(transactionMetadata mempool.TransactionMetadata) {
				attachmentID := transactionMetadata.EarliestIncludedAttachment()

				if err := r.onTransactionAttached(attachmentID); err != nil {
					r.errorHandler(ierrors.Wrap(err, "failed to store on TransactionAttached in retainer"))
				}

				transactionMetadata.OnAccepted(func() {
					if slotIndex := attachmentID.Index(); slotIndex > 0 {
						if err := r.onTransactionAccepted(attachmentID); err != nil {
							r.errorHandler(ierrors.Wrap(err, "failed to store on TransactionAccepted in retainer"))
						}
					}
				})

				transactionMetadata.OnEarliestIncludedAttachmentUpdated(func(prevBlock, newBlock iotago.BlockID) {
					if newBlock.Index() != prevBlock.Index() {
						if err := r.onAttachmentUpdated(prevBlock, newBlock); err != nil {
							r.errorHandler(ierrors.Wrap(err, "failed to delete/store on AttachmentUpdated in retainer"))
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
	r.workers.Shutdown()
}

func (r *Retainer) BlockMetadata(blockID iotago.BlockID) (*retainer.BlockMetadata, error) {
	blockStatus, block, err := r.blockStatus(blockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get block status for %s", blockID.ToHex())
	}

	hasTx, txStatus, err := r.transactionStatus(blockID, block)
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

func (r *Retainer) RetainBlockFailure(blockID iotago.BlockID, failureCode apimodels.BlockFailureReason) {
	retainerStore := r.retainerFunc(blockID.Index())
	_ = retainerStore.StoreBlockFailure(blockID, failureCode)
}

func (r *Retainer) RetainTransactionFailure(transactionID iotago.TransactionID, slotIndex iotago.SlotIndex, failureCode apimodels.TransactionFailureReason) {
	retainerStore := r.retainerFunc(slotIndex)
	_ = retainerStore.StoreTransactionFailure(transactionID, failureCode)
}

func (r *Retainer) blockStatus(blockID iotago.BlockID) (apimodels.BlockState, *model.Block, error) {
	// we store the block on acceptance, so if block is nil and slot was already committed we know its orphaned or never existed
	block, err := r.blockFromStorageFunc(blockID)
	if err != nil {
		return 0, nil, err
	}

	// block was for sure accepted in this chain
	if block != nil {
		// check if finalized in this chain
		if blockID.Index() <= r.finalizedSlotFunc() {
			return apimodels.BlockStateFinalized, block, nil
		}

		// check if confirmed
		if confirmed, err := r.retainerFunc(blockID.Index()).WasBlockConfirmed(blockID); err != nil {
			return apimodels.BlockStateUnknown, nil, ierrors.Wrap(err, "failed to get block confirmed state in retainer.")
		} else if confirmed {
			return apimodels.BlockStateConfirmed, block, nil
		}

		return apimodels.BlockStatePending, block, nil
	}

	if blockID.Index() > r.latestCommittedSlotFunc() {
		// orphaned (attached, but never accepted)
		if orphaned, err := r.retainerFunc(blockID.Index()).WasBlockOrphaned(blockID); err != nil {
			return apimodels.BlockStateUnknown, nil, ierrors.Wrap(err, "failed to get block orphaned state in retainer.")
		} else if orphaned {
			return apimodels.BlockStateOrphaned, nil, nil
		}
	}

	if block := r.blockFromCacheFunc(blockID); block != nil {
		return apimodels.BlockStatePending, block, nil
	}

	return apimodels.BlockStateUnknown, nil, nil
}

func (r *Retainer) transactionStatus(blockID iotago.BlockID, block *model.Block) (bool, apimodels.TransactionState, error) {
	// block was for sure accepted
	if block != nil {
		// return if block does not have a transaction
		if _, hasTx := block.Transaction(); !hasTx {
			return false, apimodels.TransactionStatePending, nil
		}

		if blockID.Index() <= r.finalizedSlotFunc() {
			return true, apimodels.TransactionStateFinalized, nil
		}
		// check if confirmed
		if confirmed, err := r.retainerFunc(blockID.Index()).WasTransactionConfirmed(blockID); err != nil {
			return true, apimodels.TransactionStateFailed, ierrors.Wrap(err, "failed to get transaction confirmed state in retainer.")
		} else if confirmed {
			return true, apimodels.TransactionStateConfirmed, nil
		}
	}

	if pending, err := r.retainerFunc(blockID.Index()).WasTransactionPending(blockID); err != nil {
		return true, apimodels.TransactionStateFailed, ierrors.Wrap(err, "failed to get transaction pending state in retainer.")
	} else if pending {
		return true, apimodels.TransactionStatePending, nil
	}

	return true, apimodels.TransactionStateFailed, ierrors.Errorf("failed to get transaction state in retainer.")
}

// TODO: trigger this if we have orphaned event.
func (r *Retainer) onBlockAttached(blockID iotago.BlockID, isTx bool) error {
	return r.retainerFunc(blockID.Index()).Store(blockID, isTx)
}

func (r *Retainer) onBlockAccepted(blockID iotago.BlockID) error {
	return r.retainerFunc(blockID.Index()).StoreBlockAccepted(blockID)
}

func (r *Retainer) onBlockConfirmed(blockID iotago.BlockID) error {
	return r.retainerFunc(blockID.Index()).StoreBlockConfirmed(blockID)
}

func (r *Retainer) onTransactionAttached(blockID iotago.BlockID) error {
	return r.retainerFunc(blockID.Index()).StoreTransactionPending(blockID)
}

func (r *Retainer) onTransactionAccepted(blockID iotago.BlockID) error {
	return r.retainerFunc(blockID.Index()).StoreTransactionConfirmed(blockID)
}

func (r *Retainer) onAttachmentUpdated(prevID, newID iotago.BlockID) error {
	// delete the old transactionID
	if err := r.retainerFunc(prevID.Index()).DeleteTransactionConfirmed(prevID); err != nil {
		return err
	}

	// store the new transactionID
	return r.retainerFunc(newID.Index()).StoreTransactionConfirmed(newID)
}
