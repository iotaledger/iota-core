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
	"github.com/iotaledger/iota-core/pkg/storage/utils"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

var (
	ErrEpochPruned = ierrors.New("epoch pruned")
)

type Prunable struct {
	defaultPruningDelay iotago.EpochIndex
	apiProvider         api.Provider
	prunableSlotStore   *PrunableSlotManager
	errorHandler        func(error)

	semiPermanentDBConfig database.Config
	semiPermanentDB       *database.DBInstance

	decidedUpgradeSignals *epochstore.Store[model.VersionAndHash]
	poolRewards           *epochstore.EpochKVStore
	poolStats             *epochstore.Store[*model.PoolsStats]
	committee             *epochstore.Store[*account.Accounts]
}

func New(dbConfig database.Config, pruningDelay iotago.EpochIndex, apiProvider api.Provider, errorHandler func(error), opts ...options.Option[PrunableSlotManager]) *Prunable {
	dir := utils.NewDirectory(dbConfig.Directory, true)
	semiPermanentDBConfig := dbConfig.WithDirectory(dir.PathWithCreate("semipermanent"))
	semiPermanentDB := database.NewDBInstance(semiPermanentDBConfig)

	return &Prunable{
		defaultPruningDelay: pruningDelay,
		apiProvider:         apiProvider,
		errorHandler:        errorHandler,
		prunableSlotStore:   NewPrunableSlotManager(dbConfig, errorHandler, opts...),

		semiPermanentDBConfig: semiPermanentDBConfig,
		semiPermanentDB:       semiPermanentDB,
		decidedUpgradeSignals: epochstore.NewStore(kvstore.Realm{epochPrefixDecidedUpgradeSignals}, semiPermanentDB.KVStore(), lo.Max(pruningDelayDecidedUpgradeSignals, pruningDelay), model.VersionAndHash.Bytes, model.VersionAndHashFromBytes),
		poolRewards:           epochstore.NewEpochKVStore(kvstore.Realm{epochPrefixPoolRewards}, semiPermanentDB.KVStore(), lo.Max(pruningDelayPoolRewards, pruningDelay)),
		poolStats:             epochstore.NewStore(kvstore.Realm{epochPrefixPoolStats}, semiPermanentDB.KVStore(), lo.Max(pruningDelayPoolStats, pruningDelay), (*model.PoolsStats).Bytes, model.PoolsStatsFromBytes),
		committee:             epochstore.NewStore(kvstore.Realm{epochPrefixCommittee}, semiPermanentDB.KVStore(), lo.Max(pruningDelayCommittee, pruningDelay), (*account.Accounts).Bytes, account.AccountsFromBytes),
	}
}

func (p *Prunable) RestoreFromDisk() {
	p.prunableSlotStore.RestoreFromDisk()

	// set lastPrunedEpoch based on latest pruning epoch of buckets
	lastPrunedEpoch := lo.Return1(p.prunableSlotStore.LastPrunedEpoch())
	p.decidedUpgradeSignals.RestoreLastPrunedEpoch(lastPrunedEpoch)
	p.poolRewards.RestoreLastPrunedEpoch(lastPrunedEpoch)
	p.poolStats.RestoreLastPrunedEpoch(lastPrunedEpoch)
	p.committee.RestoreLastPrunedEpoch(lastPrunedEpoch)
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

func (p *Prunable) LastPrunedEpoch() (index iotago.EpochIndex, hasPruned bool) {
	return p.prunableSlotStore.LastPrunedEpoch()
}

func (p *Prunable) EpochToPrunedBySize(targetSize int64, latestFinalizedEpoch iotago.EpochIndex) (iotago.EpochIndex, error) {
	lastPrunedEpoch := lo.Return1(p.prunableSlotStore.LastPrunedEpoch())

	var sum int64
	for i := lastPrunedEpoch + 1; i <= latestFinalizedEpoch-p.defaultPruningDelay; i++ {
		semiSize, err := p.semiPermanentDBSizeByEpoch(i)
		if err != nil {
			return 0, ierrors.Wrapf(err, "failed to get semiPermanentDB size in EpochToPrunedBasedOnSize")
		}
		sum += semiSize

		bucketSize, err := p.prunableSlotStore.BucketSize(i)
		if err != nil {
			return 0, ierrors.Wrapf(err, "failed to get bucket size in EpochToPrunedBasedOnSize")
		}
		sum += bucketSize

		if sum >= targetSize {
			return i, nil
		}
	}

	if sum >= targetSize {
		return latestFinalizedEpoch - p.defaultPruningDelay, nil
	}

	// TODO: do we return error here, or prune as much as we could
	return 0, ierrors.Errorf("failed to prune to target size %d", targetSize)
}

func (p *Prunable) semiPermanentDBSizeByEpoch(epoch iotago.EpochIndex) (int64, error) {
	var sum int64

	if prunable, versionHash, err := p.decidedUpgradeSignals.LoadPrunable(epoch); err != nil {
		return 0, ierrors.Wrapf(err, "failed to load decidedUpgradeSignals with delay for epoch %d", epoch)
	} else if prunable {
		sum += int64(len(lo.Return1(versionHash.Bytes()))) + 8 // epoch key
	}

	if prunable, stats, err := p.poolStats.LoadPrunable(epoch); err != nil {
		return 0, ierrors.Wrapf(err, "failed to load poolStats with delay for epoch %d", epoch)
	} else if prunable {
		sum += int64(len(lo.Return1(stats.Bytes()))) + 8 // epoch key
	}

	if prunable, committee, err := p.committee.LoadPrunable(epoch); err != nil {
		return 0, ierrors.Wrapf(err, "failed to load committee with delay for epoch %d", epoch)
	} else if prunable {
		sum += int64(len(lo.Return1(committee.Bytes()))) + 8 // epoch key
	}

	if rewardsStore := p.poolRewards.GetPrunableEpoch(epoch); rewardsStore != nil {
		rewardsStore.Iterate(kvstore.EmptyPrefix, func(key kvstore.Key, value kvstore.Value) bool {
			sum += int64(len(key) + len(value))

			return true
		})
	}

	return sum, nil
}
