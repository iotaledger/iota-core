package tipmanagerv1

import (
	"sync"

	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/randommap"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	"github.com/iotaledger/iota-core/pkg/protocol/tipmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TipManager is a component that manages the tips of the Tangle.
type TipManager struct {
	// retrieveBlock is a function that retrieves a Block from the Tangle.
	retrieveBlock func(blockID iotago.BlockID) (block *blocks.Block, exists bool)

	// retrieveRootBlocks is a function that returns the current root blocks.
	retrieveRootBlocks func() iotago.BlockIDs

	// conflictDAG is the ConflictDAG that is used to track conflicts.
	conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, booker.BlockVotePower]

	// memPool holds information about pending transactions.
	memPool mempool.MemPool[booker.BlockVotePower]

	// tipMetadataStorage contains the TipMetadata of all Blocks that are managed by the TipManager.
	tipMetadataStorage *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *shrinkingmap.ShrinkingMap[iotago.BlockID, *TipMetadata]]

	// strongTipSet contains the blocks of the strong tip pool that have no referencing children.
	strongTipSet *randommap.RandomMap[iotago.BlockID, *TipMetadata]

	// weakTipSet contains the blocks of the weak tip pool that have no referencing children.
	weakTipSet *randommap.RandomMap[iotago.BlockID, *TipMetadata]

	// events contains all the events that are triggered by the TipManager.
	events *tipmanager.Events

	// lastEvictedSlot is the last slot index that was evicted from the MemPool.
	lastEvictedSlot iotago.SlotIndex

	// evictionMutex is used to synchronize the eviction of slots.
	evictionMutex sync.RWMutex

	// optMaxStrongParents contains the maximum number of strong parents that are allowed.
	optMaxStrongParents int

	// optMaxLikedInsteadReferences contains the maximum number of liked instead references that are allowed.
	optMaxLikedInsteadReferences int

	// optMaxLikedInsteadReferencesPerParent contains the maximum number of liked instead references that are allowed
	// per parent.
	optMaxLikedInsteadReferencesPerParent int

	// optMaxWeakReferences contains the maximum number of weak references that are allowed.
	optMaxWeakReferences int

	module.Module
}

// NewProvider creates a new TipManager provider.
func NewProvider(opts ...options.Option[TipManager]) module.Provider[*engine.Engine, tipmanager.TipManager] {
	return module.Provide(func(e *engine.Engine) tipmanager.TipManager {
		t := NewTipManager(e.Ledger.ConflictDAG(), e.BlockCache.Block, e.EvictionState.LatestRootBlocks, opts...)

		e.Events.Booker.BlockBooked.Hook(t.AddBlock, event.WithWorkerPool(e.Workers.CreatePool("AddTip", 2)))
		e.BlockCache.Evict.Hook(t.Evict)
		e.HookStopped(t.Shutdown)
		e.Events.TipManager.LinkTo(t.events)

		t.TriggerInitialized()

		return t
	})
}

// NewTipManager creates a new TipManager.
func NewTipManager(conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, booker.BlockVotePower], blockRetriever func(blockID iotago.BlockID) (block *blocks.Block, exists bool), rootBlocksRetriever func() iotago.BlockIDs, opts ...options.Option[TipManager]) *TipManager {
	return options.Apply(&TipManager{
		retrieveBlock:                blockRetriever,
		retrieveRootBlocks:           rootBlocksRetriever,
		conflictDAG:                  conflictDAG,
		tipMetadataStorage:           shrinkingmap.New[iotago.SlotIndex, *shrinkingmap.ShrinkingMap[iotago.BlockID, *TipMetadata]](),
		strongTipSet:                 randommap.New[iotago.BlockID, *TipMetadata](),
		weakTipSet:                   randommap.New[iotago.BlockID, *TipMetadata](),
		events:                       tipmanager.NewEvents(),
		optMaxStrongParents:          8,
		optMaxLikedInsteadReferences: 8,
		optMaxWeakReferences:         8,
	}, opts, func(t *TipManager) {
		t.optMaxLikedInsteadReferencesPerParent = t.optMaxLikedInsteadReferences / 2
	}, (*TipManager).TriggerConstructed)
}

