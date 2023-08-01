package prunable

import (
	"github.com/iotaledger/hive.go/kvstore"
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

const (
	RootsKey byte = iota
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

func (p *Prunable) Blocks(slot iotago.SlotIndex) *Blocks {
	store := p.manager.Get(slot, kvstore.Realm{blocksPrefix})
	if store == nil {
		return nil
	}

	return NewBlocks(slot, store, p.apiProvider)
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

func (p *Prunable) AccountDiffs(slot iotago.SlotIndex) *AccountDiffs {
	store := p.manager.Get(slot, kvstore.Realm{accountDiffsPrefix})
	if store == nil {
		return nil
	}

	return NewAccountDiffs(slot, store, p.apiProvider.APIForSlot(slot))
}

func (p *Prunable) PerformanceFactors(slot iotago.SlotIndex) *PerformanceFactors {
	// TODO: make sure that the minimum pruning delay for this is at least 1 epoch, otherwise we won't be able to calculate the reward pools
	store := p.manager.Get(slot, kvstore.Realm{performanceFactorsPrefix})
	if store == nil {
		return nil
	}

	return NewPerformanceFactors(slot, store)
}

func (p *Prunable) UpgradeSignals(slot iotago.SlotIndex) *UpgradeSignals {
	// TODO: make sure that the minimum pruning delay for this is at least 1 epoch, otherwise we won't be able to properly determine the upgrade signals.
	store := p.manager.Get(slot, kvstore.Realm{upgradeSignalsPrefix})
	if store == nil {
		return nil
	}

	return NewUpgradeSignals(slot, store, p.apiProvider)
}

func (p *Prunable) Roots(slot iotago.SlotIndex) kvstore.KVStore {
	return p.manager.Get(slot, kvstore.Realm{rootsPrefix})
}

func (p *Prunable) Retainer(slot iotago.SlotIndex) *Retainer {
	store := p.manager.Get(slot, kvstore.Realm{retainerPrefix})
	if store == nil {
		return nil
	}

	return NewRetainer(slot, store)
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
