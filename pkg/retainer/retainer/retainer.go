package retainer

import (
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/commitmentfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/retainer"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

type (
	//nolint:revive
	RetainerFunc            func(iotago.SlotIndex) (*slotstore.Retainer, error)
	LatestCommittedSlotFunc func() iotago.SlotIndex
	FinalizedSlotFunc       func() iotago.SlotIndex
)

const MaxStakersResponsesCacheNum = 10

// Retainer keeps and resolves all the information needed in the API and INX.
type Retainer struct {
	store                   RetainerFunc
	latestCommittedSlotFunc LatestCommittedSlotFunc
	finalizedSlotFunc       FinalizedSlotFunc
	errorHandler            func(error)

	stakersResponses *shrinkingmap.ShrinkingMap[uint32, []*apimodels.ValidatorResponse]

	workerPool *workerpool.WorkerPool

	module.Module
}

func New(workersGroup *workerpool.Group, retainerFunc RetainerFunc, latestCommittedSlotFunc LatestCommittedSlotFunc, finalizedSlotFunc FinalizedSlotFunc, errorHandler func(error)) *Retainer {
	return &Retainer{
		workerPool:              workersGroup.CreatePool("Retainer", workerpool.WithWorkerCount(1)),
		store:                   retainerFunc,
		stakersResponses:        shrinkingmap.New[uint32, []*apimodels.ValidatorResponse](),
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

		e.Events.CommitmentFilter.BlockFiltered.Hook(func(e *commitmentfilter.BlockFilteredEvent) {
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
			r.RetainBlockFailure(b.ID(), apimodels.BlockFailureDroppedDueToCongestion)
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

					transactionMetadata.OnConflicting(func() {
						// transaction is not included yet, thus EarliestIncludedAttachment is not set.
						r.RetainTransactionFailure(attachment, iotago.ErrTxConflicting)
					})

					transactionMetadata.OnInvalid(func(err error) {
						// transaction is not included yet, thus EarliestIncludedAttachment is not set.
						r.RetainTransactionFailure(attachment, err)
					})

					transactionMetadata.OnAccepted(func() {
						attachmentID := transactionMetadata.EarliestIncludedAttachment()
						if slot := attachmentID.Slot(); slot > 0 {
							if err := r.onTransactionAccepted(attachmentID); err != nil {
								r.errorHandler(ierrors.Wrap(err, "failed to store on TransactionAccepted in retainer"))
							}
						}
					})

					transactionMetadata.OnEarliestIncludedAttachmentUpdated(func(prevBlock, newBlock iotago.BlockID) {
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

func (r *Retainer) RegisteredValidatorsCache(index uint32) ([]*apimodels.ValidatorResponse, bool) {
	return r.stakersResponses.Get(index)
}

func (r *Retainer) RetainRegisteredValidatorsCache(index uint32, resp []*apimodels.ValidatorResponse) {
	r.stakersResponses.Set(index, resp)
	if r.stakersResponses.Size() > MaxStakersResponsesCacheNum {
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

func (r *Retainer) blockStatus(blockID iotago.BlockID) (apimodels.BlockState, apimodels.BlockFailureReason) {
	store, err := r.store(blockID.Slot())
	if err != nil {
		r.errorHandler(ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot()))
		return apimodels.BlockStateUnknown, apimodels.BlockFailureNone
	}

	blockData, exists := store.GetBlock(blockID)
	if !exists {
		return apimodels.BlockStateUnknown, apimodels.BlockFailureNone
	}
	switch blockData.State {
	case apimodels.BlockStatePending:
		if blockID.Slot() <= r.latestCommittedSlotFunc() {
			return apimodels.BlockStateRejected, blockData.FailureReason
		}
	case apimodels.BlockStateAccepted, apimodels.BlockStateConfirmed:
		if blockID.Slot() <= r.finalizedSlotFunc() {
			return apimodels.BlockStateFinalized, apimodels.BlockFailureNone
		}
	}

	return blockData.State, blockData.FailureReason
}

func (r *Retainer) transactionStatus(blockID iotago.BlockID) (apimodels.TransactionState, apimodels.TransactionFailureReason) {
	store, err := r.store(blockID.Slot())
	if err != nil {
		r.errorHandler(ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot()))
		return apimodels.TransactionStateNoTransaction, apimodels.TxFailureNone
	}

	txData, exists := store.GetTransaction(blockID)
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

	return store.StoreTransactionNoFailureStatus(blockID, apimodels.TransactionStatePending)
}

func (r *Retainer) onTransactionAccepted(blockID iotago.BlockID) error {
	store, err := r.store(blockID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	return store.StoreTransactionNoFailureStatus(blockID, apimodels.TransactionStateAccepted)
}

func (r *Retainer) onAttachmentUpdated(prevID, newID iotago.BlockID, accepted bool) error {
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
		return store.StoreTransactionNoFailureStatus(newID, apimodels.TransactionStateAccepted)
	}

	return store.StoreTransactionNoFailureStatus(newID, apimodels.TransactionStatePending)
}
