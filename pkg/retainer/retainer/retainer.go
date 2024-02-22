package retainer

import (
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
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
	"github.com/iotaledger/iota.go/v4/api"
)

type (
	//nolint:revive
	SlotStoreFunc           func(iotago.SlotIndex) (*slotstore.Retainer, error)
	LatestCommittedSlotFunc func() iotago.SlotIndex
	FinalizedSlotFunc       func() iotago.SlotIndex
)

const maxStakersResponsesCacheNum = 10

// Retainer keeps and resolves all the information needed in the API and INX.
type Retainer struct {
	events                  *retainer.Events
	store                   *metadataStore
	latestCommittedSlotFunc LatestCommittedSlotFunc
	finalizedSlotFunc       FinalizedSlotFunc
	errorHandler            func(error)

	stakersResponses *shrinkingmap.ShrinkingMap[uint32, []*api.ValidatorResponse]

	workerPool *workerpool.WorkerPool

	module.Module
}

func New(workersGroup *workerpool.Group, slotStoreFunc SlotStoreFunc, latestCommittedSlotFunc LatestCommittedSlotFunc, finalizedSlotFunc FinalizedSlotFunc, errorHandler func(error)) *Retainer {
	return &Retainer{
		workerPool:              workersGroup.CreatePool("Retainer", workerpool.WithWorkerCount(1)),
		events:                  retainer.NewEvents(),
		store:                   newMetadataStore(slotStoreFunc),
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
			func() iotago.SlotIndex {
				// use settings in case SyncManager is not constructed yet.
				if e.SyncManager == nil {
					return e.Storage.Settings().LatestCommitment().Slot()
				}

				return e.SyncManager.LatestCommitment().Slot()
			},
			func() iotago.SlotIndex {
				// use settings in case SyncManager is not constructed yet.
				if e.SyncManager == nil {
					return e.Storage.Settings().LatestFinalizedSlot()
				}

				return e.SyncManager.LatestFinalizedSlot()
			},
			e.ErrorHandler("retainer"))

		asyncOpt := event.WithWorkerPool(r.workerPool)

		e.Events.Booker.BlockBooked.Hook(func(b *blocks.Block) {
			if err := r.onBlockBooked(b); err != nil {
				r.errorHandler(ierrors.Wrap(err, "failed to store on BlockBooked in retainer"))
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

		e.Events.Scheduler.BlockDropped.Hook(func(b *blocks.Block, _ error) {
			if err := r.onBlockDropped(b.ID()); err != nil {
				r.errorHandler(ierrors.Wrap(err, "failed to store on BlockConfirmed in retainer"))
			}
		})

		e.Initialized.OnTrigger(func() {
			e.Ledger.MemPool().OnSignedTransactionAttached(func(signedTransactionMetadata mempool.SignedTransactionMetadata) {
				txID := signedTransactionMetadata.TransactionMetadata().ID()
				signedTransactionMetadata.OnSignaturesInvalid(func(err error) {
					r.RetainTransactionFailure(txID, err)
				})

				signedTransactionMetadata.OnSignaturesValid(func() {
					transactionMetadata := signedTransactionMetadata.TransactionMetadata()

					//if err := r.onTransactionAttached(txID); err != nil {
					//	r.errorHandler(ierrors.Wrap(err, "failed to store on TransactionAttached in retainer"))
					//}

					transactionMetadata.OnInvalid(func(err error) {
						r.RetainTransactionFailure(txID, err)
					})

					transactionMetadata.OnRejected(func() {
						r.RetainTransactionFailure(txID, iotago.ErrTxConflictRejected)
					})

					transactionMetadata.OnAccepted(func() {
						//if err := r.onTransactionAccepted(txID); err != nil {
						//	r.errorHandler(ierrors.Wrap(err, "failed to store on TransactionAccepted in retainer"))
						//}
					})
				})
			})
		})

		e.Events.Retainer.LinkTo(r.events)

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

func (r *Retainer) BlockMetadata(blockID iotago.BlockID) (*api.BlockMetadataResponse, error) {
	blockState := r.blockStatus(blockID)
	if blockState == api.BlockStateUnknown {
		return nil, ierrors.Errorf("block %s not found", blockID.ToHex())
	}

	// we do not expose accepted flag
	if blockState == api.BlockStateAccepted {
		blockState = api.BlockStatePending
	}

	return &api.BlockMetadataResponse{
		BlockID:    blockID,
		BlockState: blockState,
	}, nil
}

// RetainTransactionFailure stores the transaction failure in the retainer.
func (r *Retainer) RetainTransactionFailure(_ iotago.TransactionID, _ error) {
	// TODO: remove this method after merging the TX retainer PR
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

func (r *Retainer) blockStatus(blockID iotago.BlockID) api.BlockState {
	blockData, err := r.store.getBlockData(blockID)
	if err != nil {
		r.errorHandler(ierrors.Wrapf(err, "could not get block data for slot %d", blockID.Slot()))
		return api.BlockStateUnknown
	}

	switch blockData.State {
	case api.BlockStatePending:
		if blockID.Slot() <= r.latestCommittedSlotFunc() {
			return api.BlockStateOrphaned
		}
	case api.BlockStateAccepted, api.BlockStateConfirmed:
		if blockID.Slot() <= r.finalizedSlotFunc() {
			return api.BlockStateFinalized
		}
	}

	return blockData.State
}

func (r *Retainer) onBlockBooked(block *blocks.Block) error {
	if err := r.store.setBlockBooked(block.ID()); err != nil {
		return err
	}

	return nil
}

func (r *Retainer) onBlockAccepted(blockID iotago.BlockID) error {
	return r.store.setBlockAccepted(blockID)
}

func (r *Retainer) onBlockConfirmed(blockID iotago.BlockID) error {
	return r.store.setBlockConfirmed(blockID)
}

func (r *Retainer) onBlockDropped(blockID iotago.BlockID) error {
	return r.store.setBlockDropped(blockID)
}
