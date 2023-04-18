package prunable

import (
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	blocksPrefix byte = iota
	rootBlocksPrefix
	attestationsPrefix
	ledgerStateDiffsPrefix
)

type Prunable struct {
	manager *Manager

	Blocks           *Blocks
	RootBlocks       *RootBlocks
	Attestations     func(index iotago.SlotIndex) kvstore.KVStore
	LedgerStateDiffs func(index iotago.SlotIndex) kvstore.KVStore
}

func New(dbConfig database.Config, opts ...options.Option[Manager]) *Prunable {
	manager := NewManager(dbConfig, opts...)

	return &Prunable{
		manager:          manager,
		Blocks:           NewBlocks(manager, blocksPrefix),
		RootBlocks:       NewRootBlocks(manager, rootBlocksPrefix),
		Attestations:     lo.Bind([]byte{attestationsPrefix}, manager.Get),
		LedgerStateDiffs: lo.Bind([]byte{ledgerStateDiffsPrefix}, manager.Get),
	}
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
