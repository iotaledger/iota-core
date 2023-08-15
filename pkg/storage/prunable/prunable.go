package prunable

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/epochstore"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type Prunable struct {
	defaultPruningDelay iotago.EpochIndex
	apiProvider         api.Provider
	dbConfig            database.Config
	prunableSlotStore   *PrunableSlotManager
	errorHandler        func(error)

	semiPermanentDB       *database.DBInstance
	decidedUpgradeSignals *epochstore.Store[model.VersionAndHash]
	poolRewards           *epochstore.EpochKVStore
	poolStats             *epochstore.Store[*model.PoolsStats]
	committee             *epochstore.Store[*account.Accounts]

	lastPrunedEpoch *model.EvictionIndex[iotago.EpochIndex]
	pruningMutex    syncutils.RWMutex
}

func New(dbConfig database.Config, pruningDelay iotago.EpochIndex, apiProvider api.Provider, errorHandler func(error), opts ...options.Option[PrunableSlotManager]) *Prunable {
	semiPermanentDB := database.NewDBInstance(dbConfig)

	return &Prunable{
		dbConfig:            dbConfig,
		defaultPruningDelay: pruningDelay,
		apiProvider:         apiProvider,
		errorHandler:        errorHandler,
		prunableSlotStore:   NewPrunableSlotManager(dbConfig, errorHandler, opts...),

		lastPrunedEpoch: model.NewEvictionIndex[iotago.EpochIndex](),

		semiPermanentDB:       semiPermanentDB,
		decidedUpgradeSignals: epochstore.NewStore(kvstore.Realm{epochPrefixDecidedUpgradeSignals}, semiPermanentDB.KVStore(), pruningDelayDecidedUpgradeSignals, model.VersionAndHash.Bytes, model.VersionAndHashFromBytes),
		poolRewards:           epochstore.NewEpochKVStore(kvstore.Realm{epochPrefixPoolRewards}, semiPermanentDB.KVStore(), pruningDelayPoolRewards),
		poolStats:             epochstore.NewStore(kvstore.Realm{epochPrefixPoolStats}, semiPermanentDB.KVStore(), pruningDelayPoolStats, (*model.PoolsStats).Bytes, model.PoolsStatsFromBytes),
		committee:             epochstore.NewStore(kvstore.Realm{epochPrefixCommittee}, semiPermanentDB.KVStore(), pruningDelayCommittee, (*account.Accounts).Bytes, account.AccountsFromBytes),
	}
}

func (p *Prunable) RestoreFromDisk() {
	lastPrunedEpoch := p.prunableSlotStore.RestoreFromDisk()

	p.pruningMutex.Lock()
	p.lastPrunedEpoch.MarkEvicted(lastPrunedEpoch)
	p.pruningMutex.Unlock()
}

// IsTooOld checks if the index is in a pruned epoch.
func (p *Prunable) IsTooOld(index iotago.EpochIndex) (isTooOld bool) {
	p.pruningMutex.RLock()
	defer p.pruningMutex.RUnlock()

	return index < p.lastPrunedEpoch.NextIndex()
}

// PruneUntilSlot prunes storage slots less than and equal to the given index.
func (p *Prunable) PruneUntilSlot(index iotago.SlotIndex) {
	epoch := p.apiProvider.APIForSlot(index).TimeProvider().EpochFromSlot(index)
	p.PruneUntilEpoch(epoch)
}

func (p *Prunable) PruneUntilEpoch(epoch iotago.EpochIndex) {
	p.pruningMutex.Lock()
	defer p.pruningMutex.Unlock()

	if epoch < p.lastPrunedEpoch.NextIndex() || epoch < p.defaultPruningDelay {
		return
	}

	// prune prunable_slot
	start := p.lastPrunedEpoch.NextIndex()
	p.prunableSlotStore.PruneUntilEpoch(start, epoch-p.defaultPruningDelay)
	p.lastPrunedEpoch.MarkEvicted(epoch - p.defaultPruningDelay)

	// prune prunable_epoch
	// defaultPruningDelay is larger than the threshold
	// default should be maximum, the other thresholds should be minimum
	for currentIndex := start; currentIndex <= epoch; currentIndex++ {
		p.decidedUpgradeSignals.Prune(currentIndex)
		p.poolRewards.Prune(currentIndex)
		p.poolStats.Prune(currentIndex)
		p.committee.Prune(currentIndex)
	}
}

func (p *Prunable) Size() int64 {
	semiSize, err := ioutils.FolderSize(p.dbConfig.Directory)
	if err != nil {
		p.errorHandler(ierrors.Wrapf(err, "get semiPermanentDB failed for %s", p.dbConfig.Directory))
	}

	return p.prunableSlotStore.PrunableSlotStorageSize() + semiSize
}

func (p *Prunable) Shutdown() {
	p.prunableSlotStore.Shutdown()
	p.semiPermanentDB.Close()
}

func (p *Prunable) LastPrunedEpoch() (index iotago.EpochIndex, hasPruned bool) {
	p.pruningMutex.RLock()
	defer p.pruningMutex.RUnlock()

	return p.lastPrunedEpoch.Index()
}
