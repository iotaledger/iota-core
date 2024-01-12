package retainer

import (
	"sync"

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

	stakersResponses                *shrinkingmap.ShrinkingMap[uint32, []*api.ValidatorResponse]
	transactionLatestAttachmentSlot *shrinkingmap.ShrinkingMap[iotago.TransactionID, iotago.SlotIndex]
	attachmentSlotMutex             sync.Mutex

	workerPool *workerpool.WorkerPool

	module.Module
}

func New(workersGroup *workerpool.Group, retainerStoreFunc StoreFunc, latestCommittedSlotFunc LatestCommittedSlotFunc, finalizedSlotFunc FinalizedSlotFunc, errorHandler func(error)) *Retainer {
	return &Retainer{
		workerPool:                      workersGroup.CreatePool("Retainer", workerpool.WithWorkerCount(1)),
		store:                           retainerStoreFunc,
		stakersResponses:                shrinkingmap.New[uint32, []*api.ValidatorResponse](),
		transactionLatestAttachmentSlot: shrinkingmap.New[iotago.TransactionID, iotago.SlotIndex](),
		latestCommittedSlotFunc:         latestCommittedSlotFunc,
		finalizedSlotFunc:               finalizedSlotFunc,
		errorHandler:                    errorHandler,
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

			tx, exists := b.SignedTransaction()
			transactionID := iotago.EmptyTransactionID
			if exists {
				txID, err := tx.Transaction.ID()
				if err != nil {
					r.errorHandler(ierrors.Wrap(err, "failed to get txID from attached block"))
				}
				transactionID = txID
			}

			if err := r.onBlockAttached(b.ID(), transactionID); err != nil {
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

func (r *Retainer) RetainBlockFailure(blockID iotago.BlockID, failureCode api.BlockFailureReason, transactionID ...iotago.TransactionID) {
	store, err := r.store(blockID.Slot())
	if err != nil {
		r.errorHandler(ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot()))
		return
	}

	if err = store.StoreBlockFailure(blockID, failureCode); err != nil {
		r.errorHandler(ierrors.Wrap(err, "failed to store block failure in retainer"))
	}

	if len(transactionID) > 0 {
		// tx data is kept at the latest attachment slot
		// so that it will not be pruned before all its attachemnt retainer data is pruned
		err = r.moveTransactionMetadataToTheLatestAttachmentSlot(transactionID[0], blockID.Slot())
		if err != nil {
			r.errorHandler(ierrors.Wrap(err, "failed to move transaction metadata to the latest attachment slot"))
		}
	}
}

func (r *Retainer) RetainTransactionFailure(txID iotago.TransactionID, err error) {
	store, slot, storeErr := r.getLatestAttachmentStore(txID)
	if storeErr != nil {
		r.errorHandler(ierrors.Wrapf(storeErr, "could not get retainer store for slot %d", slot))
		return
	}

	if err := store.StoreTransactionFailure(txID, determineTxFailureReason(err)); err != nil {
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

// getLatestAttachmentStore returns the store where the transaction data is kept.
// The transaction is stored in the latest attachment slot.
func (r *Retainer) getLatestAttachmentStore(txID iotago.TransactionID) (*slotstore.Retainer, iotago.SlotIndex, error) {
	r.attachmentSlotMutex.Lock()
	defer r.attachmentSlotMutex.Unlock()

	storeSlot, exist := r.transactionLatestAttachmentSlot.Get(txID)
	if !exist {
		return nil, 0, ierrors.Errorf("transaction %s store location is not known", txID.String())
	}

	store, err := r.store(storeSlot)
	if err != nil {
		return nil, 0, ierrors.Wrapf(err, "could not get retainer store for slot %d", storeSlot)
	}

	return store, storeSlot, nil
}

func (r *Retainer) moveTransactionMetadataToTheLatestAttachmentSlot(txID iotago.TransactionID, latestAttachmentSlot iotago.SlotIndex) error {
	r.attachmentSlotMutex.Lock()
	defer r.attachmentSlotMutex.Unlock()

	currentSlot, exists := r.transactionLatestAttachmentSlot.Get(txID)
	if !exists {
		r.transactionLatestAttachmentSlot.Set(txID, latestAttachmentSlot)

		return nil
	}

	if currentSlot < latestAttachmentSlot {
		err, moved := r.moveTransactionMetadata(txID, currentSlot, latestAttachmentSlot)
		if moved {
			r.transactionLatestAttachmentSlot.Set(txID, latestAttachmentSlot)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *Retainer) moveTransactionMetadata(txID iotago.TransactionID, prevSlot, newSlot iotago.SlotIndex) (error, bool) {
	store, err := r.store(prevSlot)
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", prevSlot), false
	}

	txData, exist := store.GetTransaction(txID)
	if !exist {
		// nothing to move
		return nil, false
	}

	newStore, err := r.store(newSlot)
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", newSlot), false
	}

	err = newStore.StoreTransactionData(txID, txData)
	if err != nil {
		return err, false
	}

	err = store.DeleteTransactionData(txID)
	if err != nil {
		return err, true
	}

	return nil, true
}

func (r *Retainer) blockStatus(blockID iotago.BlockID) (api.BlockState, api.BlockFailureReason, iotago.TransactionID) {
	store, err := r.store(blockID.Slot())
	if err != nil {
		r.errorHandler(ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot()))
		return api.BlockStateUnknown, api.BlockFailureNone, iotago.EmptyTransactionID
	}

	blockData, exists := store.GetBlock(blockID)
	if !exists {
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
	store, err := r.store(blockID.Slot())
	if err != nil {
		r.errorHandler(ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot()))
		return iotago.EmptyTransactionID, api.TransactionStateNoTransaction, api.TxFailureNone
	}

	blockData, exists := store.GetBlock(blockID)
	if !exists {
		return iotago.EmptyTransactionID, api.TransactionStateNoTransaction, api.TxFailureNone
	}

	if blockData.TransactionID == iotago.EmptyTransactionID {
		return iotago.EmptyTransactionID, api.TransactionStateNoTransaction, api.TxFailureNone
	}

	txStore, _, err := r.getLatestAttachmentStore(blockData.TransactionID)
	if err != nil {
		r.errorHandler(ierrors.Wrapf(err, "HERE could not get retainer store for transaction %s", blockData.TransactionID.String()))
		return blockData.TransactionID, api.TransactionStateNoTransaction, api.TxFailureNone
	}

	txData, exists := txStore.GetTransaction(blockData.TransactionID)
	if !exists {
		r.errorHandler(ierrors.Errorf("HERE transaction %s not found in the latest attachment slot", blockData.TransactionID.String()))
		return blockData.TransactionID, api.TransactionStateNoTransaction, api.TxFailureNone
	}

	if txData.State == api.TransactionStateAccepted {
		blockState, _, _ := r.blockStatus(blockID)

		switch blockState {
		case api.BlockStateConfirmed:
			return blockData.TransactionID, api.TransactionStateConfirmed, api.TxFailureNone
		case api.BlockStateFinalized:
			return blockData.TransactionID, api.TransactionStateFinalized, api.TxFailureNone
		}
	}

	return blockData.TransactionID, txData.State, txData.FailureReason
}

func (r *Retainer) onBlockAttached(blockID iotago.BlockID, transactionID iotago.TransactionID) error {
	store, err := r.store(blockID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	// tx data is kept at the latest attachment slot
	// so that it will not be pruned before all its attachemnt retainer data is pruned
	err = r.moveTransactionMetadataToTheLatestAttachmentSlot(transactionID, blockID.Slot())

	return store.StoreBlockAttached(blockID, transactionID)
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

func (r *Retainer) onTransactionAttached(txID iotago.TransactionID) error {
	store, slot, err := r.getLatestAttachmentStore(txID)
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", slot)
	}

	return store.StoreTransactionNoFailureStatus(txID, api.TransactionStatePending)
}

func (r *Retainer) onTransactionAccepted(txID iotago.TransactionID) error {
	store, slot, err := r.getLatestAttachmentStore(txID)
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", slot)
	}

	return store.StoreTransactionNoFailureStatus(txID, api.TransactionStateAccepted)
}
