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

	// blockMetadataStorage contains the BlockMetadata of all Blocks that are managed by the TipManager.
	blockMetadataStorage *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *shrinkingmap.ShrinkingMap[iotago.BlockID, *BlockMetadata]]

	// strongTipSet contains the blocks of the strong tip pool that have no referencing children.
	strongTipSet *randommap.RandomMap[iotago.BlockID, *BlockMetadata]

	// weakTipSet contains the blocks of the weak tip pool that have no referencing children.
	weakTipSet *randommap.RandomMap[iotago.BlockID, *BlockMetadata]

	// blockAdded is triggered when a new Block was added to the TipManager.
	blockAdded *event.Event1[*BlockMetadata]

	// lastEvictedSlot is the last slot index that was evicted from the MemPool.
	lastEvictedSlot iotago.SlotIndex

	// evictionMutex is used to synchronize the eviction of slots.
	evictionMutex sync.RWMutex
}

// NewTipManager creates a new TipManager.
func NewTipManager(blockRetriever func(blockID iotago.BlockID) (block *blocks.Block, exists bool)) *TipManager {
	return &TipManager{
		retrieveBlock:        blockRetriever,
		blockMetadataStorage: shrinkingmap.New[iotago.SlotIndex, *shrinkingmap.ShrinkingMap[iotago.BlockID, *BlockMetadata]](),
		strongTipSet:         randommap.New[iotago.BlockID, *BlockMetadata](),
		weakTipSet:           randommap.New[iotago.BlockID, *BlockMetadata](),
		blockAdded:           event.New1[*BlockMetadata](),
	}
}

// AddBlock adds a Block to the TipManager.
func (t *TipManager) AddBlock(block *blocks.Block) {
	newBlockMetadata := NewBlockMetadata(block)

	if storage := t.metadataStorage(block.ID().Index()); storage != nil && storage.Set(block.ID(), newBlockMetadata) {
		t.setupBlockMetadata(newBlockMetadata)
	}
}

// StrongTipSet returns the strong tip set of the TipManager.
func (t *TipManager) StrongTipSet() (tipSet []*blocks.Block) {
	t.strongTipSet.ForEach(func(_ iotago.BlockID, blockMetadata *BlockMetadata) bool {
		tipSet = append(tipSet, blockMetadata.Block)

		return true
	})

	return tipSet
}

// WeakTipSet returns the weak tip set of the TipManager.
func (t *TipManager) WeakTipSet() (tipSet []*blocks.Block) {
	t.weakTipSet.ForEach(func(_ iotago.BlockID, blockMetadata *BlockMetadata) bool {
		tipSet = append(tipSet, blockMetadata.Block)

		return true
	})

	return tipSet
}

// OnBlockAdded registers a callback that is triggered when a new Block was added to the TipManager.
func (t *TipManager) OnBlockAdded(handler func(blockMetadata *BlockMetadata)) (unsubscribe func()) {
	return t.blockAdded.Hook(handler).Unhook
}

// Evict evicts a slot from the TipManager.
func (t *TipManager) Evict(slotIndex iotago.SlotIndex) {
	if t.markSlotAsEvicted(slotIndex) {
		if evictedObjects, deleted := t.blockMetadataStorage.DeleteAndReturn(slotIndex); deleted {
			evictedObjects.ForEach(func(_ iotago.BlockID, blockMetadata *BlockMetadata) bool {
				blockMetadata.evicted.Trigger()

				return true
			})
		}
	}
}

