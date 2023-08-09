package pruningmanager

import (
	"sync"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/pruning"
	"github.com/iotaledger/iota-core/pkg/storage"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

// threshold from opts

// block: prunable              |     slot finalized
// performance factors: prunable|     1 epoch
// attestations: prunable       |     slot finalized
// account diffs: prunable      |     slot finalized
// roots: prunable              |     slot finalized
// retainer: prunable           |     1 epoch?
// protocol upgrades: prunable  |     7 epochs

// pool stats: move to prunable        |     1 year
// rewards: move to prunable           |     1 year

// How to trigger pruning? hook to a certain event?
//  * time executor?
//  * check everytime when slotFinalized event is triggered?
//  * in hornet, pruning is triggered if snapshot is taken.

// methods to prune different categories in the same bucket/slot
// remove PruneUntilSlot? if it's misused, all data are removed.

// pruning api:
// * index: The pruning target slot index.
// * depth: The pruning depth. pruningTarget = finalizedSlot - depth
// * targetDatabaseSize: The target size of the database.

type Manager struct {
	events *pruning.Events

	apiProvider  api.Provider
	db           *storage.Storage
	errorHandler func(error)

	isPruning  bool
	statusLock sync.RWMutex

	optPruningSizeTargetSizeBytes int64
	// unit of threshold: epoch
	optPerformanceFactorsPruningThreshold uint32
	optProtocolUpgradesPruningThreshold   uint32
	optRewardsPruningThreshold            uint32
	optPoolStatsPruningThreshold          uint32
	optRetainerPruningThreshold           uint32

	module.Module
}

func NewProvider(opts ...options.Option[Manager]) module.Provider[*engine.Engine, pruning.Pruning] {
	return module.Provide(func(e *engine.Engine) pruning.Pruning {
		p := New(e.Storage, e, e.ErrorHandler("pruningManager"), opts...)

		asyncOpt := event.WithWorkerPool(e.Workers.CreatePool("PruningManager", 1))

		e.Events.SlotGadget.SlotFinalized.Hook(func(slotIndex iotago.SlotIndex) {
			// This is wrong, this prune the entire slot
			p.db.Prunable.PruneUntilSlot(slotIndex)
		}, asyncOpt)

		p.TriggerInitialized()

		return p
	})
}

func New(db *storage.Storage, api api.Provider, errHandler func(error), opts ...options.Option[Manager]) *Manager {
	return options.Apply(&Manager{
		events:                                pruning.NewEvents(),
		apiProvider:                           api,
		db:                                    db,
		errorHandler:                          errHandler,
		optPerformanceFactorsPruningThreshold: 1,
		optProtocolUpgradesPruningThreshold:   7,
		optRetainerPruningThreshold:           1,
		optRewardsPruningThreshold:            2990080, // 1 year
		optPoolStatsPruningThreshold:          2990080, // 1 year
	}, opts, func(m *Manager) {

	})
}

func (p *Manager) setIsPruning(value bool) {
	p.statusLock.Lock()
	p.isPruning = value
	p.statusLock.Unlock()
}

func (p *Manager) IsPruning() bool {
	p.statusLock.RLock()
	defer p.statusLock.RUnlock()

	return p.isPruning
}

func (p *Manager) PruneDatabaseBySize(targetSizeBytes ...int64) error {
	// what is the minimum size/data we need to keep to run a node?
	// only prune prunable? or also permanent?

	// targetDatabaseSizeBytes := p.optPruningSizeTargetSizeBytes
	// if len(targetSizeBytes) > 0 {
	// 	targetDatabaseSizeBytes = targetSizeBytes[0]
	// }

	return nil
}

func (p *Manager) PruneDatabaseByDepth(depth iotago.SlotIndex) error { return nil }
func (p *Manager) PruneDatabaseByIndex(index iotago.SlotIndex) error { return nil }

func (t *Manager) Shutdown() {
	t.TriggerStopped()
}

func WithPruningSizeTargetSizeBytes(pruningSizeTargetSizeBytes int64) options.Option[Manager] {
	return func(p *Manager) {
		p.optPruningSizeTargetSizeBytes = pruningSizeTargetSizeBytes
	}
}
