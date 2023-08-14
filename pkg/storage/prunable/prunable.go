package prunable

import (
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

const (
	blocksPrefix byte = iota
	rootBlocksPrefix
	attestationsPrefix
	accountDiffsPrefix
	performanceFactorsPrefix
	upgradeSignalsPrefix
	rootsPrefix
	retainerPrefix
)

type Prunable struct {
	pruningDelay iotago.SlotIndex
	apiProvider  api.Provider
	manager      *Manager
	errorHandler func(error)
}

func New(dbConfig database.Config, pruningDelay iotago.SlotIndex, errorHandler func(error), opts ...options.Option[Manager]) *Prunable {
	return &Prunable{
		pruningDelay: pruningDelay,
		errorHandler: errorHandler,
		manager:      NewManager(dbConfig, errorHandler, opts...),
	}
}

func (p *Prunable) Initialize(apiProvider api.Provider) {
	p.apiProvider = apiProvider
}

func (p *Prunable) RestoreFromDisk() {
	p.manager.RestoreFromDisk()
}

// PruneUntilSlot prunes storage slots less than and equal to the given index.
func (p *Prunable) PruneUntilSlot(index iotago.SlotIndex) {
	if index < p.pruningDelay {
		return
	}

	p.manager.PruneUntilSlot(index - p.pruningDelay)
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
