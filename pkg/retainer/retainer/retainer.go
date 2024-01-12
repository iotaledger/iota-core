package retainer

import (
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/postsolidfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/retainer"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type (
	//nolint:revive
	StoreFunc               func(iotago.SlotIndex) (*slotstore.Retainer, error)
	LatestCommittedSlotFunc func() iotago.SlotIndex
	FinalizedSlotFunc       func() iotago.SlotIndex
)

const maxStakersResponsesCacheNum = 10

// Retainer keeps and resolves all the information needed in the API and INX.
type Retainer struct {
	store                   StoreFunc
	latestCommittedSlotFunc LatestCommittedSlotFunc
	finalizedSlotFunc       FinalizedSlotFunc
	errorHandler            func(error)

	stakersResponses *shrinkingmap.ShrinkingMap[uint32, []*api.ValidatorResponse]

	workerPool *workerpool.WorkerPool

	module.Module
}

func New(workersGroup *workerpool.Group, retainerStoreFunc StoreFunc, latestCommittedSlotFunc LatestCommittedSlotFunc, finalizedSlotFunc FinalizedSlotFunc, errorHandler func(error)) *Retainer {
	return &Retainer{
		workerPool:              workersGroup.CreatePool("Retainer", workerpool.WithWorkerCount(1)),
		store:                   retainerStoreFunc,
		stakersResponses:        shrinkingmap.New[uint32, []*api.ValidatorResponse](),
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
			e.Storage.Settings().LatestCommitment().Slot,
			e.Storage.Settings().LatestFinalizedSlot,
			e.ErrorHandler("retainer"))

		asyncOpt := event.WithWorkerPool(r.workerPool)

		e.Events.BlockDAG.BlockAttached.Hook(func(b *blocks.Block) {
			if err := r.onBlockAttached(b.ID()); err != nil {
				r.errorHandler(ierrors.Wrap(err, "failed to store on BlockAttached in retainer"))
			}
		}, asyncOpt)

		e.Events.PostSolidFilter.BlockFiltered.Hook(func(e *postsolidfilter.BlockFilteredEvent) {
			r.RetainBlockFailure(e.Block.ID(), determineBlockFailureReason(e.Reason))
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
			r.RetainBlockFailure(b.ID(), api.BlockFailureDroppedDueToCongestion)
		})

		e.Initialized.OnTrigger(func() {
			e.Ledger.MemPool().OnSignedTransactionAttached(func(signedTransactionMetadata mempool.SignedTransactionMetadata) {
				attachment := signedTransactionMetadata.Attachments()[0]

				signedTransactionMetadata.OnSignaturesInvalid(func(err error) {
					r.RetainTransactionFailure(attachment, err)
				})

				signedTransactionMetadata.OnSignaturesValid(func() {
					transactionMetadata := signedTransactionMetadata.TransactionMetadata()

					// transaction is not included yet, thus EarliestIncludedAttachment is not set.
					if err := r.onTransactionAttached(attachment); err != nil {
						r.errorHandler(ierrors.Wrap(err, "failed to store on TransactionAttached in retainer"))
					}

					transactionMetadata.OnInvalid(func(err error) {
						// transaction is not included yet, thus EarliestIncludedAttachment is not set.
						r.RetainTransactionFailure(attachment, err)
					})

					transactionMetadata.OnRejected(func() {
						r.RetainTransactionFailure(attachment, iotago.ErrTxConflicting)
					})

					transactionMetadata.OnAccepted(func() {
						attachmentID := transactionMetadata.EarliestIncludedAttachment()
						if slot := attachmentID.Slot(); slot > 0 {
							if err := r.onTransactionAccepted(attachmentID); err != nil {
								r.errorHandler(ierrors.Wrap(err, "failed to store on TransactionAccepted in retainer"))
							}
						}
					})

					transactionMetadata.OnEarliestIncludedAttachmentUpdated(func(prevBlock iotago.BlockID, newBlock iotago.BlockID) {
						// if prevBlock is genesis, we do not need to update anything, bc the tx is included in the block we attached to at start.
						if prevBlock.Slot() == 0 {
							return
						}

						if err := r.onAttachmentUpdated(prevBlock, newBlock, transactionMetadata.IsAccepted()); err != nil {
							r.errorHandler(ierrors.Wrap(err, "failed to delete/store on AttachmentUpdated in retainer"))
						}
					})
				})
			})
		})

		r.TriggerInitialized()

		return r
	})
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (r *Retainer) Reset() {
	// TODO: check if something needs to be cleaned here (author of the retainer)
}

func (r *Retainer) Shutdown() {
	r.workerPool.Shutdown()
}

func (r *Retainer) BlockMetadata(blockID iotago.BlockID) (*retainer.BlockMetadata, error) {
	blockStatus, blockFailureReason := r.blockStatus(blockID)
	if blockStatus == api.BlockStateUnknown {
		return nil, ierrors.Errorf("block %s not found", blockID.ToHex())
	}

	// we do not expose accepted flag
	if blockStatus == api.BlockStateAccepted {
		blockStatus = api.BlockStatePending
	}

	txID, txStatus, txFailureReason := r.transactionStatus(blockID)

	return &retainer.BlockMetadata{
		BlockID:                  blockID,
		BlockState:               blockStatus,
		BlockFailureReason:       blockFailureReason,
		TransactionID:            txID,
		TransactionState:         txStatus,
		TransactionFailureReason: txFailureReason,
	}, nil
}

