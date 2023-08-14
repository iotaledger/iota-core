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
	pruningDelay iotago.EpochIndex
	apiProvider  api.Provider
	manager      *Manager
	errorHandler func(error)
}

func New(dbConfig database.Config, pruningDelay iotago.EpochIndex, apiProvider api.Provider, errorHandler func(error), opts ...options.Option[Manager]) *Prunable {
	return &Prunable{
		pruningDelay: pruningDelay,
		apiProvider:  apiProvider,
		errorHandler: errorHandler,
		manager:      NewManager(dbConfig, errorHandler, opts...),
	}
}

func (p *Prunable) RestoreFromDisk() {
	p.manager.RestoreFromDisk()
}



// PruneUntilSlot prunes storage slots less than and equal to the given index.
func (p *Prunable) PruneUntilSlot(index iotago.SlotIndex) {
	epoch := p.apiProvider.CurrentAPI().TimeProvider().EpochFromSlot(index)
	if epoch < p.pruningDelay {
		return
	}

	p.manager.PruneUntilEpoch(epoch - p.pruningDelay)
}

func (p *Prunable) Size() int64 {
	return p.manager.PrunableStorageSize()
}

func (p *Prunable) Shutdown() {
	p.manager.Shutdown()
}

func (p *Prunable) LastPrunedEpoch() (index iotago.EpochIndex, hasPruned bool) {
	return p.manager.LastPrunedEpoch()
}
