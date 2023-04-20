package prunable

import (
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	blocksPrefix byte = iota
	rootBlocksPrefix
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

func (p *Prunable) Blocks(slot iotago.SlotIndex) *Blocks {
	store := p.manager.Get(slot, []byte{blocksPrefix})
	if store == nil {
		return nil
	}

	return NewBlocks(slot, store, p.api)
}

func (p *Prunable) RootBlocks(slot iotago.SlotIndex) *RootBlocks {
	store := p.manager.Get(slot, []byte{rootBlocksPrefix})
	if store == nil {
		return nil
	}

	return NewRootBlocks(slot, store)
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
