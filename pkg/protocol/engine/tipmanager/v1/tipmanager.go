package tipmanagerv1

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/randommap"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TipManager is a component that manages the tips of the Tangle.
type TipManager struct {
	// retrieveBlock is a function that retrieves a Block from the Tangle.
	retrieveBlock func(blockID iotago.BlockID) (block *blocks.Block, exists bool)

	// tipMetadataStorage contains the TipMetadata of all Blocks that are managed by the TipManager.
	tipMetadataStorage *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *shrinkingmap.ShrinkingMap[iotago.BlockID, *TipMetadata]]

	// strongTipSet contains the blocks of the strong tip pool that have no referencing children.
	strongTipSet *randommap.RandomMap[iotago.BlockID, *TipMetadata]

	// weakTipSet contains the blocks of the weak tip pool that have no referencing children.
	weakTipSet *randommap.RandomMap[iotago.BlockID, *TipMetadata]

	// blockAdded is triggered when a new Block was added to the TipManager.
	blockAdded *event.Event1[tipmanager.TipMetadata]

	// lastEvictedSlot contains the last slot index that was evicted from the MemPool.
	lastEvictedSlot iotago.SlotIndex

	// evictionMutex is used to synchronize the eviction of slots.
	evictionMutex sync.RWMutex

	// Module embeds the required module.Module interface.
	module.Module
}

// NewTipManager creates a new TipManager.
func NewTipManager(blockRetriever func(blockID iotago.BlockID) (block *blocks.Block, exists bool), opts ...options.Option[TipManager]) *TipManager {
	return options.Apply(&TipManager{
		retrieveBlock:      blockRetriever,
		tipMetadataStorage: shrinkingmap.New[iotago.SlotIndex, *shrinkingmap.ShrinkingMap[iotago.BlockID, *TipMetadata]](),
		strongTipSet:       randommap.New[iotago.BlockID, *TipMetadata](),
		weakTipSet:         randommap.New[iotago.BlockID, *TipMetadata](),
		blockAdded:         event.New1[tipmanager.TipMetadata](),
	}, opts, (*TipManager).TriggerConstructed)
}

// AddBlock adds a Block to the TipManager and returns the TipMetadata if the Block was added successfully.
func (t *TipManager) AddBlock(block *blocks.Block) tipmanager.TipMetadata {
	tipMetadata := NewBlockMetadata(block)

	if storage := t.metadataStorage(block.ID().Index()); storage == nil || !storage.Set(block.ID(), tipMetadata) {
		return nil
	}

	t.setupBlockMetadata(tipMetadata)

	return tipMetadata
}

// OnBlockAdded registers a callback that is triggered whenever a new Block was added to the TipManager.
func (t *TipManager) OnBlockAdded(handler func(block tipmanager.TipMetadata)) (unsubscribe func()) {
	return t.blockAdded.Hook(handler).Unhook
}

// StrongTips returns the strong tips of the TipManager (with an optional limit).
func (t *TipManager) StrongTips(optAmount ...int) []tipmanager.TipMetadata {
	return t.selectTips(t.strongTipSet, optAmount...)
}

// WeakTips returns the weak tips of the TipManager (with an optional limit).
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
			tipMetadata.isEvicted.Trigger()

			return true
		})
	}
}

// Shutdown does nothing but is required by the module.Interface.
func (t *TipManager) Shutdown() {}

// setupBlockMetadata sets up the behavior of the given Block.
func (t *TipManager) setupBlockMetadata(tipMetadata *TipMetadata) {
	tipMetadata.OnIsStrongTipUpdated(func(isStrongTip bool) {
		if isStrongTip {
			t.strongTipSet.Set(tipMetadata.ID(), tipMetadata)
		} else {
			t.strongTipSet.Delete(tipMetadata.ID())
		}
	})

	tipMetadata.OnIsWeakTipUpdated(func(isWeakTip bool) {
		if isWeakTip {
			t.weakTipSet.Set(tipMetadata.Block().ID(), tipMetadata)
		} else {
			t.weakTipSet.Delete(tipMetadata.Block().ID())
		}
	})

	t.forEachParentByType(tipMetadata.Block(), func(parentType model.ParentsType, parentMetadata *TipMetadata) {
		if parentType == model.StrongParentType {
			tipMetadata.setupStrongParent(parentMetadata)
		} else {
			tipMetadata.setupWeakParent(parentMetadata)
		}
	})

	t.blockAdded.Trigger(tipMetadata)
}

// forEachParentByType iterates through the parents of the given block and calls the consumer for each parent.
func (t *TipManager) forEachParentByType(block *blocks.Block, consumer func(parentType model.ParentsType, parentMetadata *TipMetadata)) {
	if block == nil || block.Block() == nil {
		return
	}

	for _, parent := range block.ParentsWithType() {
		if metadataStorage := t.metadataStorage(parent.ID.Index()); metadataStorage != nil {
			if parentMetadata, created := metadataStorage.GetOrCreate(parent.ID, func() *TipMetadata { return NewBlockMetadata(lo.Return1(t.retrieveBlock(parent.ID))) }); parentMetadata.Block() != nil {
				consumer(parent.Type, parentMetadata)

				if created {
					t.setupBlockMetadata(parentMetadata)
				}
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

// selectTips returns the given amount of tips from the given tip set.
func (t *TipManager) selectTips(tipSet *randommap.RandomMap[iotago.BlockID, *TipMetadata], optAmount ...int) []tipmanager.TipMetadata {
	if len(optAmount) != 0 {
		return lo.Map(tipSet.RandomUniqueEntries(optAmount[0]), func(tip *TipMetadata) tipmanager.TipMetadata { return tip })
	}

	return lo.Map(tipSet.Values(), func(tip *TipMetadata) tipmanager.TipMetadata { return tip })
}

// code contract (make sure the type implements all required methods).
var _ tipmanager.TipManager = new(TipManager)
