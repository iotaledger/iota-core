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

	api     func(index iotago.SlotIndex) iotago.API
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

func New(workers *workerpool.Group, currentAPI func(index iotago.SlotIndex) iotago.API, retainerFunc RetainerFunc, blockFromCacheFunc BlockFromCacheFunc, blockFromStorageFunc BlockFromStorageFunc, latestCommittedSlotFunc LatestCommittedSlotFunc, finalizedSlotFunc FinalizedSlotFunc, errorHandler func(error)) *Retainer {
	return &Retainer{
		workers:                 workers,
		retainerFunc:            retainerFunc,
		blockFromCacheFunc:      blockFromCacheFunc,
		blockFromStorageFunc:    blockFromStorageFunc,
		latestCommittedSlotFunc: latestCommittedSlotFunc,
		finalizedSlotFunc:       finalizedSlotFunc,
		errorHandler:            errorHandler,
		api:                     currentAPI,
	}
}

// NewProvider creates a new Retainer provider.
func NewProvider() module.Provider[*engine.Engine, retainer.Retainer] {
	return module.Provide(func(e *engine.Engine) retainer.Retainer {
		r := New(e.Workers.CreateGroup("Retainer"),
			e.Storage.Settings().APIForSlot,
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
			if err := r.onBlockAttached(b.ID()); err != nil {
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
					if err := r.onAttachmentUpdated(prevBlock, newBlock); err != nil {
						r.errorHandler(ierrors.Wrap(err, "failed to delete/store on AttachmentUpdated in retainer"))
					}
				})

				// TODO attach on invalid transactions, add error codes and store to retainer
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
	blockStatus, err := r.blockStatus(blockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get block status for %s", blockID.ToHex())
	}
	// we do not expose accepted flag
	if blockStatus == apimodels.BlockStateAccepted {
		blockStatus = apimodels.BlockStatePending
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

func (r *Retainer) RetainBlockFailure(blockID iotago.BlockID, failureCode apimodels.BlockFailureReason) {
	retainerStore := r.retainerFunc(blockID.Index())
	_ = retainerStore.StoreBlockFailure(blockID, failureCode)
}

func (r *Retainer) RetainTransactionFailure(transactionID iotago.TransactionID, slotIndex iotago.SlotIndex, failureCode apimodels.TransactionFailureReason) {
	retainerStore := r.retainerFunc(slotIndex)
	_ = retainerStore.StoreTransactionFailure(transactionID, failureCode)
}

func (r *Retainer) blockStatus(blockID iotago.BlockID) (apimodels.BlockState, error) {
	blockData, exists := r.retainerFunc(blockID.Index()).GetBlock(blockID)
	if !exists {
		return apimodels.BlockStateUnknown, nil
	}
	// block was for sure accepted in this chain
	if blockData.State == apimodels.BlockStateAccepted || blockData.State == apimodels.BlockStateConfirmed {
		// check if finalized in this chain
		if blockID.Index() <= r.finalizedSlotFunc() {
			return apimodels.BlockStateFinalized, nil
		}
		return blockData.State, nil
	}
	if blockData.State == apimodels.BlockStatePending && blockID.Index() <= r.latestCommittedSlotFunc() {
		return apimodels.BlockStateOrphaned, nil
	}

	return apimodels.BlockStatePending, nil
}

func (r *Retainer) transactionStatus(blockID iotago.BlockID) (bool, apimodels.TransactionState, error) {
	txData, exists := r.retainerFunc(blockID.Index()).GetTransaction(blockID)
	if !exists {
		return false, apimodels.TransactionStateUnknown, nil
	}
	// we update tx status up to accepted, for conf anf finalization we need to check for the block status
	if txData.State != apimodels.TransactionStateAccepted {
		return false, apimodels.TransactionStatePending, nil
	}

	blockData, exists := r.retainerFunc(blockID.Index()).GetBlock(blockID)
	if !exists {
		return false, apimodels.TransactionStateUnknown, nil
	}

	switch blockData.State {
	case apimodels.BlockStateConfirmed:
		return true, apimodels.TransactionStateConfirmed, nil
	case apimodels.BlockStateFinalized:
		return true, apimodels.TransactionStateFinalized, nil
	}

	return true, apimodels.TransactionStatePending, ierrors.Errorf("failed to get transaction state in retainer.")
}

// todo: check if we are on the same engine
// TODO: trigger this if we have orphaned event.
func (r *Retainer) onBlockAttached(blockID iotago.BlockID) error {
	return r.retainerFunc(blockID.Index()).StoreBlockAttached(blockID)
}

func (r *Retainer) onBlockAccepted(blockID iotago.BlockID) error {
	return r.retainerFunc(blockID.Index()).StoreBlockAccepted(blockID)
}

func (r *Retainer) onBlockConfirmed(blockID iotago.BlockID) error {
	return r.retainerFunc(blockID.Index()).StoreBlockConfirmed(blockID)
}

func (r *Retainer) onTransactionAttached(blockID iotago.BlockID) error {
	return r.retainerFunc(blockID.Index()).StoreTransactionNoFailureStatus(blockID, apimodels.TransactionStatePending)
}

func (r *Retainer) onTransactionAccepted(blockID iotago.BlockID) error {
	return r.retainerFunc(blockID.Index()).StoreTransactionNoFailureStatus(blockID, apimodels.TransactionStateAccepted)
}

func (r *Retainer) onAttachmentUpdated(prevID, newID iotago.BlockID) error {

	// delete the old transactionID
	if err := r.retainerFunc(prevID.Index()).DeleteTransactionData(prevID); err != nil {
		return err
	}
	// TODO: why it was always confirmed? Does the attachement update event is triggered ony on confirmed blocK?
	// store the new transactionID
	return r.retainerFunc(newID.Index()).StoreTransactionNoFailureStatus(newID, apimodels.TransactionStateConfirmed)
}