// AddBlock adds a Block to the TipManager.
func (t *TipManager) AddBlock(block *blocks.Block) {
	newBlockMetadata := NewBlockMetadata(block)

	if storage := t.metadataStorage(block.ID().Index()); storage != nil && storage.Set(block.ID(), newBlockMetadata) {
		t.setupBlockMetadata(newBlockMetadata)
	}
}

// SelectTips selects the references that should be used for block issuance.
func (t *TipManager) SelectTips(amount int) (references model.ParentReferences) {
	references = make(model.ParentReferences)

	seenStrongTips := advancedset.New[iotago.BlockID]()
	selectStrongTips := func(amount int) (strongTips []*TipMetadata) {
		if amount <= 0 {
			return strongTips
		}

		for _, strongTip := range t.strongTipSet.RandomUniqueEntries(amount + seenStrongTips.Size()) {
			if seenStrongTips.Add(strongTip.Block().ID()) {
				strongTips = append(strongTips, strongTip)

				if len(strongTips) == amount {
					return strongTips
				}
			}
		}

		return strongTips
	}

	_ = t.conflictDAG.ReadConsistent(func(conflictDAG conflictdag.ReadLockedConflictDAG[iotago.TransactionID, iotago.OutputID, booker.BlockVotePower]) error {
		likedConflicts := advancedset.New[iotago.TransactionID]()

		likedInsteadReferences := func(tipMetadata *TipMetadata) (references []iotago.BlockID, updatedLikedConflicts *advancedset.AdvancedSet[iotago.TransactionID], err error) {
			necessaryReferences := make(map[iotago.TransactionID]iotago.BlockID)
			if err = conflictDAG.LikedInstead(tipMetadata.Block().ConflictIDs()).ForEach(func(likedConflictID iotago.TransactionID) error {
				transactionMetadata, exists := t.memPool.TransactionMetadata(likedConflictID)
				if !exists {
					return xerrors.Errorf("transaction required for liked instead reference (%s) not found in mem-pool", likedConflictID)
				}

				necessaryReferences[likedConflictID] = lo.First(transactionMetadata.Attachments())

				return nil
			}); err != nil {
				return nil, nil, err
			}

			references, updatedLikedConflicts = make([]iotago.BlockID, 0), likedConflicts.Clone()
			for conflictID, attachmentID := range necessaryReferences {
				if updatedLikedConflicts.Add(conflictID) {
					references = append(references, attachmentID)
				}
			}

			if len(references) > t.optMaxLikedInsteadReferencesPerParent {
				return nil, nil, xerrors.Errorf("too many liked instead references (%d) for block %s", len(references), tipMetadata.Block().ID())
			}

			return references, updatedLikedConflicts, nil
		}

		for strongTipCandidates := selectStrongTips(amount); len(strongTipCandidates) != 0; strongTipCandidates = selectStrongTips(amount - len(references[model.StrongParentType])) {
			for _, strongTip := range strongTipCandidates {
				if addedLikedInsteadReferences, updatedLikedConflicts, err := likedInsteadReferences(strongTip); err != nil {
					// TODO: LOG REASON FOR DOWNGRADE?

					strongTip.setTipPool(t.determineTipPool(strongTip, tipmanager.WeakTipPool))
				} else if len(addedLikedInsteadReferences) <= t.optMaxLikedInsteadReferences-len(references[model.ShallowLikeParentType]) {
					references[model.StrongParentType] = append(references[model.StrongParentType], strongTip.Block().ID())
					references[model.ShallowLikeParentType] = append(references[model.ShallowLikeParentType], addedLikedInsteadReferences...)

					likedConflicts = updatedLikedConflicts
				}
			}
		}

		references[model.WeakParentType] = lo.Map(t.weakTipSet.RandomUniqueEntries(t.optMaxWeakReferences), func(sourceType *TipMetadata) iotago.BlockID {
			return sourceType.Block().ID()
		})

		if len(references[model.StrongParentType]) == 0 {
			rootBlocks := t.retrieveRootBlocks()

			rootBlockCount := len(rootBlocks)
			if rootBlockCount > t.optMaxStrongParents {
				rootBlockCount = t.optMaxStrongParents
			}

			references[model.StrongParentType] = rootBlocks[:rootBlockCount]
		}

		return nil
	})

	return references
}

