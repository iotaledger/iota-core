package tipmanagerv1

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/randommap"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TipManager is a component that manages the selectTips of the Tangle.
type TipManager struct {
	// retrieveBlock is a function that retrieves a Block from the Tangle.
	retrieveBlock func(blockID iotago.BlockID) (block *blocks.Block, exists bool)

	// tipMetadataStorage contains the TipMetadata of all Blocks that are managed by the TipManager.
	tipMetadataStorage *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *shrinkingmap.ShrinkingMap[iotago.BlockID, *TipMetadata]]

	// strongTipSet contains the blocks of the strong tip pool that have no referencing children.
	strongTipSet *randommap.RandomMap[iotago.BlockID, *TipMetadata]

	// weakTipSet contains the blocks of the weak tip pool that have no referencing children.
	weakTipSet *randommap.RandomMap[iotago.BlockID, *TipMetadata]

	// lastEvictedSlot is the last slot index that was evicted from the MemPool.
	lastEvictedSlot iotago.SlotIndex

	// evictionMutex is used to synchronize the eviction of slots.
	evictionMutex sync.RWMutex
}

// NewTipManager creates a new TipManager.
func NewTipManager(blockRetriever func(blockID iotago.BlockID) (block *blocks.Block, exists bool), opts ...options.Option[TipManager]) *TipManager {
	return options.Apply(&TipManager{
		retrieveBlock:      blockRetriever,
		tipMetadataStorage: shrinkingmap.New[iotago.SlotIndex, *shrinkingmap.ShrinkingMap[iotago.BlockID, *TipMetadata]](),
		strongTipSet:       randommap.New[iotago.BlockID, *TipMetadata](),
		weakTipSet:         randommap.New[iotago.BlockID, *TipMetadata](),
	}, opts)
}

// AddBlock adds a Block to the TipManager and returns the TipMetadata if the Block was added successfully.
func (t *TipManager) AddBlock(block *blocks.Block) tipmanager.TipMetadata {
	return t.addBlock(block)
}

func (t *TipManager) addBlock(block *blocks.Block) *TipMetadata {
	tipMetadata := NewBlockMetadata(block)
	if storage := t.metadataStorage(block.ID().Index()); storage == nil || !storage.Set(block.ID(), tipMetadata) {
		return nil
	}

	t.setupBlockMetadata(tipMetadata)

	return tipMetadata
}

// StrongTips returns the strong selectTips of the TipManager (with an optional limit).
func (t *TipManager) StrongTips(optAmount ...int) []tipmanager.TipMetadata {
	return t.selectTips(t.strongTipSet, optAmount...)
}

// WeakTips returns the weak selectTips of the TipManager (with an optional limit).
func (t *TipManager) WeakTips(optAmount ...int) []tipmanager.TipMetadata {
	return t.selectTips(t.weakTipSet, optAmount...)
}

// Evict evicts a slot from the TipManager.
func (t *TipManager) Evict(slotIndex iotago.SlotIndex) {
	if !t.markSlotAsEvicted(slotIndex) {
		return
	}

	if evictedObjects, deleted := t.tipMetadataStorage.DeleteAndReturn(slotIndex); deleted {
		evictedObjects.ForEach(func(_ iotago.BlockID, tipMetadata *TipMetadata) bool {
			tipMetadata.evicted.Trigger()

			return true
		})
	}
}