// setupBlockMetadata sets up the behavior of the given Block.
func (t *TipManager) setupBlockMetadata(blockMetadata *BlockMetadata) {
	blockMetadata.stronglyConnectedToTips.OnUpdate(func(_, isConnected bool) {
		t.updateParents(blockMetadata, propagateConnectedChildren(isConnected, true))
	})

	blockMetadata.weaklyConnectedToTips.OnUpdate(func(_, isConnected bool) {
		t.updateParents(blockMetadata, propagateConnectedChildren(isConnected, false))
	})

	joinTipPool := func(tipSet *randommap.RandomMap[iotago.BlockID, *BlockMetadata], blockReferencedByTips *promise.Value[bool]) (leaveTipPool func()) {
		unsubscribe := blockReferencedByTips.OnUpdate(func(_, isReferenced bool) {
			if isReferenced {
				tipSet.Delete(blockMetadata.ID())
			} else {
				tipSet.Set(blockMetadata.ID(), blockMetadata)
			}
		})

		return func() {
			unsubscribe()

			tipSet.Delete(blockMetadata.ID())
		}
	}

	var leaveTipPool func()

	blockMetadata.tipPool.OnUpdate(func(prevTipPool, newTipPool TipPool) {
		if leaveTipPool != nil {
			leaveTipPool()
		}

		if newTipPool == StrongTipPool {
			leaveTipPool = joinTipPool(t.strongTipSet, blockMetadata.stronglyReferencedByTips)
		} else if newTipPool == WeakTipPool {
			leaveTipPool = joinTipPool(t.weakTipSet, blockMetadata.referencedByTips)
		} else {
			leaveTipPool = nil
		}
	})

	blockMetadata.setTipPool(t.determineTipPool(blockMetadata))

	t.blockAdded.Trigger(blockMetadata)
}

// determineTipPool determines the initial TipPool of the given Block.
func (t *TipManager) determineTipPool(blockMetadata *BlockMetadata, minPool ...TipPool) TipPool {
	blockIsVotingForNonRejectedBranches := func(blockMetadata *BlockMetadata) bool {
		// TODO: implement check of conflict dag
		return true
	}

	payloadIsLiked := func(blockMetadata *BlockMetadata) bool {
		// TODO: implement check of conflict dag
		return true
	}

	if lo.First(minPool) <= StrongTipPool && blockIsVotingForNonRejectedBranches(blockMetadata) {
		return StrongTipPool
	}

	if lo.First(minPool) <= WeakTipPool && payloadIsLiked(blockMetadata) {
		return WeakTipPool
	}

	return DroppedTipPool
}

// updateParents updates the parents of the given Block.
func (t *TipManager) updateParents(blockMetadata *BlockMetadata, updates map[model.ParentsType]func(*BlockMetadata)) {
	blockMetadata.ForEachParent(func(parent model.Parent) {
		storage := t.metadataStorage(parent.ID.Index())
		if storage == nil {
			return
		}

		parentMetadata, created := storage.GetOrCreate(parent.ID, func() *BlockMetadata { return NewBlockMetadata(lo.Return1(t.retrieveBlock(parent.ID))) })
		if parentMetadata.Block == nil {
			return
		} else if created {
			t.setupBlockMetadata(parentMetadata)
		}

		if update, exists := updates[parent.Type]; exists {
			update(parentMetadata)
		}
	})
}

// metadataStorage returns the BlockMetadata storage for the given slotIndex.
func (t *TipManager) metadataStorage(slotIndex iotago.SlotIndex) (storage *shrinkingmap.ShrinkingMap[iotago.BlockID, *BlockMetadata]) {
	t.evictionMutex.RLock()
	defer t.evictionMutex.RUnlock()

	if t.lastEvictedSlot >= slotIndex {
		return nil
	}

	return lo.Return1(t.blockMetadataStorage.GetOrCreate(slotIndex, lo.NoVariadic(shrinkingmap.New[iotago.BlockID, *BlockMetadata])))
}

// markSlotAsEvicted marks the given slotIndex as evicted.
func (t *TipManager) markSlotAsEvicted(slotIndex iotago.SlotIndex) (success bool) {
	t.evictionMutex.Lock()
	defer t.evictionMutex.Unlock()

	if success = t.lastEvictedSlot < slotIndex; success {
		t.lastEvictedSlot = slotIndex
	}

	return success
}
