package inmemoryblockdag

import (
	"fmt"
	"sync"

	"github.com/iotaledger/hive.go/core/causalorder"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	iotago "github.com/iotaledger/iota.go/v4"
)

// BlockDAG is a causally ordered DAG that forms the central data structure of the IOTA protocol.
type BlockDAG struct {
	// Events contains the Events of the BlockDAG.
	events *blockdag.Events

	// evictionState contains information about the current eviction state.
	evictionState *eviction.State

	// solidifier contains the solidifier instance used to determine the solidity of Blocks.
	solidifier *causalorder.CausalOrder[iotago.SlotIndex, iotago.BlockID, *blocks.Block]

	blockCache *blocks.Blocks

	solidifierMutex sync.RWMutex

	workers    *workerpool.Group
	workerPool *workerpool.WorkerPool

	errorHandler func(error)

	module.Module
}

func NewProvider(opts ...options.Option[BlockDAG]) module.Provider[*engine.Engine, blockdag.BlockDAG] {
	return module.Provide(func(e *engine.Engine) blockdag.BlockDAG {
		b := New(e.Workers.CreateGroup("BlockDAG"), e.EvictionState, e.BlockCache, e.ErrorHandler("blockdag"), opts...)

		e.HookConstructed(func() {
			wp := b.workers.CreatePool("BlockDAG.Attach", 2)

			e.Events.CommitmentFilter.BlockAllowed.Hook(func(block *model.Block) {
				if _, _, err := b.Attach(block); err != nil {
					b.errorHandler(ierrors.Wrapf(err, "failed to attach block with %s (issuerID: %s)", block.ID(), block.ProtocolBlock().IssuerID))
				}
			}, event.WithWorkerPool(wp))

			e.Events.BlockDAG.LinkTo(b.events)

			b.TriggerInitialized()
		})

		return b
	})
}

// New is the constructor for the BlockDAG and creates a new BlockDAG instance.
func New(workers *workerpool.Group, evictionState *eviction.State, blockCache *blocks.Blocks, errorHandler func(error), opts ...options.Option[BlockDAG]) (newBlockDAG *BlockDAG) {
	return options.Apply(&BlockDAG{
		events:        blockdag.NewEvents(),
		evictionState: evictionState,
		blockCache:    blockCache,
		workers:       workers,
		workerPool:    workers.CreatePool("Solidifier", 2),
		errorHandler:  errorHandler,
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

		b.solidifierMutex.RLock()
		defer b.solidifierMutex.RUnlock()

		b.solidifier.Queue(block)
	}

	return
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

func (b *BlockDAG) checkParents(block *blocks.Block) (err error) {
	for _, parentID := range block.Parents() {
		parent, parentExists := b.blockCache.Block(parentID)
		if !parentExists {
			panic(fmt.Sprintf("parent %s of block %s should exist as block was marked ordered by the solidifier", parentID, block.ID()))
		}

		// check timestamp monotonicity
		if parent.IssuingTime().After(block.IssuingTime()) {
			return ierrors.Errorf("timestamp monotonicity check failed for parent %s with timestamp %s. block timestamp %s", parent.ID(), parent.IssuingTime(), block.IssuingTime())
		}

		// check commitment monotonicity
		if parent.SlotCommitmentID().Index() > block.SlotCommitmentID().Index() {
			return ierrors.Errorf("commitment monotonicity check failed for parent %s with commitment index %d. block commitment index %d", parentID, parent.SlotCommitmentID().Index(), block.SlotCommitmentID().Index())
		}
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
		return block, false, ierrors.New("block is too old")
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

	parentBlock, _ := b.blockCache.GetOrCreate(parent.ID, func() (newBlock *blocks.Block) {
		newBlock = blocks.NewMissingBlock(parent.ID)
		b.events.BlockMissing.Trigger(newBlock)

		return newBlock
	})

	if parentBlock != nil {
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