// setupBlockMetadata sets up the behavior of the given Block.
func (t *TipManager) setupBlockMetadata(tipMetadata *TipMetadata) {
	unhookMethods := []func(){
		tipMetadata.stronglyConnectedToTips.OnUpdate(func(_, isConnected bool) {
			t.forEachParentByType(tipMetadata, updateConnectedChildren(isConnected, true))
		}),

		tipMetadata.weaklyConnectedToTips.OnUpdate(func(_, isConnected bool) {
			t.forEachParentByType(tipMetadata, updateConnectedChildren(isConnected, false))
		}),

		tipMetadata.OnIsStrongTipUpdated(func(isStrongTip bool) {
			if isStrongTip {
				t.strongTipSet.Set(tipMetadata.ID(), tipMetadata)
			} else {
				t.strongTipSet.Delete(tipMetadata.ID())
			}
		}),

		tipMetadata.OnIsWeakTipUpdated(func(isWeakTip bool) {
			if isWeakTip {
				t.weakTipSet.Set(tipMetadata.Block().ID(), tipMetadata)
			} else {
				t.weakTipSet.Delete(tipMetadata.Block().ID())
			}
		}),
	}

	t.forEachParentByType(tipMetadata, map[model.ParentsType]func(*TipMetadata){
		model.StrongParentType: func(strongParent *TipMetadata) {
			unhookMethods = append(unhookMethods,
				strongParent.OnIsOrphanedUpdated(func(isOrphaned bool) {
					tipMetadata.orphanedStrongParents.Compute(lo.Cond(isOrphaned, increase, decrease))
				}),
			)
		},
	})

	tipMetadata.OnEvicted(func() {
		tipMetadata.setTipPool(tipmanager.DroppedTipPool)

		lo.Batch(unhookMethods...)()
	})
}

// forEachParentByType updates the parents of the given Block.
func (t *TipManager) forEachParentByType(tipMetadata *TipMetadata, updates map[model.ParentsType]func(*TipMetadata)) {
	if parentBlock := tipMetadata.Block(); parentBlock != nil && parentBlock.Block() != nil {
		for _, parent := range parentBlock.ParentsWithType() {
			metadataStorage := t.metadataStorage(parent.ID.Index())
			if metadataStorage == nil {
				return
			}

			parentMetadata, created := metadataStorage.GetOrCreate(parent.ID, func() *TipMetadata { return NewBlockMetadata(lo.Return1(t.retrieveBlock(parent.ID))) })
			if parentMetadata.Block() == nil {
				return
			}

			if created {
				t.setupBlockMetadata(parentMetadata)
			}

			if update, exists := updates[parent.Type]; exists {
				update(parentMetadata)
			}
		}
	}
}

// metadataStorage returns the TipMetadata storage for the given slotIndex.
func (t *TipManager) metadataStorage(slotIndex iotago.SlotIndex) (storage *shrinkingmap.ShrinkingMap[iotago.BlockID, *TipMetadata]) {
	t.evictionMutex.RLock()
	defer t.evictionMutex.RUnlock()

	if t.lastEvictedSlot >= slotIndex {
		return nil
	}

	return lo.Return1(t.tipMetadataStorage.GetOrCreate(slotIndex, lo.NoVariadic(shrinkingmap.New[iotago.BlockID, *TipMetadata])))
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

// selectTips returns the given amount of selectTips from the given tip set.
func (t *TipManager) selectTips(tipSet *randommap.RandomMap[iotago.BlockID, *TipMetadata], optAmount ...int) []tipmanager.TipMetadata {
	if len(optAmount) != 0 {
		return lo.Map(tipSet.RandomUniqueEntries(optAmount[0]), func(tip *TipMetadata) tipmanager.TipMetadata { return tip })
	}

	return lo.Map(tipSet.Values(), func(tip *TipMetadata) tipmanager.TipMetadata { return tip })
}

// updateConnectedChildren returns the update functions for the connected children counters of the parents of a Block.
func updateConnectedChildren(isConnected bool, stronglyConnected bool) (propagationRules map[model.ParentsType]func(*TipMetadata)) {
	updateFunc := lo.Cond(isConnected, increase, decrease)

	propagationRules = map[model.ParentsType]func(*TipMetadata){
		model.WeakParentType: func(parent *TipMetadata) {
			parent.weaklyConnectedChildren.Compute(updateFunc)
		},
	}

	if stronglyConnected {
		propagationRules[model.StrongParentType] = func(parent *TipMetadata) {
			parent.stronglyConnectedChildren.Compute(updateFunc)
		}
	}

	return propagationRules
}

// code contract (make sure the type implements all required methods).
var _ tipmanager.TipManager = new(TipManager)
