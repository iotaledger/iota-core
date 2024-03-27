package tipmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/randommap"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TipManager is a component that manages the tips of the Tangle.
type TipManager struct {
	// retrieveBlock is a function that retrieves a Block from the Tangle.
	retrieveBlock func(blockID iotago.BlockID) (block *blocks.Block, exists bool)

	// retrieveCommitteeInSlot is a function that retrieves the committee in a given slot.
	retrieveCommitteeInSlot func(slot iotago.SlotIndex) (*account.SeatedAccounts, bool)

	// tipMetadataStorage contains the TipMetadata of all Blocks that are managed by the TipManager.
	tipMetadataStorage *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *shrinkingmap.ShrinkingMap[iotago.BlockID, *TipMetadata]]

	// latestValidationBlocks contains a Variable for each validator that stores the latest validation block.
	latestValidationBlocks *shrinkingmap.ShrinkingMap[account.SeatIndex, reactive.Variable[*TipMetadata]]

	// validationTipSet contains the subset of blocks from the strong tip set that reference the latest validation block.
	validationTipSet *randommap.RandomMap[iotago.BlockID, *TipMetadata]

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
func New(
	subModule module.Module,
	blockRetriever func(blockID iotago.BlockID) (block *blocks.Block, exists bool),
	retrieveCommitteeInSlot func(slot iotago.SlotIndex) (*account.SeatedAccounts, bool),
) *TipManager {
	return options.Apply(&TipManager{
		Module:                  subModule,
		retrieveBlock:           blockRetriever,
		retrieveCommitteeInSlot: retrieveCommitteeInSlot,
		tipMetadataStorage:      shrinkingmap.New[iotago.SlotIndex, *shrinkingmap.ShrinkingMap[iotago.BlockID, *TipMetadata]](),
		latestValidationBlocks:  shrinkingmap.New[account.SeatIndex, reactive.Variable[*TipMetadata]](),
		validationTipSet:        randommap.New[iotago.BlockID, *TipMetadata](),
		strongTipSet:            randommap.New[iotago.BlockID, *TipMetadata](),
		weakTipSet:              randommap.New[iotago.BlockID, *TipMetadata](),
		blockAdded:              event.New1[tipmanager.TipMetadata](),
	}, nil, func(t *TipManager) {
		t.initLogging()

		t.ShutdownEvent().OnTrigger(func() {
			t.StoppedEvent().Trigger()
		})

		t.ConstructedEvent().Trigger()
	})
}

// AddBlock adds a Block to the TipManager and returns the TipMetadata if the Block was added successfully.
func (t *TipManager) AddBlock(block *blocks.Block) tipmanager.TipMetadata {
	storage := t.metadataStorage(block.ID().Slot())
	if storage == nil {
		return nil
	}

	tipMetadata, created := storage.GetOrCreate(block.ID(), t.tipMetadataFactory(block))
	if created {
		t.trackTipMetadata(tipMetadata)
	}

	return tipMetadata
}

// OnBlockAdded registers a callback that is triggered whenever a new Block was added to the TipManager.
func (t *TipManager) OnBlockAdded(handler func(block tipmanager.TipMetadata)) (unsubscribe func()) {
	return t.blockAdded.Hook(handler).Unhook
}

// AddSeat adds a validator to the tracking of the TipManager.
func (t *TipManager) AddSeat(seat account.SeatIndex) {
	t.latestValidationBlocks.GetOrCreate(seat, func() reactive.Variable[*TipMetadata] {
		return reactive.NewVariable[*TipMetadata]()
	})
}

// RemoveSeat removes a validator from the tracking of the TipManager.
func (t *TipManager) RemoveSeat(seat account.SeatIndex) {
	latestValidationBlock, removed := t.latestValidationBlocks.DeleteAndReturn(seat)
	if removed {
		latestValidationBlock.Set(nil)
	}
}

