package inmemoryblockdag

import (
	"fmt"

	"github.com/iotaledger/hive.go/core/causalorder"
	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
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

	// shouldParkFutureBlocksFunc is a function that returns whether the BlockDAG should park future Blocks.
	shouldParkFutureBlocksFunc func() bool

	// commitmentFunc is a function that returns the commitment corresponding to the given slot index.
	commitmentFunc func(index iotago.SlotIndex) (*model.Commitment, error)

	// futureBlocks contains blocks with a commitment in the future, that should not be passed to the booker yet.
	futureBlocks *memstorage.IndexedStorage[iotago.SlotIndex, iotago.CommitmentID, *advancedset.AdvancedSet[*blocks.Block]]

	nextIndexToPromote iotago.SlotIndex

	blockCache *blocks.Blocks

	// The Queue always read-locks the eviction mutex of the solidifier, and then evaluates if the block is
	// future thus read-locking the futureBlocks mutex. At the same time, when re-adding parked blocks,
	// promoteFutureBlocksMethod write-locks the futureBlocks mutex, and then read-locks the eviction mutex
	// of the solidifier. As the locks are non-starving, and locks are interlocked in different orders a
	// deadlock can occur only when an eviction is triggered while the above scenario unfolds.
	solidifierMutex syncutils.RWMutex

	futureBlocksMutex syncutils.RWMutex

	workers    *workerpool.Group
	workerPool *workerpool.WorkerPool

	errorHandler func(error)

	module.Module
}

func NewProvider(opts ...options.Option[BlockDAG]) module.Provider[*engine.Engine, blockdag.BlockDAG] {
	return module.Provide(func(e *engine.Engine) blockdag.BlockDAG {
		b := New(e.Workers.CreateGroup("BlockDAG"), e.EvictionState, e.BlockCache, e.IsBootstrapped, e.Storage.Commitments().Load, e.ErrorHandler("blockdag"), opts...)

		e.HookConstructed(func() {
			wp := b.workers.CreatePool("BlockDAG.Attach", 2)

			e.Events.Filter.BlockAllowed.Hook(func(block *model.Block) {
				if _, _, err := b.Attach(block); err != nil {
					b.errorHandler(ierrors.Wrapf(err, "failed to attach block with %s (issuerID: %s)", block.ID(), block.ProtocolBlock().IssuerID))
				}
			}, event.WithWorkerPool(wp))

			e.Events.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
				b.PromoteFutureBlocksUntil(details.Commitment.Index())
			}, event.WithWorkerPool(wp))

			e.Events.BlockDAG.LinkTo(b.events)

			b.TriggerInitialized()
		})

		return b
	})
}

// New is the constructor for the BlockDAG and creates a new BlockDAG instance.
func New(workers *workerpool.Group, evictionState *eviction.State, blockCache *blocks.Blocks, shouldParkBlocksFunc func() bool, latestCommitmentFunc func(iotago.SlotIndex) (*model.Commitment, error), errorHandler func(error), opts ...options.Option[BlockDAG]) (newBlockDAG *BlockDAG) {
	return options.Apply(&BlockDAG{
		events:                     blockdag.NewEvents(),
		evictionState:              evictionState,
		shouldParkFutureBlocksFunc: shouldParkBlocksFunc,
		commitmentFunc:             latestCommitmentFunc,
		blockCache:                 blockCache,
		futureBlocks:               memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.CommitmentID, *advancedset.AdvancedSet[*blocks.Block]](),
		workers:                    workers,
		workerPool:                 workers.CreatePool("Solidifier", 2),
		errorHandler:               errorHandler,
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

func (b *BlockDAG) PromoteFutureBlocksUntil(index iotago.SlotIndex) {
	b.solidifierMutex.RLock()
	defer b.solidifierMutex.RUnlock()
	b.futureBlocksMutex.Lock()
	defer b.futureBlocksMutex.Unlock()

	for i := b.nextIndexToPromote; i <= index; i++ {
		cm, err := b.commitmentFunc(i)
		if err != nil {
			panic(fmt.Sprintf("failed to load commitment for index %d: %s", i, err))
		}
		if storage := b.futureBlocks.Get(i, false); storage != nil {
			if futureBlocks, exists := storage.Get(cm.ID()); exists {
				_ = futureBlocks.ForEach(func(futureBlock *blocks.Block) (err error) {
					b.solidifier.Queue(futureBlock)
					return nil
				})
			}
		}
		b.futureBlocks.Evict(i)
	}

	b.nextIndexToPromote = index + 1
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
	// Future blocks already have passed these checks, as they are revisited again at a later point in time.
	// It is important to note that this check only passes once for a specific future block, as it is not yet marked as
	// such. The next time the block is added to the causal order and marked as solid (that is, when it got promoted),
	// it will be marked solid straight away, without checking if it is still a future block: that's why this method
	// must be only called at most twice for any block.
	if !block.IsFuture() {
		if err := b.checkParents(block); err != nil {
			return err
		}

		if b.shouldParkFutureBlocksFunc() && b.isFutureBlock(block) {
			return
		}
	}

	// It is important to only set the block as solid when it was not "parked" as a future block.
	// Future blocks are queued for solidification again when the slot is committed.
	if block.SetSolid() {
		b.events.BlockSolid.Trigger(block)
	}

	return nil
}

func (b *BlockDAG) isFutureBlock(block *blocks.Block) (isFutureBlock bool) {
	b.futureBlocksMutex.RLock()
	defer b.futureBlocksMutex.RUnlock()

	// If we are not able to load the commitment for the block, it means we haven't committed this slot yet.
	if _, err := b.commitmentFunc(block.SlotCommitmentID().Index()); err != nil {
		// We set the block as future block so that we can skip some checks when revisiting it later in markSolid via the solidifier.
		block.SetFuture()

		lo.Return1(b.futureBlocks.Get(block.SlotCommitmentID().Index(), true).GetOrCreate(block.SlotCommitmentID(), func() *advancedset.AdvancedSet[*blocks.Block] {
			return advancedset.New[*blocks.Block]()
		})).Add(block)

		return true
	}

	return false
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
