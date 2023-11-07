package tipmanagerv1

import (
	"fmt"

	"github.com/iotaledger/hive.go/ds/randommap"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/syncutils"
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
	evictionMutex syncutils.RWMutex

	// Module embeds the required module.Module interface.
	module.Module
}

// New creates a new TipManager.
func New(blockRetriever func(blockID iotago.BlockID) (block *blocks.Block, exists bool)) *TipManager {
	t := &TipManager{
		retrieveBlock:      blockRetriever,
		tipMetadataStorage: shrinkingmap.New[iotago.SlotIndex, *shrinkingmap.ShrinkingMap[iotago.BlockID, *TipMetadata]](),
		strongTipSet:       randommap.New[iotago.BlockID, *TipMetadata](),
		weakTipSet:         randommap.New[iotago.BlockID, *TipMetadata](),
		blockAdded:         event.New1[tipmanager.TipMetadata](),
	}

	t.TriggerConstructed()
	t.TriggerInitialized()

	return t
}

// AddBlock adds a Block to the TipManager and returns the TipMetadata if the Block was added successfully.
func (t *TipManager) AddBlock(block *blocks.Block) tipmanager.TipMetadata {
	storage := t.metadataStorage(block.ID().Slot())
	if storage == nil {
		return nil
	}

	tipMetadata, created := storage.GetOrCreate(block.ID(), func() *TipMetadata {
		return NewBlockMetadata(block)
	})

	if created {
		t.setupBlockMetadata(tipMetadata)
	}

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
func (t *TipManager) Evict(slot iotago.SlotIndex) {
	if !t.markSlotAsEvicted(slot) {
		return
	}

	if evictedObjects, deleted := t.tipMetadataStorage.DeleteAndReturn(slot); deleted {
		evictedObjects.ForEach(func(_ iotago.BlockID, tipMetadata *TipMetadata) bool {
			tipMetadata.evicted.Trigger()

			return true
		})
	}
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (t *TipManager) Reset() {
	t.evictionMutex.Lock()
	defer t.evictionMutex.Unlock()

	t.tipMetadataStorage.Clear()
	lo.ForEach(t.strongTipSet.Keys(), func(id iotago.BlockID) { t.strongTipSet.Delete(id) })
	lo.ForEach(t.weakTipSet.Keys(), func(id iotago.BlockID) { t.strongTipSet.Delete(id) })
}

// Shutdown marks the TipManager as shutdown.
func (t *TipManager) Shutdown() {
	t.TriggerShutdown()
	t.TriggerStopped()
}

// setupBlockMetadata sets up the behavior of the given Block.
func (t *TipManager) setupBlockMetadata(tipMetadata *TipMetadata) {
	tipMetadata.isStrongTip.OnUpdate(func(_, isStrongTip bool) {
		if isStrongTip {
			t.strongTipSet.Set(tipMetadata.ID(), tipMetadata)
		} else {
			t.strongTipSet.Delete(tipMetadata.ID())
		}
	})

	tipMetadata.isWeakTip.OnUpdate(func(_, isWeakTip bool) {
		if isWeakTip {
			t.weakTipSet.Set(tipMetadata.Block().ID(), tipMetadata)
		} else {
			t.weakTipSet.Delete(tipMetadata.Block().ID())
		}
	})

	t.forEachParentByType(tipMetadata.Block(), func(parentType iotago.ParentsType, parentMetadata *TipMetadata) {
		if parentType == iotago.StrongParentType {
			tipMetadata.connectStrongParent(parentMetadata)
		} else {
			tipMetadata.connectWeakParent(parentMetadata)
		}
	})

	t.blockAdded.Trigger(tipMetadata)
}

// forEachParentByType iterates through the parents of the given block and calls the consumer for each parent.
func (t *TipManager) forEachParentByType(block *blocks.Block, consumer func(parentType iotago.ParentsType, parentMetadata *TipMetadata)) {
	for _, parent := range block.ParentsWithType() {
		if metadataStorage := t.metadataStorage(parent.ID.Slot()); metadataStorage != nil {
			// Make sure we don't add root blocks back to the tips.
			parentBlock, exists := t.retrieveBlock(parent.ID)

			if !exists || parentBlock.IsRootBlock() {
				continue
			}

			if parentBlock.ModelBlock() == nil {
				fmt.Printf(">> parentBlock exists, but parentBlock.ProtocolBlock() == nil\n ParentBlock: %s\n Block: %s\n", parentBlock.String(), block.String())
			}

			parentMetadata, created := metadataStorage.GetOrCreate(parent.ID, func() *TipMetadata { return NewBlockMetadata(parentBlock) })
			consumer(parent.Type, parentMetadata)

			if created {
				t.setupBlockMetadata(parentMetadata)
			}

		}
	}
}

// metadataStorage returns the TipMetadata storage for the given slotIndex.
func (t *TipManager) metadataStorage(slot iotago.SlotIndex) (storage *shrinkingmap.ShrinkingMap[iotago.BlockID, *TipMetadata]) {
	t.evictionMutex.RLock()
	defer t.evictionMutex.RUnlock()

	if t.lastEvictedSlot >= slot {
		return nil
	}

	return lo.Return1(t.tipMetadataStorage.GetOrCreate(slot, lo.NoVariadic(shrinkingmap.New[iotago.BlockID, *TipMetadata])))
}

// markSlotAsEvicted marks the given slotIndex as evicted.
func (t *TipManager) markSlotAsEvicted(slot iotago.SlotIndex) (success bool) {
	t.evictionMutex.Lock()
	defer t.evictionMutex.Unlock()

	if success = t.lastEvictedSlot < slot; success {
		t.lastEvictedSlot = slot
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
