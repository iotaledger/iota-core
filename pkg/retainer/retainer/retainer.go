package retainer

import (
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
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
	store                   *metadataStore
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
		store:                   newMetadataStore(retainerStoreFunc),
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
			if err := r.onBlockAttached(b); err != nil {
				r.errorHandler(ierrors.Wrap(err, "failed to store on BlockAttached in retainer"))
			}
		}, asyncOpt)

		e.Events.PostSolidFilter.BlockFiltered.Hook(func(e *postsolidfilter.BlockFilteredEvent) {
			r.onBlockFilter(e.Block, e.Reason)
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
			r.RetainBlockFailure(b, api.BlockFailureDroppedDueToCongestion)
		})

		e.Initialized.OnTrigger(func() {
			e.Ledger.MemPool().OnSignedTransactionAttached(func(signedTransactionMetadata mempool.SignedTransactionMetadata) {
				//attachment := signedTransactionMetadata.Attachments()[0]
				txID := signedTransactionMetadata.TransactionMetadata().ID()
				signedTransactionMetadata.OnSignaturesInvalid(func(err error) {
					r.RetainTransactionFailure(txID, err)
				})

				signedTransactionMetadata.OnSignaturesValid(func() {
					transactionMetadata := signedTransactionMetadata.TransactionMetadata()

					// transaction is not included yet, thus EarliestIncludedAttachment is not set.
					if err := r.onTransactionAttached(txID); err != nil {
						r.errorHandler(ierrors.Wrap(err, "failed to store on TransactionAttached in retainer"))
					}

					transactionMetadata.OnInvalid(func(err error) {
						// transaction is not included yet, thus EarliestIncludedAttachment is not set.
						r.RetainTransactionFailure(txID, err)
					})

					transactionMetadata.OnRejected(func() {
						r.RetainTransactionFailure(txID, iotago.ErrTxConflicting)
					})

					transactionMetadata.OnAccepted(func() {
						if err := r.onTransactionAccepted(txID); err != nil {
							r.errorHandler(ierrors.Wrap(err, "failed to store on TransactionAccepted in retainer"))
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
	blockStatus, blockFailureReason, _ := r.blockStatus(blockID)
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

// RetainBlockFailure stores the block failure in the retainer and determines if the block had a transaction attached.
func (r *Retainer) RetainBlockFailure(block *blocks.Block, failureCode api.BlockFailureReason) {
	if err := r.store.storeBlockData(block.ID(), failureCode, r.transactionID(block)); err != nil {
		r.errorHandler(ierrors.Wrap(err, "failed to store block failure in retainer"))
	}
}

// RetainModelBlockFailure stores the block failure in the retainer and determines if the model block had a transaction attached.
func (r *Retainer) RetainModelBlockFailure(block *model.Block, failureCode api.BlockFailureReason) {
	txID := iotago.EmptyTransactionID
	var err error
	tx, hasTx := block.SignedTransaction()
	if hasTx {
		txID, err = tx.Transaction.ID()
		if err != nil {
			r.errorHandler(ierrors.Wrap(err, "failed to get txID from attached block on RetainModelBlockFailure"))
		}
	}
	if err = r.store.storeBlockData(block.ID(), failureCode, txID); err != nil {
		r.errorHandler(ierrors.Wrap(err, "failed to store block failure in retainer"))
	}
}

// RetainTransactionFailure stores the transaction failure in the retainer.
func (r *Retainer) RetainTransactionFailure(txID iotago.TransactionID, reason error) {
	err := r.store.setTransactionFailure(txID, determineTxFailureReason(reason))
	if err != nil {
		r.errorHandler(ierrors.Wrap(err, "failed to store transaction failure in retainer"))
	}
}

// RegisteredValidatorsCache returns the cached registered validators for the given index.
func (r *Retainer) RegisteredValidatorsCache(index uint32) ([]*api.ValidatorResponse, bool) {
	return r.stakersResponses.Get(index)
}

// RetainRegisteredValidatorsCache stores the registered validators responses for the given index.
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

func (r *Retainer) transactionID(block *blocks.Block) iotago.TransactionID {
	tx, exists := block.SignedTransaction()
	if !exists {
		return iotago.EmptyTransactionID
	}

	txID, err := tx.Transaction.ID()
	if err != nil {
		r.errorHandler(ierrors.Wrap(err, "failed to get txID from attached block"))
	}

	return txID
}

func (r *Retainer) blockStatus(blockID iotago.BlockID) (api.BlockState, api.BlockFailureReason, iotago.TransactionID) {
	blockData, err := r.store.getBlockData(blockID)
	if err != nil {
		r.errorHandler(ierrors.Wrapf(err, "could not get block data for slot %d", blockID.Slot()))
		return api.BlockStateUnknown, api.BlockFailureNone, iotago.EmptyTransactionID
	}

	switch blockData.State {
	case api.BlockStatePending:
		if blockID.Slot() <= r.latestCommittedSlotFunc() {
			return api.BlockStateRejected, blockData.FailureReason, blockData.TransactionID
		}
	case api.BlockStateAccepted, api.BlockStateConfirmed:
		if blockID.Slot() <= r.finalizedSlotFunc() {
			return api.BlockStateFinalized, api.BlockFailureNone, blockData.TransactionID
		}
	}

	return blockData.State, blockData.FailureReason, blockData.TransactionID
}

func (r *Retainer) transactionStatus(blockID iotago.BlockID) (iotago.TransactionID, api.TransactionState, api.TransactionFailureReason) {
	blockData, err := r.store.getBlockData(blockID)
	if err != nil {
		r.errorHandler(ierrors.Wrapf(err, "could not get block data for slot %d", blockID.Slot()))
		return iotago.EmptyTransactionID, api.TransactionStateNoTransaction, api.TxFailureNone
	}
	if blockData.TransactionID == iotago.EmptyTransactionID {
		r.errorHandler(ierrors.Errorf("transaction %s not found for block %s", blockData.TransactionID.String(), blockID.String()))
		return iotago.EmptyTransactionID, api.TransactionStateNoTransaction, api.TxFailureNone
	}

	txData, err := r.store.getTransactionData(blockData.TransactionID)
	if err != nil {
		r.errorHandler(ierrors.Wrapf(err, "could not get transaction data for transaction %s", blockData.TransactionID.String()))
		return blockData.TransactionID, api.TransactionStateNoTransaction, api.TxFailureNone
	}

	// check if any attachment is already confirmed
	if txData.State == api.TransactionStateConfirmed {
		// is attachmetn already finalized?
		if txData.ConfirmedAttachmentSlot <= r.finalizedSlotFunc() {
			return blockData.TransactionID, api.TransactionStateFinalized, api.TxFailureNone
		}

		return blockData.TransactionID, api.TransactionStateConfirmed, api.TxFailureNone
	}

	return blockData.TransactionID, txData.State, txData.FailureReason
}

func (r *Retainer) onBlockAttached(block *blocks.Block) error {
	return r.store.storeBlockData(block.ID(), api.BlockFailureNone, r.transactionID(block))
}

func (r *Retainer) onBlockFilter(block *blocks.Block, reason error) {
	r.RetainBlockFailure(block, determineBlockFailureReason(reason))
}

func (r *Retainer) onBlockAccepted(blockID iotago.BlockID) error {
	return r.store.setBlockAccepted(blockID)
}

func (r *Retainer) onBlockConfirmed(blockID iotago.BlockID) error {
	return r.store.setBlockConfirmed(blockID)
}

func (r *Retainer) onTransactionAttached(txID iotago.TransactionID) error {
	return r.store.setTransactionNoFailure(txID, api.TransactionStatePending)
}

func (r *Retainer) onTransactionAccepted(txID iotago.TransactionID) error {
	return r.store.setTransactionNoFailure(txID, api.TransactionStateAccepted)
}
