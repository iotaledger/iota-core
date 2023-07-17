package retainer

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/retainer"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/models"
)

type (
	BlockRetrieveFunc func(iotago.BlockID) (*model.Block, bool)
	BlockDiskFunc     func(iotago.SlotIndex) *prunable.Blocks
	FinalizedSlotFunc func() iotago.SlotIndex
	RetainerFunc      func(iotago.SlotIndex) *prunable.Retainer
)

// Retainer keeps and resolves all the information needed in the API and INX.
type Retainer struct {
	blockRetrieveFunc BlockRetrieveFunc
	blockDiskFunc     BlockDiskFunc
	finalizedSlotFunc FinalizedSlotFunc
	retainerFunc      RetainerFunc
	errorHandler      func(error)

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

func New(retainerFunc RetainerFunc, blockRetrieveFunc BlockRetrieveFunc, blockDiskFunc BlockDiskFunc, finalizedSlotIndex FinalizedSlotFunc, errorHandler func(error)) *Retainer {
	return &Retainer{
		blockRetrieveFunc: blockRetrieveFunc,
		blockDiskFunc:     blockDiskFunc,
		finalizedSlotFunc: finalizedSlotIndex,
		retainerFunc:      retainerFunc,
		errorHandler:      errorHandler,
	}
}

// NewProvider creates a new SyncManager provider.
func NewProvider() module.Provider[*engine.Engine, retainer.Retainer] {
	return module.Provide(func(e *engine.Engine) retainer.Retainer {
		r := New(e.Storage.Retainer, e.Block, e.Storage.Blocks, e.Storage.Settings().LatestFinalizedSlot, e.ErrorHandler("retainer"))
		asyncOpt := event.WithWorkerPool(e.Workers.CreatePool("Retainer", 1))

		e.Events.BlockDAG.BlockAttached.Hook(func(b *blocks.Block) {
			err := r.onBlockAttached(b.ID())
			if err != nil {
				r.errorHandler(ierrors.Wrap(err, "failed to store on Block Attached in retainer"))
			}
		}, asyncOpt)

		e.Events.BlockGadget.BlockAccepted.Hook(func(b *blocks.Block) {
			err := r.onBlockAccepted(b.ID())
			if err != nil {
				r.errorHandler(ierrors.Wrap(err, "failed to store on Block Accepted in retainer"))
			}
		}, asyncOpt)

		e.Events.BlockGadget.BlockConfirmed.Hook(func(b *blocks.Block) {
			err := r.onBlockConfirmed(b.ID())
			if err != nil {
				r.errorHandler(ierrors.Wrap(err, "failed to store on Block Confirmed in retainer"))
			}
		}, asyncOpt)

		r.TriggerInitialized()

		return r
	})
}

func (r *Retainer) Shutdown() {
	r.retainerFunc = nil
}

func (r *Retainer) Block(blockID iotago.BlockID) (*model.Block, error) {
	block, _ := r.blockRetrieveFunc(blockID)
	if block == nil {
		return nil, ierrors.Errorf("block not found: %s", blockID.ToHex())
	}

	return block, nil
}

func (r *Retainer) BlockMetadata(blockID iotago.BlockID) (*retainer.BlockMetadata, error) {
	status, err := r.blockStatus(blockID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get block status for %s", blockID.ToHex())
	}

	// TODO: fill in blockReason, TxState, TxReason.

	return &retainer.BlockMetadata{Status: status}, nil
}

func (r *Retainer) blockStatus(blockID iotago.BlockID) (models.BlockState, error) {
	_, blockStorageErr := r.blockDiskFunc(blockID.Index()).Load(blockID)
	// block was for sure accepted
	if blockStorageErr == nil {
		// check if finalized
		if blockID.Index() <= r.finalizedSlotFunc() {
			return models.BlockStateFinalized, nil
		}
		// check if confirmed
		if confirmed, err := r.retainerFunc(blockID.Index()).WasConfirmed(blockID); err != nil {
			return models.BlockStateUnknown, err
		} else if confirmed {
			return models.BlockStateConfirmed, nil
		}
		return models.BlockStateAccepted, nil
	}
	// orphaned (attached, but never accepeted)
	if orphaned, err := r.retainerFunc(blockID.Index()).WasOrphaned(blockID); err != nil {
		return models.BlockStateUnknown, err
	} else if orphaned {
		return models.BlockStateOrphaned, nil
	}

	return models.BlockStateUnknown, nil
}

func (r *Retainer) onBlockAttached(blockID iotago.BlockID) error {
	retainerStore := r.retainerFunc(blockID.Index())
	return retainerStore.Store(blockID)
}

func (r *Retainer) onBlockAccepted(blockID iotago.BlockID) error {
	retainerStore := r.retainerFunc(blockID.Index())
	return retainerStore.StoreAccepted(blockID)
}

func (r *Retainer) onBlockConfirmed(blockID iotago.BlockID) error {
	retainerStore := r.retainerFunc(blockID.Index())
	return retainerStore.StoreConfirmed(blockID)
}
