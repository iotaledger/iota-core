package inmemoryblockdag

import (
	"sync/atomic"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/buffer"
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

	latestCommitmentFunc  func() *model.Commitment
	uncommittedSlotBlocks *buffer.UnsolidCommitmentBuffer[*blocks.Block]

	blockCache *blocks.Blocks

	workers      *workerpool.Group
	errorHandler func(error)

	module.Module
}

func NewProvider(opts ...options.Option[BlockDAG]) module.Provider[*engine.Engine, blockdag.BlockDAG] {
	return module.Provide(func(e *engine.Engine) blockdag.BlockDAG {
		b := New(e.NewSubModule("BlockDAG"), e.Workers.CreateGroup("BlockDAG"), int(e.Storage.Settings().APIProvider().CommittedAPI().ProtocolParameters().MaxCommittableAge())*2, e.EvictionState, e.BlockCache, e.ErrorHandler("blockdag"), opts...)

		e.ConstructedEvent().OnTrigger(func() {
			b.Init(e.SyncManager.LatestCommitment)

			wp := b.workers.CreatePool("BlockDAG.Append", workerpool.WithWorkerCount(2))

			e.Events.PreSolidFilter.BlockPreAllowed.Hook(func(block *model.Block) {
				if _, _, err := b.Append(block); err != nil {
					b.LogError("failed to append block", "blockID", block.ID(), "issuer", block.ProtocolBlock().Header.IssuerID, "err", err)
				}
			}, event.WithWorkerPool(wp))

			e.Events.Notarization.LatestCommitmentUpdated.Hook(func(commitment *model.Commitment) {
				for _, block := range b.uncommittedSlotBlocks.GetValuesAndEvict(commitment.ID()) {
					b.LogTrace("remove from uncommittedSlotBlocks", "block", block.ID(), "commitmentID", block.SlotCommitmentID())
					b.setupBlock(block)
				}
			}, event.WithWorkerPool(wp))

			e.Events.BlockDAG.LinkTo(b.events)
		})

		return b
	})
}

// New is the constructor for the BlockDAG and creates a new BlockDAG instance.
func New(subModule module.Module, workers *workerpool.Group, unsolidCommitmentBufferSize int, evictionState *eviction.State, blockCache *blocks.Blocks, errorHandler func(error), opts ...options.Option[BlockDAG]) (newBlockDAG *BlockDAG) {
	return options.Apply(&BlockDAG{
		Module:                subModule,
		events:                blockdag.NewEvents(),
		evictionState:         evictionState,
		blockCache:            blockCache,
		workers:               workers,
		errorHandler:          errorHandler,
		uncommittedSlotBlocks: buffer.NewUnsolidCommitmentBuffer[*blocks.Block](unsolidCommitmentBufferSize),
	}, opts, func(b *BlockDAG) {
		b.ShutdownEvent().OnTrigger(b.shutdown)

		b.ConstructedEvent().Trigger()
	})
}

func (b *BlockDAG) Init(latestCommitmentFunc func() *model.Commitment) {
	b.latestCommitmentFunc = latestCommitmentFunc

	b.InitializedEvent().Trigger()
}