// StrongTipSet returns the strong tip set of the TipManager.
func (t *TipManager) StrongTipSet() []*blocks.Block {
	var tipSet []*blocks.Block
	t.strongTipSet.ForEach(func(_ iotago.BlockID, tipMetadata *TipMetadata) bool {
		tipSet = append(tipSet, tipMetadata.Block())

		return true
	})

	return tipSet
}

// WeakTipSet returns the weak tip set of the TipManager.
func (t *TipManager) WeakTipSet() (tipSet []*blocks.Block) {
	t.weakTipSet.ForEach(func(_ iotago.BlockID, tipMetadata *TipMetadata) bool {
		tipSet = append(tipSet, tipMetadata.Block())

		return true
	})

	return tipSet
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

// Events returns the events of the TipManager.
func (t *TipManager) Events() *tipmanager.Events {
	return t.events
}

// Shutdown marks the TipManager as shutdown.
func (t *TipManager) Shutdown() {
	t.TriggerStopped()
}

// setupBlockMetadata sets up the behavior of the given Block.
func (t *TipManager) setupBlockMetadata(tipMetadata *TipMetadata) {
	tipMetadata.stronglyConnectedToTips.OnUpdate(func(_, isConnected bool) {
		t.updateParents(tipMetadata, propagateConnectedChildren(isConnected, true))
	})

	tipMetadata.weaklyConnectedToTips.OnUpdate(func(_, isConnected bool) {
		t.updateParents(tipMetadata, propagateConnectedChildren(isConnected, false))
	})

	tipMetadata.OnIsStrongTipUpdated(func(isStrongTip bool) {
		if isStrongTip {
			t.strongTipSet.Set(tipMetadata.Block().ID(), tipMetadata)
		} else {
			t.strongTipSet.Delete(tipMetadata.Block().ID())
		}
	})

	tipMetadata.OnIsWeakTipUpdated(func(isWeakTip bool) {
		if isWeakTip {
			t.weakTipSet.Set(tipMetadata.Block().ID(), tipMetadata)
		} else {
			t.weakTipSet.Delete(tipMetadata.Block().ID())
		}
	})

	tipMetadata.OnEvicted(func() {
		t.strongTipSet.Delete(tipMetadata.Block().ID())
		t.weakTipSet.Delete(tipMetadata.Block().ID())
	})

	tipMetadata.setTipPool(t.determineTipPool(tipMetadata))

	t.events.BlockAdded.Trigger(tipMetadata)
}

// determineTipPool determines the initial TipPool of the given Block.
func (t *TipManager) determineTipPool(tipMetadata *TipMetadata, minPool ...tipmanager.TipPool) tipmanager.TipPool {
	if lo.First(minPool) <= tipmanager.StrongTipPool && !t.conflictDAG.AcceptanceState(tipMetadata.Block().ConflictIDs()).IsRejected() {
		return tipmanager.StrongTipPool
	}

	if lo.First(minPool) <= tipmanager.WeakTipPool && t.conflictDAG.LikedInstead(tipMetadata.Block().PayloadConflictIDs()).Size() == 0 {
		return tipmanager.WeakTipPool
	}

	return tipmanager.DroppedTipPool
}

// updateParents updates the parents of the given Block.
func (t *TipManager) updateParents(tipMetadata *TipMetadata, updates map[model.ParentsType]func(*TipMetadata)) {
	if parentBlock := tipMetadata.Block(); parentBlock != nil && parentBlock.Block() != nil {
		parentBlock.ForEachParent(func(parent model.Parent) {
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
		})
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

// WithMaxLikedInsteadReferences is an option for the TipManager that allows to configure the maximum number of liked
// instead references.
func WithMaxLikedInsteadReferences(maxLikedInsteadReferences int) options.Option[TipManager] {
	return func(tipManager *TipManager) {
		tipManager.optMaxLikedInsteadReferences = maxLikedInsteadReferences
	}
}

// WithMaxWeakReferences is an option for the TipManager that allows to configure the maximum number of weak references.
func WithMaxWeakReferences(maxWeakReferences int) options.Option[TipManager] {
	return func(tipManager *TipManager) {
		tipManager.optMaxWeakReferences = maxWeakReferences
	}
}

// code contract (make sure the type implements all required methods).
var _ tipmanager.TipManager = new(TipManager)
