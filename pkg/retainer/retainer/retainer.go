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
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

type (
	//nolint:revive
	RetainerFunc            func(iotago.SlotIndex) *prunable.Retainer
	LatestCommittedSlotFunc func() iotago.SlotIndex
	FinalizedSlotFunc       func() iotago.SlotIndex
)

// Retainer keeps and resolves all the information needed in the API and INX.
type Retainer struct {
	retainerFunc            RetainerFunc
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

func New(workers *workerpool.Group, currentAPI func(index iotago.SlotIndex) iotago.API, retainerFunc RetainerFunc, latestCommittedSlotFunc LatestCommittedSlotFunc, finalizedSlotFunc FinalizedSlotFunc, errorHandler func(error)) *Retainer {
	return &Retainer{
		workers:                 workers,
		retainerFunc:            retainerFunc,
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
			e.Storage.Settings().LatestFinalizedSlot,
			e.Storage.Settings().LatestCommitment().Index,
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
			r.RetainBlockFailure(b.ID(), apimodels.ErrBlockDroppedDueToCongestion)
		})

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
					if err := r.onAttachmentUpdated(prevBlock, newBlock, transactionMetadata.IsAccepted()); err != nil {
						r.errorHandler(ierrors.Wrap(err, "failed to delete/store on AttachmentUpdated in retainer"))
					}
				})

				transactionMetadata.OnInvalid(func(err error) {
					// TODO: determine the error code
					// r.RetainTransactionFailure(attachmentID, apimodels.ErrTxStateChainStateTransitionInvalid)
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

func (r *Retainer) BlockMetadata(blockID iotago.BlockID) (*apimodels.BlockMetadataResponse, error) {
	blockStatus, blockFailureReason, err := r.blockStatus(blockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get block status for %s", blockID.ToHex())
	}
	// we do not expose accepted flag
	if blockStatus == apimodels.BlockStateAccepted {
		blockStatus = apimodels.BlockStatePending
	}

	txStatus, txFailureReason, err := r.transactionStatus(blockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get transaction status for %s", blockID.ToHex())
	}
	resp := &apimodels.BlockMetadataResponse{
		BlockID:            blockID.ToHex(),
		BlockState:         blockStatus.String(),
		BlockFailureReason: blockFailureReason,
	}
	if txStatus != apimodels.TransactionStateUnknown {
		resp.TxState = txStatus.String()
		resp.TxFailureReason = txFailureReason
	}

	return resp, nil
}

func (r *Retainer) RetainBlockFailure(blockID iotago.BlockID, failureCode apimodels.BlockFailureReason) {
	retainerStore := r.retainerFunc(blockID.Index())
	_ = retainerStore.StoreBlockFailure(blockID, failureCode)
}

func (r *Retainer) RetainTransactionFailure(blockID iotago.BlockID, failureCode apimodels.TransactionFailureReason) {
	retainerStore := r.retainerFunc(blockID.Index())
	_ = retainerStore.StoreTransactionFailure(blockID, failureCode)
}

func (r *Retainer) blockStatus(blockID iotago.BlockID) (apimodels.BlockState, apimodels.BlockFailureReason, error) {
	blockData, exists := r.retainerFunc(blockID.Index()).GetBlock(blockID)
	if !exists {
		return apimodels.BlockStateUnknown, apimodels.NoBlockFailureReason, nil
	}
	switch blockData.State {
	case apimodels.BlockStatePending:
		if blockID.Index() <= r.latestCommittedSlotFunc() {
			return apimodels.BlockStateOrphaned, blockData.FailureReason, nil
		}
	case apimodels.BlockStateAccepted, apimodels.BlockStateConfirmed:
		if blockID.Index() <= r.finalizedSlotFunc() {
			return apimodels.BlockStateFinalized, apimodels.NoBlockFailureReason, nil
		}
	}

	return blockData.State, blockData.FailureReason, nil
}

func (r *Retainer) transactionStatus(blockID iotago.BlockID) (apimodels.TransactionState, apimodels.TransactionFailureReason, error) {
	txData, exists := r.retainerFunc(blockID.Index()).GetTransaction(blockID)
	if !exists {
		return apimodels.TransactionStateUnknown, apimodels.NoTransactionFailureReason, nil
	}

	// for confirmed and finalized we need to check for the block status
	if txData.State == apimodels.TransactionStateAccepted {
		blockState, _, err := r.blockStatus(blockID)
		if err != nil {
			return apimodels.TransactionStateUnknown, apimodels.NoTransactionFailureReason, err
		}

		switch blockState {
		case apimodels.BlockStateConfirmed:
			return apimodels.TransactionStateConfirmed, apimodels.NoTransactionFailureReason, nil
		case apimodels.BlockStateFinalized:
			return apimodels.TransactionStateFinalized, apimodels.NoTransactionFailureReason, nil
		}
	}

	return txData.State, txData.FailureReason, nil
}

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

func (r *Retainer) onAttachmentUpdated(prevID, newID iotago.BlockID, accepted bool) error {
	txData, exists := r.retainerFunc(prevID.Index()).GetTransaction(prevID)
	// delete the old transactionID
	if exists {
		if err := r.retainerFunc(prevID.Index()).DeleteTransactionData(prevID); err != nil {
			return err
		}

		return r.retainerFunc(newID.Index()).StoreTransactionNoFailureStatus(newID, txData.State)
	}

	if accepted {
		return r.retainerFunc(newID.Index()).StoreTransactionNoFailureStatus(newID, apimodels.TransactionStateAccepted)
	}

	return r.retainerFunc(newID.Index()).StoreTransactionNoFailureStatus(newID, apimodels.TransactionStatePending)
}
