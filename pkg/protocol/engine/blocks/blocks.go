package blocks

import (
	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Blocks struct {
	Evict         *event.Event1[iotago.SlotIndex]
	blocks        *memstorage.IndexedStorage[iotago.SlotIndex, iotago.BlockID, *Block]
	evictionState *eviction.State
	apiProvider   iotago.APIProvider
	evictionMutex syncutils.RWMutex
}

func New(evictionState *eviction.State, apiProvider iotago.APIProvider) *Blocks {
	return &Blocks{
		Evict:         event.New1[iotago.SlotIndex](),
		blocks:        memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.BlockID, *Block](),
		evictionState: evictionState,
		apiProvider:   apiProvider,
	}
}

func (b *Blocks) EvictUntil(index iotago.SlotIndex) {
	b.Evict.Trigger(index)

	b.evictionMutex.Lock()
	defer b.evictionMutex.Unlock()

	b.blocks.Evict(index)
}

func (b *Blocks) Block(id iotago.BlockID) (block *Block, exists bool) {
	b.evictionMutex.RLock()
	defer b.evictionMutex.RUnlock()

	if commitmentID, isRootBlock := b.evictionState.RootBlockCommitmentID(id); isRootBlock {
		return NewRootBlock(id, commitmentID, b.apiProvider.APIForSlot(id.Index()).TimeProvider().SlotEndTime(id.Index())), true
	}

	storage := b.blocks.Get(id.Index(), false)
	if storage == nil {
		return nil, false
	}

	return storage.Get(id)
}

func (b *Blocks) StoreOrUpdate(data *model.Block) (storedBlock *Block, evicted, updated bool) {
	b.evictionMutex.RLock()
	defer b.evictionMutex.RUnlock()

	if evictedIndex := b.evictionState.LastEvictedSlot(); evictedIndex >= data.ID().Index() {
		return nil, true, false
	}

	storage := b.blocks.Get(data.ID().Index(), true)
	createdBlock, created := storage.GetOrCreate(data.ID(), func() *Block { return NewBlock(data) })
	if !created {
		return createdBlock, false, createdBlock.Update(data)
	}

	return createdBlock, false, false
}

func (b *Blocks) GetOrCreate(blockID iotago.BlockID, createFunc func() *Block) (block *Block, created bool) {
	b.evictionMutex.RLock()
	defer b.evictionMutex.RUnlock()

	if evictedIndex := b.evictionState.LastEvictedSlot(); evictedIndex >= blockID.Index() {
		return nil, false
	}

	storage := b.blocks.Get(blockID.Index(), true)

	return storage.GetOrCreate(blockID, createFunc)
}

func (b *Blocks) StoreBlock(block *Block) (stored bool) {
	b.evictionMutex.RLock()
	defer b.evictionMutex.RUnlock()

	if evictedIndex := b.evictionState.LastEvictedSlot(); evictedIndex >= block.ID().Index() {
		return false
	}

	storage := b.blocks.Get(block.ID().Index(), true)

	return storage.Set(block.ID(), block)
}
