package blockretainer

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/postsolidfilter"
	"github.com/iotaledger/iota-core/pkg/retainer"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type (
	StoreFunc               func(iotago.SlotIndex) (*slotstore.Retainer, error)
	LatestCommittedSlotFunc func() iotago.SlotIndex
	FinalizedSlotFunc       func() iotago.SlotIndex
)

type BlockRetainer struct {
	store StoreFunc

	latestCommittedSlotFunc LatestCommittedSlotFunc
	finalizedSlotFunc       FinalizedSlotFunc
	errorHandler            func(error)

	workerPool *workerpool.WorkerPool
	module.Module
}

func New(workersGroup *workerpool.Group, retainerStoreFunc StoreFunc, latestCommittedSlotFunc LatestCommittedSlotFunc, finalizedSlotFunc FinalizedSlotFunc, errorHandler func(error)) *BlockRetainer {
	return &BlockRetainer{
		workerPool:              workersGroup.CreatePool("Retainer", workerpool.WithWorkerCount(1)),
		store:                   retainerStoreFunc,
		latestCommittedSlotFunc: latestCommittedSlotFunc,
		finalizedSlotFunc:       finalizedSlotFunc,
		errorHandler:            errorHandler,
	}
}

// NewProvider creates a new Retainer provider.
func NewProvider() module.Provider[*engine.Engine, retainer.BlockRetainer] {
	return module.Provide(func(e *engine.Engine) retainer.BlockRetainer {
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

		e.Events.BlockDAG.BlockAppended.Hook(func(b *blocks.Block) {
			if err := r.OnBlockAppended(b.ModelBlock()); err != nil {
				r.errorHandler(ierrors.Wrap(err, "failed to store on BlockAppended in retainer"))
			}
		}, asyncOpt)

		e.Events.PostSolidFilter.BlockFiltered.Hook(func(e *postsolidfilter.BlockFilteredEvent) {
			r.OnBlockFilter(e.Block, e.Reason)
		}, asyncOpt)

		e.Events.BlockGadget.BlockAccepted.Hook(func(b *blocks.Block) {
			if err := r.OnBlockAccepted(b.ID()); err != nil {
				r.errorHandler(ierrors.Wrap(err, "failed to store on BlockAccepted in retainer"))
			}
		}, asyncOpt)

		e.Events.BlockGadget.BlockConfirmed.Hook(func(b *blocks.Block) {
			if err := r.OnBlockConfirmed(b.ID()); err != nil {
				r.errorHandler(ierrors.Wrap(err, "failed to store on BlockConfirmed in retainer"))
			}
		}, asyncOpt)

		e.Events.Scheduler.BlockDropped.Hook(func(b *blocks.Block, _ error) {
			r.RetainBlockFailure(b.ModelBlock(), api.BlockFailureDroppedDueToCongestion)
		})

		r.TriggerInitialized()

		return r
	})
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (r *BlockRetainer) Reset() {
	// TODO: check if something needs to be cleaned here
	// on chain switching reset everything up to the forking point
}

func (r *BlockRetainer) Shutdown() {
	r.workerPool.Shutdown()
}

func (r *BlockRetainer) getBlockData(blockID iotago.BlockID) (*slotstore.BlockRetainerData, error) {
	store, err := r.store(blockID.Slot())
	if err != nil {
		return nil, err
	}

	data, found := store.GetBlock(blockID)
	if !found {
		return nil, ierrors.Errorf("block %s not found", blockID.String())
	}

	return data, nil
}

func (r *BlockRetainer) storeBlockData(modelBlock *model.Block, failureCode api.BlockFailureReason) error {
	store, err := r.store(modelBlock.ID().Slot())
	if err != nil {
		return err
	}

	if failureCode == api.BlockFailureNone {
		return store.StoreBlockBooked(modelBlock.ID())
	}

	return store.StoreBlockFailure(modelBlock.ID(), failureCode)
}

func (r *BlockRetainer) BlockMetadata(blockID iotago.BlockID) (*api.BlockMetadataResponse, error) {
	blockStatus, blockFailureReason := r.blockStatus(blockID)
	if blockStatus == api.BlockStateUnknown {
		return nil, ierrors.Errorf("block %s not found", blockID.ToHex())
	}

	// we do not expose accepted flag
	if blockStatus == api.BlockStateAccepted {
		blockStatus = api.BlockStatePending
	}

	return &api.BlockMetadataResponse{
		BlockID:            blockID,
		BlockState:         blockStatus,
		BlockFailureReason: blockFailureReason,
	}, nil
}

func (r *BlockRetainer) blockStatus(blockID iotago.BlockID) (api.BlockState, api.BlockFailureReason) {
	blockData, err := r.getBlockData(blockID)
	if err != nil {
		r.errorHandler(ierrors.Wrapf(err, "could not get block data for slot %d", blockID.Slot()))
		return api.BlockStateUnknown, api.BlockFailureNone
	}

	switch blockData.State {
	case api.BlockStatePending:
		if blockID.Slot() <= r.latestCommittedSlotFunc() {
			return api.BlockStateOrphaned, blockData.FailureReason
		}
	case api.BlockStateAccepted, api.BlockStateConfirmed:
		if blockID.Slot() <= r.finalizedSlotFunc() {
			return api.BlockStateFinalized, api.BlockFailureNone
		}
	}

	return blockData.State, blockData.FailureReason
}

// RetainBlockFailure stores the block failure in the retainer and determines if the model block had a transaction attached.
func (r *BlockRetainer) RetainBlockFailure(modelBlock *model.Block, failureCode api.BlockFailureReason) {
	if err := r.storeBlockData(modelBlock, failureCode); err != nil {
		r.errorHandler(ierrors.Wrap(err, "failed to store block failure in retainer"))
	}
}

func (r *BlockRetainer) OnBlockAppended(modelBlock *model.Block) error {
	if err := r.storeBlockData(modelBlock, api.BlockFailureNone); err != nil {
		return ierrors.Wrap(err, "failed to store block failure in retainer")
	}

	return nil
}

func (r *BlockRetainer) OnBlockFilter(block *blocks.Block, reason error) {
	r.RetainBlockFailure(block.ModelBlock(), api.DetermineBlockFailureReason(reason))
}

func (r *BlockRetainer) OnBlockAccepted(blockID iotago.BlockID) error {
	store, err := r.store(blockID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	return store.StoreBlockAccepted(blockID)
}

func (r *BlockRetainer) OnBlockConfirmed(blockID iotago.BlockID) error {
	store, err := r.store(blockID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	if err := store.StoreBlockConfirmed(blockID); err != nil {
		return ierrors.Wrapf(err, "")
	}

	return nil
}
