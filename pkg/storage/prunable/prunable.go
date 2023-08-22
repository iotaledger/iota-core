package prunable

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/epochstore"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type Prunable struct {
	apiProvider       api.Provider
	prunableSlotStore *SlotManager
	errorHandler      func(error)

	semiPermanentDBConfig database.Config
	semiPermanentDB       *database.DBInstance

	decidedUpgradeSignals *epochstore.Store[model.VersionAndHash]
	poolRewards           *epochstore.EpochKVStore
	poolStats             *epochstore.Store[*model.PoolsStats]
	committee             *epochstore.Store[*account.Accounts]
}

func New(dbConfig database.Config, apiProvider api.Provider, errorHandler func(error), opts ...options.Option[SlotManager]) *Prunable {
	dir := utils.NewDirectory(dbConfig.Directory, true)
	semiPermanentDBConfig := dbConfig.WithDirectory(dir.PathWithCreate("semipermanent"))
	semiPermanentDB := database.NewDBInstance(semiPermanentDBConfig)

	return &Prunable{
		apiProvider:       apiProvider,
		errorHandler:      errorHandler,
		prunableSlotStore: NewSlotManager(dbConfig, errorHandler, opts...),

		semiPermanentDBConfig: semiPermanentDBConfig,
		semiPermanentDB:       semiPermanentDB,
		decidedUpgradeSignals: epochstore.NewStore(kvstore.Realm{epochPrefixDecidedUpgradeSignals}, semiPermanentDB.KVStore(), pruningDelayDecidedUpgradeSignals, model.VersionAndHash.Bytes, model.VersionAndHashFromBytes),
		poolRewards:           epochstore.NewEpochKVStore(kvstore.Realm{epochPrefixPoolRewards}, semiPermanentDB.KVStore(), pruningDelayPoolRewards),
		poolStats:             epochstore.NewStore(kvstore.Realm{epochPrefixPoolStats}, semiPermanentDB.KVStore(), pruningDelayPoolStats, (*model.PoolsStats).Bytes, model.PoolsStatsFromBytes),
		committee:             epochstore.NewStore(kvstore.Realm{epochPrefixCommittee}, semiPermanentDB.KVStore(), pruningDelayCommittee, (*account.Accounts).Bytes, account.AccountsFromBytes),
	}
}

func (p *Prunable) RestoreFromDisk() (lastPrunedEpoch iotago.EpochIndex) {
	lastPrunedEpoch = p.prunableSlotStore.RestoreFromDisk()

	if err := p.decidedUpgradeSignals.RestoreLastPrunedEpoch(); err != nil {
		p.errorHandler(err)
	}
	if err := p.poolRewards.RestoreLastPrunedEpoch(); err != nil {
		p.errorHandler(err)
	}
	if err := p.poolStats.RestoreLastPrunedEpoch(); err != nil {
		p.errorHandler(err)
	}
	if err := p.committee.RestoreLastPrunedEpoch(); err != nil {
		p.errorHandler(err)
	}

	return
}

func (p *Prunable) Prune(epoch iotago.EpochIndex, defaultPruningDelay iotago.EpochIndex) error {
	// prune prunable_slot
	if err := p.prunableSlotStore.Prune(epoch); err != nil {
		return ierrors.Wrapf(err, "prune prunableSlotStore failed for epoch %d", epoch)
	}

	// prune prunable_epoch: each component has its own pruning delay.
	if err := p.decidedUpgradeSignals.Prune(epoch, defaultPruningDelay); err != nil {
		return ierrors.Wrapf(err, "prune decidedUpgradeSignals failed for epoch %d", epoch)
	}

	if err := p.poolRewards.Prune(epoch, defaultPruningDelay); err != nil {
		return ierrors.Wrapf(err, "prune poolRewards failed for epoch %d", epoch)
	}

	if err := p.poolStats.Prune(epoch, defaultPruningDelay); err != nil {
		return ierrors.Wrapf(err, "prune poolStats failed for epoch %d", epoch)
	}

	if err := p.committee.Prune(epoch, defaultPruningDelay); err != nil {
		return ierrors.Wrapf(err, "prune committee failed for epoch %d", epoch)
	}

	return nil
}

func (p *Prunable) BucketSize(epoch iotago.EpochIndex) (int64, error) {
	return p.prunableSlotStore.BucketSize(epoch)
}

func (p *Prunable) Size() int64 {
	semiSize, err := ioutils.FolderSize(p.semiPermanentDBConfig.Directory)
	if err != nil {
		p.errorHandler(ierrors.Wrapf(err, "get semiPermanentDB failed for %s", p.semiPermanentDBConfig.Directory))
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
