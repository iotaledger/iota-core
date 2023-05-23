package tipmanagerv1

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/randommap"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TipManager is a component that manages the tips of the Tangle.
type TipManager struct {
	// retrieveBlock is a function that retrieves a Block from the Tangle.
	retrieveBlock func(blockID iotago.BlockID) (block *blocks.Block, exists bool)

	// blocks contains the blocks that are managed by the TipManager.
	blocks *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *shrinkingmap.ShrinkingMap[iotago.BlockID, *Block]]

	// strongTips contains the strong tips that are available for tip selection.
	strongTips *randommap.RandomMap[iotago.BlockID, *Block]

	// weakTips contains the weak tips that are available for tip selection.
	weakTips *randommap.RandomMap[iotago.BlockID, *Block]

	// blockAdded is triggered when a new Block was added to the TipManager.
	blockAdded *event.Event1[*Block]

	// lastEvictedSlot is the last slot index that was evicted from the MemPool.
	lastEvictedSlot iotago.SlotIndex

	// evictionMutex is used to synchronize the eviction of slots.
	evictionMutex sync.RWMutex
}

// NewTipManager creates a new TipManager.
func NewTipManager(blockRetriever func(blockID iotago.BlockID) (block *blocks.Block, exists bool)) *TipManager {
	return &TipManager{
		retrieveBlock: blockRetriever,
		blocks:        shrinkingmap.New[iotago.SlotIndex, *shrinkingmap.ShrinkingMap[iotago.BlockID, *Block]](),
		strongTips:    randommap.New[iotago.BlockID, *Block](),
		weakTips:      randommap.New[iotago.BlockID, *Block](),
		blockAdded:    event.New1[*Block](),
	}
}

// AddBlock adds a Block to the TipManager.
func (t *TipManager) AddBlock(block *blocks.Block) {
	newBlock := NewBlock(block)

	if blocks := t.blocksBySlotIndex(block.ID().Index()); blocks != nil && blocks.Set(block.ID(), newBlock) {
		t.setupBlock(newBlock)
	}
}

// OnBlockAdded registers a callback that is triggered when a new Block was added to the TipManager.
func (t *TipManager) OnBlockAdded(handler func(block *Block)) (unsubscribe func()) {
	return t.blockAdded.Hook(handler).Unhook
}

// Evict evicts a slot from the TipManager.
func (t *TipManager) Evict(slotIndex iotago.SlotIndex) {
	if t.evict(slotIndex) {
		if evictedBlocks, deleted := t.blocks.DeleteAndReturn(slotIndex); deleted {
			evictedBlocks.ForEach(func(_ iotago.BlockID, block *Block) bool {
				block.evicted.Trigger()

				return true
			})
		}
	}
}

// setupBlock sets up the behavior of the given Block.
func (t *TipManager) setupBlock(block *Block) {
	block.stronglyConnectedToTips.OnUpdate(func(_, isConnected bool) {
		t.updateParents(block, propagateConnectedChildren(isConnected, true))
	})

	block.weaklyConnectedToTips.OnUpdate(func(_, isConnected bool) {
		t.updateParents(block, propagateConnectedChildren(isConnected, false))
	})

	joinTipPool := func(tipSet *randommap.RandomMap[iotago.BlockID, *Block], blockReferencedByTips *promise.Value[bool]) (leaveTipPool func()) {
		unsubscribe := blockReferencedByTips.OnUpdate(func(_, isReferenced bool) {
			if isReferenced {
				tipSet.Delete(block.ID())
			} else {
				tipSet.Set(block.ID(), block)
			}
		})

		return func() {
			unsubscribe()

			tipSet.Delete(block.ID())
		}
	}

	var leaveTipPool func()

	block.tipPool.OnUpdate(func(prevTipPool, newTipPool TipPool) {
		if leaveTipPool != nil {
			leaveTipPool()
		}

		if newTipPool == StrongTipPool {
			leaveTipPool = joinTipPool(t.strongTips, block.stronglyReferencedByTips)
		} else if newTipPool == WeakTipPool {
			leaveTipPool = joinTipPool(t.weakTips, block.referencedByTips)
		} else {
			leaveTipPool = nil
		}
	})

	block.setTipPool(t.determineInitialTipPool(block))

	t.blockAdded.Trigger(block)
}

// determineInitialTipPool determines the initial TipPool of the given Block.
func (t *TipManager) determineInitialTipPool(block *Block, optMinType ...TipPool) TipPool {
	blockIsVotingForNonRejectedBranches := func(block *Block) bool {
		// TODO: implement check of conflict dag
		return true
	}

	payloadIsLiked := func(block *Block) bool {
		// TODO: implement check of conflict dag
		return true
	}

	if lo.First(optMinType) <= StrongTipPool && blockIsVotingForNonRejectedBranches(block) {
		return StrongTipPool
	}

	if lo.First(optMinType) <= WeakTipPool && payloadIsLiked(block) {
		return WeakTipPool
	}

	return DroppedTipPool
}

// updateParents updates the parents of the given Block.
func (t *TipManager) updateParents(block *Block, parentTypeSpecificUpdates map[model.ParentsType]func(*Block)) {
	block.ForEachParent(func(parent model.Parent) {
		if blocks := t.blocksBySlotIndex(parent.ID.Index()); blocks != nil {
			// TODO: MAKE GetOrCreate ignore nil return values
			if parentBlock, created := blocks.GetOrCreate(parent.ID, func() *Block {
				if parentBlock, parentBlockExists := t.retrieveBlock(parent.ID); parentBlockExists {
					return NewBlock(parentBlock)
				}

				return nil
			}); parentBlock != nil {
				if parentTypeSpecificUpdate, exists := parentTypeSpecificUpdates[parent.Type]; exists {
					parentTypeSpecificUpdate(parentBlock)
				}

				if created {
					t.setupBlock(parentBlock)
				}
			}
		}
	})
}

// blocksBySlotIndex returns the Blocks of the given SlotIndex.
func (t *TipManager) blocksBySlotIndex(slotIndex iotago.SlotIndex) (blocks *shrinkingmap.ShrinkingMap[iotago.BlockID, *Block]) {
	t.evictionMutex.RLock()
	defer t.evictionMutex.RUnlock()

	if t.lastEvictedSlot >= slotIndex {
		return nil
	}

	return lo.Return1(t.blocks.GetOrCreate(slotIndex, lo.NoVariadic(shrinkingmap.New[iotago.BlockID, *Block])))
}

// evict updates the last evicted slot of the TipManager.
func (t *TipManager) evict(slotIndex iotago.SlotIndex) (evicted bool) {
	t.evictionMutex.Lock()
	defer t.evictionMutex.Unlock()

	if evicted = t.lastEvictedSlot < slotIndex; evicted {
		t.lastEvictedSlot = slotIndex
	}

	return evicted
}