func (r *Retainer) RetainBlockFailure(blockID iotago.BlockID, failureCode api.BlockFailureReason) {
	store, err := r.store(blockID.Slot())
	if err != nil {
		r.errorHandler(ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot()))
		return
	}

	if err := store.StoreBlockFailure(blockID, failureCode); err != nil {
		r.errorHandler(ierrors.Wrap(err, "failed to store block failure in retainer"))
	}
}

func (r *Retainer) RetainTransactionFailure(blockID iotago.BlockID, err error) {
	store, storeErr := r.store(blockID.Slot())
	if storeErr != nil {
		r.errorHandler(ierrors.Wrapf(storeErr, "could not get retainer store for slot %d", blockID.Slot()))
		return
	}

	if err := store.StoreTransactionFailure(blockID, determineTxFailureReason(err)); err != nil {
		r.errorHandler(ierrors.Wrap(err, "failed to store transaction failure in retainer"))
	}
}

func (r *Retainer) RegisteredValidatorsCache(index uint32) ([]*api.ValidatorResponse, bool) {
	return r.stakersResponses.Get(index)
}

func (r *Retainer) RetainRegisteredValidatorsCache(index uint32, resp []*api.ValidatorResponse) {
	r.stakersResponses.Set(index, resp)
	if r.stakersResponses.Size() > maxStakersResponsesCacheNum {
		keys := r.stakersResponses.Keys()
		minKey := index + 1
		for _, key := range keys {
			if key < minKey {
				minKey = key
			}
		}
		r.stakersResponses.Delete(minKey)
	}
}

func (r *Retainer) blockStatus(blockID iotago.BlockID) (api.BlockState, api.BlockFailureReason) {
	store, err := r.store(blockID.Slot())
	if err != nil {
		r.errorHandler(ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot()))
		return api.BlockStateUnknown, api.BlockFailureNone
	}

	blockData, exists := store.GetBlock(blockID)
	if !exists {
		return api.BlockStateUnknown, api.BlockFailureNone
	}
	switch blockData.State {
	case api.BlockStatePending:
		if blockID.Slot() <= r.latestCommittedSlotFunc() {
			return api.BlockStateRejected, blockData.FailureReason
		}
	case api.BlockStateAccepted, api.BlockStateConfirmed:
		if blockID.Slot() <= r.finalizedSlotFunc() {
			return api.BlockStateFinalized, api.BlockFailureNone
		}
	}

	return blockData.State, blockData.FailureReason
}

func (r *Retainer) transactionStatus(blockID iotago.BlockID) (iotago.TransactionID, api.TransactionState, api.TransactionFailureReason) {
	store, err := r.store(blockID.Slot())
	if err != nil {
		r.errorHandler(ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot()))
		return iotago.EmptyTransactionID, api.TransactionStateNoTransaction, api.TxFailureNone
	}

	txData, exists := store.GetTransaction(blockID)
	if !exists {
		return iotago.EmptyTransactionID, api.TransactionStateNoTransaction, api.TxFailureNone
	}

	// for confirmed and finalized we need to check for the block status
	if txData.State == api.TransactionStateAccepted {
		blockState, _ := r.blockStatus(blockID)

		switch blockState {
		case api.BlockStateConfirmed:
			return txData.TransactionID, api.TransactionStateConfirmed, api.TxFailureNone
		case api.BlockStateFinalized:
			return txData.TransactionID, api.TransactionStateFinalized, api.TxFailureNone
		}
	}

	return txData.TransactionID, txData.State, txData.FailureReason
}

func (r *Retainer) onBlockAttached(blockID iotago.BlockID) error {
	store, err := r.store(blockID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	return store.StoreBlockAttached(blockID)
}

func (r *Retainer) onBlockAccepted(blockID iotago.BlockID) error {
	store, err := r.store(blockID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	return store.StoreBlockAccepted(blockID)
}

func (r *Retainer) onBlockConfirmed(blockID iotago.BlockID) error {
	store, err := r.store(blockID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	return store.StoreBlockConfirmed(blockID)
}

func (r *Retainer) onTransactionAttached(blockID iotago.BlockID) error {
	store, err := r.store(blockID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	return store.StoreTransactionNoFailureStatus(blockID, api.TransactionStatePending)
}

func (r *Retainer) onTransactionAccepted(blockID iotago.BlockID) error {
	store, err := r.store(blockID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	return store.StoreTransactionNoFailureStatus(blockID, api.TransactionStateAccepted)
}

func (r *Retainer) onAttachmentUpdated(prevID iotago.BlockID, newID iotago.BlockID, accepted bool) error {
	store, err := r.store(prevID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", prevID.Slot())
	}

	if err := store.DeleteTransactionData(prevID); err != nil {
		return err
	}

	store, err = r.store(newID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", newID.Slot())
	}

	if accepted {
		return store.StoreTransactionNoFailureStatus(newID, api.TransactionStateAccepted)
	}

	return store.StoreTransactionNoFailureStatus(newID, api.TransactionStatePending)
}
