package blockretainer

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/retainer"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type (
	StoreFunc               func(iotago.SlotIndex) (*slotstore.BlockMetadataStore, error)
	LatestCommittedSlotFunc func() iotago.SlotIndex
	FinalizedSlotFunc       func() iotago.SlotIndex
)

type BlockRetainer struct {
	events *retainer.Events
	store  StoreFunc

	latestCommittedSlotFunc LatestCommittedSlotFunc
	finalizedSlotFunc       FinalizedSlotFunc
	errorHandler            func(error)

	workerPool *workerpool.WorkerPool
	module.Module
}

func New(workersGroup *workerpool.Group, retainerStoreFunc StoreFunc, latestCommittedSlotFunc LatestCommittedSlotFunc, finalizedSlotFunc FinalizedSlotFunc, errorHandler func(error)) *BlockRetainer {
	return &BlockRetainer{
		events:                  retainer.NewEvents(),
		workerPool:              workersGroup.CreatePool("Retainer", workerpool.WithWorkerCount(1)),
		store:                   retainerStoreFunc,
		latestCommittedSlotFunc: latestCommittedSlotFunc,
		finalizedSlotFunc:       finalizedSlotFunc,
		errorHandler:            errorHandler,
	}
}

// NewProvider creates a new BlockRetainer provider.
func NewProvider() module.Provider[*engine.Engine, retainer.BlockRetainer] {
	return module.Provide(func(e *engine.Engine) retainer.BlockRetainer {
		r := New(e.Workers.CreateGroup("Retainer"),
			e.Storage.BlockMetadata,
			func() iotago.SlotIndex {
				return e.SyncManager.LatestCommitment().Slot()
			},
			func() iotago.SlotIndex {
				return e.SyncManager.LatestFinalizedSlot()
			},
			e.ErrorHandler("retainer"))

		asyncOpt := event.WithWorkerPool(r.workerPool)

		e.Events.Booker.BlockBooked.Hook(func(b *blocks.Block) {
			if err := r.OnBlockBooked(b); err != nil {
				r.errorHandler(ierrors.Wrap(err, "failed to store on BlockBooked in retainer"))
			}
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
			if err := r.OnBlockDropped(b.ID()); err != nil {
				r.errorHandler(ierrors.Wrap(err, "failed to store on BlockDropped in retainer"))
			}
		})

		e.Events.Retainer.BlockRetained.LinkTo(r.events.BlockRetained)

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

func (r *BlockRetainer) getBlockMetadata(blockID iotago.BlockID) (*slotstore.BlockMetadata, error) {
	store, err := r.store(blockID.Slot())
	if err != nil {
		return nil, err
	}

	data, found := store.BlockMetadata(blockID)
	if !found {
		return nil, ierrors.Errorf("block %s not found", blockID.String())
	}

	return data, nil
}

func (r *BlockRetainer) BlockMetadata(blockID iotago.BlockID) (*api.BlockMetadataResponse, error) {
	blockStatus := r.blockState(blockID)
	if blockStatus == api.BlockStateUnknown {
		return nil, ierrors.Errorf("block %s not found", blockID.ToHex())
	}

	// we do not expose accepted flag
	if blockStatus == api.BlockStateAccepted {
		blockStatus = api.BlockStatePending
	}

	return &api.BlockMetadataResponse{
		BlockID:    blockID,
		BlockState: blockStatus,
	}, nil
}

func (r *BlockRetainer) blockState(blockID iotago.BlockID) api.BlockState {
	blockMetadata, err := r.getBlockMetadata(blockID)
	if err != nil {
		r.errorHandler(ierrors.Wrapf(err, "could not get block data for slot %d", blockID.Slot()))
		return api.BlockStateUnknown
	}

	switch blockMetadata.State {
	case api.BlockStatePending, api.BlockStateDropped:
		if blockID.Slot() <= r.latestCommittedSlotFunc() {
			return api.BlockStateOrphaned
		}
	case api.BlockStateAccepted, api.BlockStateConfirmed:
		if blockID.Slot() <= r.finalizedSlotFunc() {
			return api.BlockStateFinalized
		}
	}

	return blockMetadata.State
}

// OnBlockBooked triggers storing block in the retainer on block booked event.
func (r *BlockRetainer) OnBlockBooked(block *blocks.Block) error {
	if err := r.setBlockBooked(block.ID()); err != nil {
		return err
	}

	r.events.BlockRetained.Trigger(block)

	return nil
}

func (r *BlockRetainer) setBlockBooked(blockID iotago.BlockID) error {
	store, err := r.store(blockID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	return store.StoreBlockBooked(blockID)
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

	return store.StoreBlockConfirmed(blockID)
}

func (r *BlockRetainer) OnBlockDropped(blockID iotago.BlockID) error {
	store, err := r.store(blockID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	return store.StoreBlockDropped(blockID)
}
