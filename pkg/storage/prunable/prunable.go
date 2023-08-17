package prunable

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	"github.com/iotaledger/hive.go/runtime/options"
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
}

func New(dbConfig database.Config, pruningDelay iotago.EpochIndex, apiProvider api.Provider, errorHandler func(error), opts ...options.Option[PrunableSlotManager]) *Prunable {
	semiPermanentDB := database.NewDBInstance(dbConfig)

	return &Prunable{
		dbConfig:            dbConfig,
		defaultPruningDelay: pruningDelay,
		apiProvider:         apiProvider,
		errorHandler:        errorHandler,
		prunableSlotStore:   NewPrunableSlotManager(dbConfig, errorHandler, opts...),

		semiPermanentDB:       semiPermanentDB,
		decidedUpgradeSignals: epochstore.NewStore(kvstore.Realm{epochPrefixDecidedUpgradeSignals}, semiPermanentDB.KVStore(), lo.Max(pruningDelayDecidedUpgradeSignals, pruningDelay), model.VersionAndHash.Bytes, model.VersionAndHashFromBytes),
		poolRewards:           epochstore.NewEpochKVStore(kvstore.Realm{epochPrefixPoolRewards}, semiPermanentDB.KVStore(), lo.Max(pruningDelayPoolRewards, pruningDelay)),
		poolStats:             epochstore.NewStore(kvstore.Realm{epochPrefixPoolStats}, semiPermanentDB.KVStore(), lo.Max(pruningDelayPoolStats, pruningDelay), (*model.PoolsStats).Bytes, model.PoolsStatsFromBytes),
		committee:             epochstore.NewStore(kvstore.Realm{epochPrefixCommittee}, semiPermanentDB.KVStore(), lo.Max(pruningDelayCommittee, pruningDelay), (*account.Accounts).Bytes, account.AccountsFromBytes),
	}
}

func (p *Prunable) RestoreFromDisk() {
	p.prunableSlotStore.RestoreFromDisk()

	// TODO: what is the last pruned epoch for semiPermanentDB? -> set based on latest pruning epoch of buckets
}

func (p *Prunable) PruneUntilEpoch(epoch iotago.EpochIndex) {
	// No need to prune.
	if epoch < p.defaultPruningDelay {
		return
	}

	// prune prunable_slot
	p.prunableSlotStore.PruneUntilEpoch(epoch - p.defaultPruningDelay)

	// prune prunable_epoch: each component has its own pruning delay based on max(individualPruningDelay, defaultPruningDelay)
	if err := p.decidedUpgradeSignals.PruneUntilEpoch(epoch); err != nil {
		p.errorHandler(err)
	}

	if err := p.poolRewards.PruneUntilEpoch(epoch); err != nil {
		p.errorHandler(err)
	}

	if err := p.poolStats.PruneUntilEpoch(epoch); err != nil {
		p.errorHandler(err)
	}

	if err := p.committee.PruneUntilEpoch(epoch); err != nil {
		p.errorHandler(err)
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

func (p *Prunable) Flush() {
	if err := p.prunableSlotStore.Flush(); err != nil {
		p.errorHandler(err)
	}
	if err := p.semiPermanentDB.KVStore().Flush(); err != nil {
		p.errorHandler(err)
	}
}

func (p *Prunable) LastPrunedEpoch() (index iotago.EpochIndex, hasPruned bool) {
	return p.prunableSlotStore.LastPrunedEpoch()
}