// Append is used to append new Blocks to the BlockDAG. It is the main function of the BlockDAG that triggers Events.
func (b *BlockDAG) Append(modelBlock *model.Block) (block *blocks.Block, wasAppended bool, err error) {
	if block, wasAppended, err = b.append(modelBlock); wasAppended {
		// We add blocks that commit to a commitment we haven't committed ourselves yet to this limited size buffer and
		// only let them become solid once we committed said slot ourselves (to the same commitment).
		// This is necessary in order to make sure that all necessary state is available after a block is solid (specifically
		// the state of the referenced commitment for the commitment filter). All the while, we need to make sure that
		// the requesting of missing blocks (done in b.append) continues so that we can advance our state and eventually
		// commit the slot.
		// This limited size buffer has a nice side effect: In normal behavior (e.g. no attack of a neighbor that sends you
		// unsolidifiable blocks in your committed slots) it will prevent the node from storing too many blocks in memory.
		if b.uncommittedSlotBlocks.AddWithFunc(block.SlotCommitmentID(), block, func() bool {
			return block.SlotCommitmentID().Slot() > b.latestCommitmentFunc().Commitment().Slot
		}) {
			b.LogTrace("add to uncommittedSlotBlocks", "block", block.ID(), "commitmentID", block.SlotCommitmentID())
			return
		}

		b.LogTrace("block appended", "block", block.ID())

		b.events.BlockAppended.Trigger(block)

		b.setupBlock(block)
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

// Reset resets the component to a clean state as if it was created at the last commitment.
func (b *BlockDAG) Reset() {
	b.uncommittedSlotBlocks.Reset()
}

func (b *BlockDAG) setupBlock(block *blocks.Block) {
	var unsolidParentsCount atomic.Int32
	unsolidParentsCount.Store(int32(len(block.Parents())))

	block.ForEachParent(func(parent iotago.Parent) {
		parentBlock, exists := b.blockCache.Block(parent.ID)
		if !exists {
			b.errorHandler(ierrors.Errorf("failed to setup block %s, parent %s is missing", block.ID(), parent.ID))

			return
		}

		parentBlock.Solid().OnUpdateOnce(func(_ bool, _ bool) {
			if unsolidParentsCount.Add(-1) == 0 {
				if block.SetSolid() {
					b.events.BlockSolid.Trigger(block)
				}
			}
		})

		parentBlock.Invalid().OnUpdateOnce(func(_ bool, _ bool) {
			if block.SetInvalid() {
				b.events.BlockInvalid.Trigger(block, ierrors.Errorf("parent block %s is marked as invalid", parent.ID))
			}
		})
	})
}

// append tries to append the given Block to the BlockDAG.
func (b *BlockDAG) append(modelBlock *model.Block) (block *blocks.Block, wasAppended bool, err error) {
	shouldAppend, err := b.shouldAppend(modelBlock)

	if !shouldAppend {
		return nil, false, err
	}

	block, evicted, updated := b.blockCache.StoreOrUpdate(modelBlock)

	if evicted {
		return block, false, ierrors.New("cannot append, block is too old, it was already evicted from the cache")
	}

	if updated {
		b.LogTrace("missing block appended", "block", block.ID())

		b.events.MissingBlockAppended.Trigger(block)
	}

	block.ForEachParent(func(parent iotago.Parent) {
		b.registerChild(block, parent)
	})

	return block, true, nil
}

// shouldAppend determines if the Block can be appended (does not exist and addresses a recent slot).
func (b *BlockDAG) shouldAppend(modelBlock *model.Block) (shouldAppend bool, err error) {
	if isBelowRange, isInRange := b.evictionState.BelowOrInActiveRootBlockRange(modelBlock.ID()); isBelowRange || isInRange {
		return false, ierrors.Errorf("block data with %s is too old (issued at: %s)", modelBlock.ID(), modelBlock.ProtocolBlock().Header.IssuingTime)
	}

	storedBlock, storedBlockExists := b.blockCache.Block(modelBlock.ID())
	// We already appended it before
	if storedBlockExists && !storedBlock.IsMissing() {
		return false, nil
	}

	// We already appended it before, but the parents are invalid, then we set the block as invalid.
	if parentsValid, err := b.canAppendToParents(modelBlock); !parentsValid {
		if storedBlock != nil {
			storedBlock.SetInvalid()
		}

		return false, err
	}

	return true, nil
}

// canAppendToParents determines if the Block references parents in a non-pruned slot. If a Block is found to violate
// this condition but exists as a missing entry, we mark it as invalid.
func (b *BlockDAG) canAppendToParents(modelBlock *model.Block) (parentsValid bool, err error) {
	for _, parentID := range modelBlock.ProtocolBlock().Parents() {
		if isBelowRange, isInRange := b.evictionState.BelowOrInActiveRootBlockRange(parentID); isBelowRange || isInRange && !b.evictionState.IsActiveRootBlock(parentID) {
			return false, ierrors.Errorf("parent %s of block %s is too old", parentID, modelBlock.ID())
		}
	}

	return true, nil
}

// registerChild registers the given Block as a child of the parent. It triggers a BlockMissing event if the referenced
// Block does not exist, yet.
func (b *BlockDAG) registerChild(child *blocks.Block, parent iotago.Parent) {
	if b.evictionState.IsActiveRootBlock(parent.ID) {
		return
	}

	if parentBlock, _ := b.GetOrRequestBlock(parent.ID); parentBlock != nil {
		parentBlock.AppendChild(child, parent.Type)
	}
}

func (b *BlockDAG) shutdown() {
	b.workers.Shutdown()

	b.StoppedEvent().Trigger()
}
