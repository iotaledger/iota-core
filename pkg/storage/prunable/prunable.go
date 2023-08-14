package prunable

import (
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/serializer/v2/byteutils"
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

func (p *Prunable) Blocks(slot iotago.SlotIndex) *Blocks {
	epoch := p.apiProvider.CurrentAPI().TimeProvider().EpochFromSlot(slot)
	store := p.manager.Get(epoch, byteutils.ConcatBytes(slot.MustBytes(), []byte{blocksPrefix}))
	if store == nil {
		return nil
	}

	return NewBlocks(slot, store, p.apiProvider)
}

func (p *Prunable) RootBlocks(slot iotago.SlotIndex) *RootBlocks {
	epoch := p.apiProvider.CurrentAPI().TimeProvider().EpochFromSlot(slot)
	store := p.manager.Get(epoch, byteutils.ConcatBytes(slot.MustBytes(), []byte{rootBlocksPrefix}))
	if store == nil {
		return nil
	}

	return NewRootBlocks(slot, store)
}

func (p *Prunable) Attestations(slot iotago.SlotIndex) kvstore.KVStore {
	epoch := p.apiProvider.CurrentAPI().TimeProvider().EpochFromSlot(slot)

	return p.manager.Get(epoch, byteutils.ConcatBytes(slot.MustBytes(), []byte{attestationsPrefix}))
}

func (p *Prunable) AccountDiffs(slot iotago.SlotIndex) *AccountDiffs {
	epoch := p.apiProvider.CurrentAPI().TimeProvider().EpochFromSlot(slot)
	store := p.manager.Get(epoch, byteutils.ConcatBytes(slot.MustBytes(), []byte{accountDiffsPrefix}))
	if store == nil {
		return nil
	}

	return NewAccountDiffs(slot, store, p.apiProvider.APIForEpoch(epoch))
}

func (p *Prunable) PerformanceFactors(slot iotago.SlotIndex) *PerformanceFactors {
	epoch := p.apiProvider.CurrentAPI().TimeProvider().EpochFromSlot(slot)
	store := p.manager.Get(epoch, byteutils.ConcatBytes(slot.MustBytes(), []byte{performanceFactorsPrefix}))
	if store == nil {
		return nil
	}

	return NewPerformanceFactors(slot, store)
}

func (p *Prunable) UpgradeSignals(slot iotago.SlotIndex) *UpgradeSignals {
	epoch := p.apiProvider.CurrentAPI().TimeProvider().EpochFromSlot(slot)
	store := p.manager.Get(epoch, byteutils.ConcatBytes(slot.MustBytes(), []byte{upgradeSignalsPrefix}))
	if store == nil {
		return nil
	}

	return NewUpgradeSignals(slot, store, p.apiProvider)
}

func (p *Prunable) Roots(slot iotago.SlotIndex) kvstore.KVStore {
	epoch := p.apiProvider.CurrentAPI().TimeProvider().EpochFromSlot(slot)
	return p.manager.Get(epoch, byteutils.ConcatBytes(slot.MustBytes(), []byte{rootsPrefix}))
}

func (p *Prunable) Retainer(slot iotago.SlotIndex) *Retainer {
	epoch := p.apiProvider.CurrentAPI().TimeProvider().EpochFromSlot(slot)
	store := p.manager.Get(epoch, byteutils.ConcatBytes(slot.MustBytes(), []byte{retainerPrefix}))
	if store == nil {
		return nil
	}

	return NewRetainer(slot, store)
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
