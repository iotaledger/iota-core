package prunable

import (
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	blocksPrefix byte = iota
	rootBlocksPrefix
	attestationsPrefix
)

type Prunable struct {
	api     iotago.API
	manager *Manager
}

func New(dbConfig database.Config, opts ...options.Option[Manager]) *Prunable {
	return &Prunable{
		manager: NewManager(dbConfig, opts...),
	}
}

func (p *Prunable) Initialize(a iotago.API) {
	p.api = a
}

func (p *Prunable) RestoreFromDisk() {
	p.manager.RestoreFromDisk()
}

func (p *Prunable) Blocks(slot iotago.SlotIndex) *Blocks {
	store := p.manager.Get(slot, kvstore.Realm{blocksPrefix})
	if store == nil {
		return nil
	}

	return NewBlocks(slot, store, p.api)
}

func (p *Prunable) RootBlocks(slot iotago.SlotIndex) *RootBlocks {
	store := p.manager.Get(slot, kvstore.Realm{rootBlocksPrefix})
	if store == nil {
		return nil
	}

	return NewRootBlocks(slot, store)
}

func (p *Prunable) Attestations(slot iotago.SlotIndex) kvstore.KVStore {
	return p.manager.Get(slot, kvstore.Realm{attestationsPrefix})
}

// PruneUntilSlot prunes storage slots less than and equal to the given index.
func (p *Prunable) PruneUntilSlot(index iotago.SlotIndex) {
	p.manager.PruneUntilSlot(index)
}

func (p *Prunable) Size() int64 {
	return p.manager.PrunableStorageSize()
}

func (p *Prunable) Shutdown() {
	p.manager.Shutdown()
}

func (p *Prunable) LastPrunedSlot() (index iotago.SlotIndex, hasPruned bool) {
	return p.manager.LastPrunedSlot()
}
