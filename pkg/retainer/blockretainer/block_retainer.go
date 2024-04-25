package blockretainer

import (
	"sync"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/retainer"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type (
	StoreFunc         func(iotago.SlotIndex) (*slotstore.BlockMetadataStore, error)
	FinalizedSlotFunc func() iotago.SlotIndex
)

type BlockRetainer struct {
	events *retainer.BlockRetainerEvents
	store  StoreFunc
	cache  *cache

	latestCommittedSlot iotago.SlotIndex
	finalizedSlotFunc   FinalizedSlotFunc
	errorHandler        func(error)

	workerPool *workerpool.WorkerPool
	sync.RWMutex
	module.Module
}

func New(subModule module.Module, workersGroup *workerpool.Group, retainerStoreFunc StoreFunc, finalizedSlotFunc FinalizedSlotFunc, errorHandler func(error)) *BlockRetainer {
	return options.Apply(&BlockRetainer{
		Module:            subModule,
		events:            retainer.NewBlockRetainerEvents(),
		workerPool:        workersGroup.CreatePool("BlockRetainer", workerpool.WithWorkerCount(1)),
		store:             retainerStoreFunc,
		cache:             newCache(),
		finalizedSlotFunc: finalizedSlotFunc,
		errorHandler:      errorHandler,
	}, nil, func(r *BlockRetainer) {
		r.ShutdownEvent().OnTrigger(r.shutdown)

		r.ConstructedEvent().Trigger()
	})
}

// NewProvider creates a new BlockRetainer provider.
func NewProvider() module.Provider[*engine.Engine, retainer.BlockRetainer] {
	return module.Provide(func(e *engine.Engine) retainer.BlockRetainer {
		r := New(e.NewSubModule("BlockRetainer"),
			e.Workers.CreateGroup("BlockRetainer"),
			e.Storage.BlockMetadata,
			func() iotago.SlotIndex {
				return e.SyncManager.LatestFinalizedSlot()
			},
			e.ErrorHandler("blockRetainer"))

		asyncOpt := event.WithWorkerPool(r.workerPool)

		e.ConstructedEvent().OnTrigger(func() {
			e.Events.Booker.BlockBooked.Hook(func(block *blocks.Block) {
				if err := r.OnBlockBooked(block); err != nil {
					r.errorHandler(ierrors.Wrap(err, "failed to store on BlockBooked in retainer"))
				}
			}, asyncOpt)

			e.Events.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
				if err := r.OnBlockAccepted(block); err != nil {
					r.errorHandler(ierrors.Wrap(err, "failed to store on BlockAccepted in retainer"))
				}
			}, asyncOpt)

			e.Events.BlockGadget.BlockConfirmed.Hook(func(block *blocks.Block) {
				if err := r.OnBlockConfirmed(block); err != nil {
					r.errorHandler(ierrors.Wrap(err, "failed to store on BlockConfirmed in retainer"))
				}
			}, asyncOpt)

			e.Events.Scheduler.BlockDropped.Hook(func(block *blocks.Block, _ error) {
				if err := r.OnBlockDropped(block); err != nil {
					r.errorHandler(ierrors.Wrap(err, "failed to store on BlockDropped in retainer"))
				}
			})

			// this event is fired when a new commitment is detected
			e.Events.Notarization.LatestCommitmentUpdated.Hook(func(commitment *model.Commitment) {
				if err := r.CommitSlot(commitment.Slot()); err != nil {
					panic(err)
				}
			}, asyncOpt)

			e.Events.BlockRetainer.LinkTo(r.events)

			r.InitializedEvent().Trigger()
		})

		return r
	})
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (r *BlockRetainer) Reset() {
	r.Lock()
	defer r.Unlock()

	r.cache.uncommittedBlockMetadata.Clear()
}

// Shutdown shuts down the BlockRetainer.
func (r *BlockRetainer) shutdown() {
	r.workerPool.Shutdown()

	r.StoppedEvent().Trigger()
}

func (r *BlockRetainer) BlockMetadata(blockID iotago.BlockID) (*api.BlockMetadataResponse, error) {
	r.RLock()
	defer r.RUnlock()

	blockStatus, err := r.blockState(blockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "block %s not found", blockID)
	}

	return &api.BlockMetadataResponse{
		BlockID:    blockID,
		BlockState: blockStatus,
	}, nil
}

