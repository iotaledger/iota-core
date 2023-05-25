package tipmanagerv1

import (
	"sync"

	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/randommap"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TipManager is a component that manages the tips of the Tangle.
type TipManager struct {
	// retrieveBlock is a function that retrieves a Block from the Tangle.
	retrieveBlock func(blockID iotago.BlockID) (block *blocks.Block, exists bool)

	// conflictDAG is the ConflictDAG that is used to track conflicts.
	conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, booker.BlockVotePower]

	memPool mempool.MemPool[booker.BlockVotePower]

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
func NewTipManager(conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, booker.BlockVotePower], blockRetriever func(blockID iotago.BlockID) (block *blocks.Block, exists bool)) *TipManager {
	return &TipManager{
		retrieveBlock:        blockRetriever,
		conflictDAG:          conflictDAG,
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

// OnBlockAdded registers a callback that is triggered when a new Block was added to the TipManager.
func (t *TipManager) OnBlockAdded(handler func(blockMetadata *BlockMetadata)) (unsubscribe func()) {
	return t.blockAdded.Hook(handler).Unhook
}

func (t *TipManager) uniqueTipSelector() func(amount int) (strongTips []*BlockMetadata) {
	seenStrongTips := advancedset.New[iotago.BlockID]()

	return func(amount int) (strongTips []*BlockMetadata) {
		if amount > 0 {
			for _, uniqueTip := range t.strongTipSet.RandomUniqueEntries(amount + seenStrongTips.Size()) {
				if seenStrongTips.Add(uniqueTip.Block.ID()) {
					if strongTips = append(strongTips, uniqueTip); len(strongTips) == amount {
						return strongTips
					}
				}
			}
		}

		return strongTips
	}
}

// SelectTips selects the references that should be used for block issuance.
func (t *TipManager) SelectTips(amount int) (references model.ParentReferences) {
	references = make(model.ParentReferences)

	seenStrongTips := advancedset.New[iotago.BlockID]()
	selectStrongTips := func(amount int) (strongTips []*BlockMetadata) {
		if amount > 0 {
			for _, strongTip := range t.strongTipSet.RandomUniqueEntries(amount + seenStrongTips.Size()) {
				if seenStrongTips.Add(strongTip.Block.ID()) {
					if strongTips = append(strongTips, strongTip); len(strongTips) == amount {
						return strongTips
					}
				}
			}
		}

		return strongTips
	}

	t.conflictDAG.ReadConsistent(func(conflictDAG conflictdag.ReadLockedConflictDAG[iotago.TransactionID, iotago.OutputID, booker.BlockVotePower]) error {
		likedConflicts := advancedset.New[iotago.TransactionID]()

		likedInsteadReferences := func(blockMetadata *BlockMetadata) (references []iotago.BlockID, updatedLikedConflicts *advancedset.AdvancedSet[iotago.TransactionID], err error) {
			necessaryReferences := make(map[iotago.TransactionID]iotago.BlockID)
			if err = conflictDAG.LikedInstead(blockMetadata.ConflictIDs()).ForEach(func(likedConflictID iotago.TransactionID) error {
				if transactionMetadata, exists := t.memPool.TransactionMetadata(likedConflictID); !exists {
					return xerrors.Errorf("transaction required for liked instead reference (%s) not found in mem-pool", likedConflictID)
				} else {
					necessaryReferences[likedConflictID] = lo.First(transactionMetadata.Attachments())
				}

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

			if len(references) > maxLikedInsteadReferencesPerParent {
				return nil, nil, xerrors.Errorf("too many liked instead references (%d) for block %s", len(references), blockMetadata.Block.ID())
			}

			return references, updatedLikedConflicts, nil
		}

		for strongTipCandidates := selectStrongTips(amount - len(references[model.StrongParentType])); len(strongTipCandidates) != 0; strongTipCandidates = selectStrongTips(amount - len(references[model.StrongParentType])) {
			for _, strongTip := range strongTipCandidates {
				if addedLikedInsteadReferences, updatedLikedConflicts, err := likedInsteadReferences(strongTip); err != nil {
					// TODO: LOG REASON FOR DOWNGRADE?

					strongTip.setTipPool(t.determineTipPool(strongTip, WeakTipPool))
				} else if len(addedLikedInsteadReferences) <= maxLikedInsteadReferences-len(references[model.ShallowLikeParentType]) {
					references[model.StrongParentType] = append(references[model.StrongParentType], strongTip.ID())
					references[model.ShallowLikeParentType] = append(references[model.ShallowLikeParentType], addedLikedInsteadReferences...)

					likedConflicts = updatedLikedConflicts
				}
			}
		}

		references[model.WeakParentType] = lo.Map(t.weakTipSet.RandomUniqueEntries(maxWeakReferences), (*BlockMetadata).ID)

		return nil
	})

	return references
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

	blockMetadata.isStrongTip.OnUpdate(func(_, isStrongTip bool) {
		if isStrongTip {
			t.strongTipSet.Set(blockMetadata.ID(), blockMetadata)
		} else {
			t.strongTipSet.Delete(blockMetadata.ID())
		}
	})

	blockMetadata.isWeakTip.OnUpdate(func(_, isWeakTip bool) {
		if isWeakTip {
			t.weakTipSet.Set(blockMetadata.ID(), blockMetadata)
		} else {
			t.weakTipSet.Delete(blockMetadata.ID())
		}
	})

	blockMetadata.OnEvicted(func() {
		t.strongTipSet.Delete(blockMetadata.ID())
		t.weakTipSet.Delete(blockMetadata.ID())
	})

	blockMetadata.setTipPool(t.determineTipPool(blockMetadata))

	t.blockAdded.Trigger(blockMetadata)
}

// determineTipPool determines the initial TipPool of the given Block.
func (t *TipManager) determineTipPool(blockMetadata *BlockMetadata, minPool ...TipPool) TipPool {
	if lo.First(minPool) <= StrongTipPool && !t.conflictDAG.AcceptanceState(blockMetadata.ConflictIDs()).IsRejected() {
		return StrongTipPool
	}

	if lo.First(minPool) <= WeakTipPool && t.conflictDAG.LikedInstead(blockMetadata.PayloadConflictIDs()).Size() == 0 {
		return WeakTipPool
	}

	return DroppedTipPool
}

// updateParents updates the parents of the given Block.
func (t *TipManager) updateParents(blockMetadata *BlockMetadata, updates map[model.ParentsType]func(*BlockMetadata)) {
	blockMetadata.ForEachParent(func(parent model.Parent) {
		metadataStorage := t.metadataStorage(parent.ID.Index())
		if metadataStorage == nil {
			return
		}

		parentMetadata, created := metadataStorage.GetOrCreate(parent.ID, func() *BlockMetadata { return NewBlockMetadata(lo.Return1(t.retrieveBlock(parent.ID))) })
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
