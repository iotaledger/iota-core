package inmemoryblockdag

import (
	"github.com/iotaledger/hive.go/core/causalorder"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/buffer"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

// BlockDAG is a causally ordered DAG that forms the central data structure of the IOTA protocol.
type BlockDAG struct {
	// Events contains the Events of the BlockDAG.
	events *blockdag.Events

	// evictionState contains information about the current eviction state.
	evictionState *eviction.State

	// solidifier contains the solidifier instance used to determine the solidity of Blocks.
	solidifier *causalorder.CausalOrder[iotago.SlotIndex, iotago.BlockID, *blocks.Block]

	latestCommitmentFunc  func() *model.Commitment
	uncommittedSlotBlocks *buffer.UnsolidCommitmentBuffer[*blocks.Block]

	retainBlockFailure func(blockID iotago.BlockID, failureReason apimodels.BlockFailureReason)

	blockCache *blocks.Blocks

	solidifierMutex syncutils.RWMutex

	workers    *workerpool.Group
	workerPool *workerpool.WorkerPool

	errorHandler func(error)
	apiProvider  iotago.APIProvider

	module.Module
}

func NewProvider(opts ...options.Option[BlockDAG]) module.Provider[*engine.Engine, blockdag.BlockDAG] {
	return module.Provide(func(e *engine.Engine) blockdag.BlockDAG {
		b := New(e.Workers.CreateGroup("BlockDAG"), e, e.EvictionState, e.BlockCache, e.ErrorHandler("blockdag"), opts...)

		e.HookConstructed(func() {
			wp := b.workers.CreatePool("BlockDAG.Attach", 2)

			e.Events.Filter.BlockPreAllowed.Hook(func(block *model.Block) {
				if _, _, err := b.Attach(block); err != nil {
					b.errorHandler(ierrors.Wrapf(err, "failed to attach block with %s (issuerID: %s)", block.ID(), block.ProtocolBlock().IssuerID))
				}
			}, event.WithWorkerPool(wp))

			e.Events.Notarization.LatestCommitmentUpdated.Hook(func(commitment *model.Commitment) {
				unsolidBlocks := b.uncommittedSlotBlocks.GetValuesAndEvict(commitment.ID())

				b.solidifierMutex.RLock()
				defer b.solidifierMutex.RUnlock()

				for _, block := range unsolidBlocks {
					b.solidifier.Queue(block)
				}
			}, event.WithWorkerPool(wp))

			b.setRetainBlockFailureFunc(e.Retainer.RetainBlockFailure)
			b.latestCommitmentFunc = e.Storage.Settings().LatestCommitment

			e.Events.BlockDAG.LinkTo(b.events)

			b.TriggerInitialized()
		})

		return b
	})
}

// New is the constructor for the BlockDAG and creates a new BlockDAG instance.
func New(workers *workerpool.Group, apiProvider iotago.APIProvider, evictionState *eviction.State, blockCache *blocks.Blocks, errorHandler func(error), opts ...options.Option[BlockDAG]) (newBlockDAG *BlockDAG) {
	return options.Apply(&BlockDAG{
		apiProvider:           apiProvider,
		events:                blockdag.NewEvents(),
		evictionState:         evictionState,
		blockCache:            blockCache,
		workers:               workers,
		workerPool:            workers.CreatePool("Solidifier", 2),
		errorHandler:          errorHandler,
		uncommittedSlotBlocks: buffer.NewUnsolidCommitmentBuffer[*blocks.Block](int(apiProvider.CurrentAPI().ProtocolParameters().MaxCommittableAge()) * 2),
	}, opts,
		func(b *BlockDAG) {
			b.solidifier = causalorder.New(
				b.workerPool,
				blockCache.Block,
				(*blocks.Block).IsSolid,
				b.markSolid,
				b.markInvalid,
				(*blocks.Block).Parents,
				causalorder.WithReferenceValidator[iotago.SlotIndex, iotago.BlockID](checkReference),
			)

			blockCache.Evict.Hook(b.evictSlot)
		},
		(*BlockDAG).TriggerConstructed,
		(*BlockDAG).TriggerInitialized,
	)
}

var _ blockdag.BlockDAG = new(BlockDAG)

// Attach is used to attach new Blocks to the BlockDAG. It is the main function of the BlockDAG that triggers Events.
func (b *BlockDAG) Attach(data *model.Block) (block *blocks.Block, wasAttached bool, err error) {
	if block, wasAttached, err = b.attach(data); wasAttached {
		b.events.BlockAttached.Trigger(block)

		// We add blocks that commit to a commitment we haven't committed ourselves yet to this limited size buffer and
		// only let them become solid once we committed said slot ourselves (to the same commitment).
		// This is necessary in order to make sure that all necessary state is available after a block is solid (specifically
		// the state of the referenced commitment for the commitment filter). All the while, we need to make sure that
		// the requesting of missing blocks (done in b.attach) continues so that we can advance our state and eventually
		// commit the slot.
		// This limited size buffer has a nice side effect: In normal behavior (e.g. no attack of a neighbor that sends you
		// unsolidifiable blocks in your committed slots) it will prevent the node from storing too many blocks in memory.
		if b.uncommittedSlotBlocks.AddWithFunc(block.SlotCommitmentID(), block, func() bool {
			return block.SlotCommitmentID().Index() > b.latestCommitmentFunc().Commitment().Index
		}) {
			return
		}

		b.solidifierMutex.RLock()
		defer b.solidifierMutex.RUnlock()

		b.solidifier.Queue(block)
	}

	return
}

// GetOrRequestBlock returns the Block with the given BlockID from the BlockDAG (and requests it from the network if it
// is missing). If the requested Block is below the eviction threshold, then this method will return a nil block without
// creating it.
func (b *BlockDAG) GetOrRequestBlock(blockID iotago.BlockID) (block *blocks.Block, requested bool) {
	return b.blockCache.GetOrCreate(blockID, func() (newBlock *blocks.Block) {
		newBlock = blocks.NewMissingBlock(blockID)
		b.events.BlockMissing.Trigger(newBlock)

		return newBlock
	})
}

// SetInvalid marks a Block as invalid.
func (b *BlockDAG) SetInvalid(block *blocks.Block, reason error) (wasUpdated bool) {
	if wasUpdated = block.SetInvalid(); wasUpdated {
		b.events.BlockInvalid.Trigger(block, reason)
	}

	return
}

func (b *BlockDAG) Shutdown() {
	b.TriggerStopped()
	b.workers.Shutdown()
}

func (b *BlockDAG) setRetainBlockFailureFunc(retainBlockFailure func(blockID iotago.BlockID, failureReason apimodels.BlockFailureReason)) {
	b.retainBlockFailure = retainBlockFailure
}

// evictSlot is used to evict Blocks from committed slots from the BlockDAG.
func (b *BlockDAG) evictSlot(index iotago.SlotIndex) {
	b.solidifierMutex.Lock()
	defer b.solidifierMutex.Unlock()

	b.solidifier.EvictUntil(index)
}

func (b *BlockDAG) markSolid(block *blocks.Block) (err error) {
	if block.SetSolid() {
		b.events.BlockSolid.Trigger(block)
	}

	return nil
}

func (b *BlockDAG) markInvalid(block *blocks.Block, reason error) {
	b.SetInvalid(block, ierrors.Wrap(reason, "block marked as invalid in BlockDAG"))
}

// attach tries to attach the given Block to the BlockDAG.
func (b *BlockDAG) attach(data *model.Block) (block *blocks.Block, wasAttached bool, err error) {
	shouldAttach, err := b.shouldAttach(data)

	if !shouldAttach {
		return nil, false, err
	}

	block, evicted, updated := b.blockCache.StoreOrUpdate(data)

	if evicted {
		b.retainBlockFailure(data.ID(), apimodels.BlockFailureIsTooOld)
		return block, false, ierrors.New("cannot attach, block is too old, it was already evicted from the cache")
	}

	if updated {
		b.events.MissingBlockAttached.Trigger(block)
	}

	block.ForEachParent(func(parent iotago.Parent) {
		b.registerChild(block, parent)
	})

	return block, true, nil
}

// canAttach determines if the Block can be attached (does not exist and addresses a recent slot).
func (b *BlockDAG) shouldAttach(data *model.Block) (shouldAttach bool, err error) {
	if b.evictionState.InRootBlockSlot(data.ID()) && !b.evictionState.IsRootBlock(data.ID()) {
		b.retainBlockFailure(data.ID(), apimodels.BlockFailureIsTooOld)
		return false, ierrors.Errorf("block data with %s is too old (issued at: %s)", data.ID(), data.ProtocolBlock().IssuingTime)
	}

	storedBlock, storedBlockExists := b.blockCache.Block(data.ID())
	// We already attached it before
	if storedBlockExists && !storedBlock.IsMissing() {
		return false, nil
	}

	// We already attached it before, but the parents are invalid, then we set the block as invalid.
	if parentsValid, err := b.canAttachToParents(data); !parentsValid && storedBlock != nil {
		storedBlock.SetInvalid()
		return false, err
	}

	return true, nil
}

// canAttachToParents determines if the Block references parents in a non-pruned slot. If a Block is found to violate
// this condition but exists as a missing entry, we mark it as invalid.
func (b *BlockDAG) canAttachToParents(modelBlock *model.Block) (parentsValid bool, err error) {
	for _, parentID := range modelBlock.ProtocolBlock().Parents() {
		if b.evictionState.InRootBlockSlot(parentID) && !b.evictionState.IsRootBlock(parentID) {
			b.retainBlockFailure(modelBlock.ID(), apimodels.BlockFailureParentIsTooOld)
			return false, ierrors.Errorf("parent %s of block %s is too old", parentID, modelBlock.ID())
		}
	}

	return true, nil
}

// registerChild registers the given Block as a child of the parent. It triggers a BlockMissing event if the referenced
// Block does not exist, yet.
func (b *BlockDAG) registerChild(child *blocks.Block, parent iotago.Parent) {
	if b.evictionState.IsRootBlock(parent.ID) {
		return
	}

	if parentBlock, _ := b.GetOrRequestBlock(parent.ID); parentBlock != nil {
		parentBlock.AppendChild(child, parent.Type)
	}
}

// checkReference checks if the reference between the child and its parent is valid.
func checkReference(child *blocks.Block, parent *blocks.Block) (err error) {
	if parent.IsInvalid() {
		return ierrors.Errorf("parent %s of child %s is marked as invalid", parent.ID(), child.ID())
	}

	return nil
}