func (r *BlockRetainer) blockState(blockID iotago.BlockID) (api.BlockState, error) {
	state, found := r.cache.blockMetadataByID(blockID)
	if !found {
		// block is not committed yet, should be in cache
		if blockID.Slot() > r.latestCommittedSlot {
			return api.BlockStateUnknown, kvstore.ErrKeyNotFound
		}

		blockMetadata, err := r.getBlockMetadata(blockID)
		if err != nil {
			return api.BlockStateUnknown, err
		}

		state = blockMetadata.State
	}

	switch state {
	case api.BlockStatePending, api.BlockStateDropped:
		if blockID.Slot() <= r.latestCommittedSlot {
			return api.BlockStateOrphaned, nil
		}
	case api.BlockStateAccepted, api.BlockStateConfirmed:
		if blockID.Slot() <= r.finalizedSlotFunc() {
			return api.BlockStateFinalized, nil
		}
	}

	return state, nil
}

func (r *BlockRetainer) getBlockMetadata(blockID iotago.BlockID) (*slotstore.BlockMetadata, error) {
	store, err := r.store(blockID.Slot())
	if err != nil {
		return nil, ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	data, err := store.BlockMetadata(blockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "block %s not found", blockID.String())
	}

	return data, nil
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
	return r.UpdateBlockMetadata(blockID, api.BlockStatePending)
}

func (r *BlockRetainer) OnBlockAccepted(block *blocks.Block) error {
	if err := r.UpdateBlockMetadata(block.ID(), api.BlockStateAccepted); err != nil {
		return err
	}

	r.events.BlockAccepted.Trigger(block)

	return nil
}

func (r *BlockRetainer) OnBlockConfirmed(block *blocks.Block) error {
	if err := r.UpdateBlockMetadata(block.ID(), api.BlockStateConfirmed); err != nil {
		return err
	}

	r.events.BlockConfirmed.Trigger(block)

	return nil
}

func (r *BlockRetainer) OnBlockDropped(block *blocks.Block) error {
	if err := r.UpdateBlockMetadata(block.ID(), api.BlockStateDropped); err != nil {
		return err
	}

	r.events.BlockDropped.Trigger(block)

	return nil
}

func (r *BlockRetainer) UpdateBlockMetadata(blockID iotago.BlockID, state api.BlockState) error {
	r.Lock()
	defer r.Unlock()

	// we can safely use this as a check where block is stored as r.latestCommittedSlot is updated on commitment
	if blockID.Slot() > r.latestCommittedSlot {
		r.cache.setBlockMetadata(blockID, state)

		return nil
	}

	//  for blocks the state might still change after the commitment but only on confirmation
	if state != api.BlockStateConfirmed {
		return ierrors.Errorf("cannot update block metadata for block %s with state %s as block is already committed", blockID.String(), state)
	}

	// store in the database
	store, err := r.store(blockID.Slot())
	if err != nil {
		return ierrors.Wrapf(err, "could not get retainer store for slot %d", blockID.Slot())
	}

	return store.StoreBlockMetadata(blockID, state)
}

func (r *BlockRetainer) CommitSlot(committedSlot iotago.SlotIndex) error {
	r.Lock()
	defer r.Unlock()

	var innerErr error
	r.cache.uncommittedBlockMetadata.ForEach(func(cacheSlot iotago.SlotIndex, blocks map[iotago.BlockID]api.BlockState) bool {
		if cacheSlot <= committedSlot {
			store, err := r.store(cacheSlot)
			if err != nil {
				innerErr = ierrors.Wrapf(err, "could not get retainer store for slot %d", cacheSlot)
				return false
			}

			for blockID, state := range blocks {
				if err = store.StoreBlockMetadata(blockID, state); err != nil {
					innerErr = ierrors.Wrapf(err, "could not store block metadata for block %s", blockID.String())
					return false
				}
			}

			r.cache.uncommittedBlockMetadata.Delete(cacheSlot)
		}

		return true
	})
	if innerErr != nil {
		return innerErr
	}

	r.latestCommittedSlot = committedSlot

	return nil
}
