package blocks

import (
	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Blocks struct {
	blocks        *memstorage.IndexedStorage[iotago.SlotIndex, iotago.BlockID, *Block]
	evictionState *eviction.State
	apiProvider   iotago.APIProvider
	evictionMutex syncutils.RWMutex
}

func New(evictionState *eviction.State, apiProvider iotago.APIProvider) *Blocks {
	return &Blocks{
		blocks:        memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.BlockID, *Block](),
		evictionState: evictionState,
		apiProvider:   apiProvider,
	}
}

func (b *Blocks) Evict(slot iotago.SlotIndex) {
	b.evictionMutex.Lock()
	defer b.evictionMutex.Unlock()

	b.blocks.Evict(slot)
}

func (b *Blocks) Block(id iotago.BlockID) (block *Block, exists bool) {
	b.evictionMutex.RLock()
	defer b.evictionMutex.RUnlock()

	if storage := b.blocks.Get(id.Slot()); storage != nil {
		if block, exists = storage.Get(id); exists {
			return block, true
		}
	}

	if _, isInRange := b.evictionState.BelowOrInActiveRootBlockRange(id); isInRange {
		if commitmentID, isRootBlock := b.evictionState.RootBlockCommitmentID(id); isRootBlock {
			return NewRootBlock(id, commitmentID, b.apiProvider.APIForSlot(id.Slot()).TimeProvider().SlotEndTime(id.Slot())), true
		}
	}

	return nil, false
}

func (b *Blocks) StoreOrUpdate(modelBlock *model.Block) (storedBlock *Block, evicted bool, updated bool) {
	b.evictionMutex.RLock()
	defer b.evictionMutex.RUnlock()

	if evictedIndex := b.evictionState.LastEvictedSlot(); evictedIndex >= modelBlock.ID().Slot() {
		return nil, true, false
	}

	storage := b.blocks.Get(modelBlock.ID().Slot(), true)
	createdBlock, created := storage.GetOrCreate(modelBlock.ID(), func() *Block { return NewBlock(modelBlock) })
	if !created {
		return createdBlock, false, createdBlock.Update(modelBlock)
	}

	return createdBlock, false, false
}

func (b *Blocks) GetOrCreate(blockID iotago.BlockID, createFunc func() *Block) (block *Block, created bool) {
	b.evictionMutex.RLock()
	defer b.evictionMutex.RUnlock()

	if evictedIndex := b.evictionState.LastEvictedSlot(); evictedIndex >= blockID.Slot() {
		return nil, false
	}

	storage := b.blocks.Get(blockID.Slot(), true)

	return storage.GetOrCreate(blockID, createFunc)
}

func (b *Blocks) StoreBlock(block *Block) (stored bool) {
	b.evictionMutex.RLock()
	defer b.evictionMutex.RUnlock()

	if evictedIndex := b.evictionState.LastEvictedSlot(); evictedIndex >= block.ID().Slot() {
		return false
	}

	storage := b.blocks.Get(block.ID().Slot(), true)

	return storage.Set(block.ID(), block)
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (b *Blocks) Reset() {
	b.blocks.Clear()
}