func (t *TipManager) ValidationTips(optAmount ...int) []tipmanager.TipMetadata {
	return t.selectTips(t.validationTipSet, optAmount...)
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

	t.latestValidationBlocks.Clear()

	// TODO: introduce a method to clear the randommap
	lo.ForEach(t.validationTipSet.Keys(), func(id iotago.BlockID) { t.validationTipSet.Delete(id) })
	lo.ForEach(t.strongTipSet.Keys(), func(id iotago.BlockID) { t.strongTipSet.Delete(id) })
	lo.ForEach(t.weakTipSet.Keys(), func(id iotago.BlockID) { t.weakTipSet.Delete(id) })
}

// initLogging initializes the logging of the TipManager.
func (t *TipManager) initLogging() {
	logLevel := log.LevelTrace

	t.blockAdded.Hook(func(metadata tipmanager.TipMetadata) {
		t.Log("block added", logLevel, "blockID", metadata.ID())
	})
}

// tipMetadataFactory creates a function that can be called to create a new TipMetadata instance for the given Block.
func (t *TipManager) tipMetadataFactory(block *blocks.Block) func() *TipMetadata {
	return func() *TipMetadata {
		return NewTipMetadata(t.NewChildLogger(block.ID().String()), block)
	}
}

// trackTipMetadata sets up the tracking of the given TipMetadata in the TipManager.
func (t *TipManager) trackTipMetadata(tipMetadata *TipMetadata) {
	t.blockAdded.Trigger(tipMetadata)

	tipMetadata.isStrongTipPoolMember.WithNonEmptyValue(func(_ bool) func() {
		return t.trackLatestValidationBlock(tipMetadata)
	})

	tipMetadata.isValidationTip.OnUpdate(func(_ bool, isValidationTip bool) {
		if isValidationTip {
			t.validationTipSet.Set(tipMetadata.ID(), tipMetadata)
		} else {
			t.validationTipSet.Delete(tipMetadata.ID())
		}
	})

	tipMetadata.isStrongTip.OnUpdate(func(_ bool, isStrongTip bool) {
		if isStrongTip {
			t.strongTipSet.Set(tipMetadata.ID(), tipMetadata)
		} else {
			t.strongTipSet.Delete(tipMetadata.ID())
		}
	})

	tipMetadata.isWeakTip.OnUpdate(func(_ bool, isWeakTip bool) {
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
}

// trackLatestValidationBlock tracks the latest validator block and takes care of marking the corresponding TipMetadata.
func (t *TipManager) trackLatestValidationBlock(tipMetadata *TipMetadata) (teardown func()) {
	if _, isValidationBlock := tipMetadata.Block().ValidationBlock(); !isValidationBlock {
		return nil
	}

	committee, exists := t.retrieveCommitteeInSlot(tipMetadata.Block().ID().Slot())
	if !exists {
		return nil
	}

	seat, exists := committee.GetSeat(tipMetadata.Block().ProtocolBlock().Header.IssuerID)
	if !exists {
		return nil
	}

	// We only track the validation blocks of validators that are tracked by the TipManager (via AddSeat).
	latestValidationBlock, exists := t.latestValidationBlocks.Get(seat)
	if !exists {
		return nil
	}

	if !tipMetadata.registerAsLatestValidationBlock(latestValidationBlock) {
		return nil
	}

	// reset the latest validator block to nil if we are still the latest one during teardown
	return func() {
		latestValidationBlock.Compute(func(latestValidationBlock *TipMetadata) *TipMetadata {
			return lo.Cond(latestValidationBlock == tipMetadata, nil, latestValidationBlock)
		})
	}
}

// forEachParentByType iterates through the parents of the given block and calls the consumer for each parent.
func (t *TipManager) forEachParentByType(block *blocks.Block, consumer func(parentType iotago.ParentsType, parentMetadata *TipMetadata)) {
	for _, parent := range block.ParentsWithType() {
		if metadataStorage := t.metadataStorage(parent.ID.Slot()); metadataStorage != nil {
			// make sure we don't add root blocks back to the tips.
			parentBlock, exists := t.retrieveBlock(parent.ID)
			if !exists || parentBlock.IsRootBlock() {
				continue
			}

			parentMetadata, created := metadataStorage.GetOrCreate(parent.ID, t.tipMetadataFactory(parentBlock))
			if created {
				t.trackTipMetadata(parentMetadata)
			}

			consumer(parent.Type, parentMetadata)
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
