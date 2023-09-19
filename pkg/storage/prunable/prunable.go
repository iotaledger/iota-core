package prunable

import (
	"fmt"

	copydir "github.com/otiai10/copy"

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
	prunableSlotStore *BucketManager
	errorHandler      func(error)

	semiPermanentDBConfig database.Config
	semiPermanentDB       *database.DBInstance

	decidedUpgradeSignals *epochstore.Store[model.VersionAndHash]
	poolRewards           *epochstore.EpochKVStore
	poolStats             *epochstore.Store[*model.PoolsStats]
	committee             *epochstore.Store[*account.Accounts]
}

func New(dbConfig database.Config, apiProvider api.Provider, errorHandler func(error), opts ...options.Option[BucketManager]) *Prunable {
	dir := utils.NewDirectory(dbConfig.Directory, true)
	semiPermanentDBConfig := dbConfig.WithDirectory(dir.PathWithCreate("semipermanent"))
	semiPermanentDB := database.NewDBInstance(semiPermanentDBConfig)

	return &Prunable{
		apiProvider:       apiProvider,
		errorHandler:      errorHandler,
		prunableSlotStore: NewBucketManager(dbConfig, errorHandler, opts...),

		semiPermanentDBConfig: semiPermanentDBConfig,
		semiPermanentDB:       semiPermanentDB,
		decidedUpgradeSignals: epochstore.NewStore(kvstore.Realm{epochPrefixDecidedUpgradeSignals}, kvstore.Realm{lastPrunedEpochKey}, semiPermanentDB.KVStore(), pruningDelayDecidedUpgradeSignals, model.VersionAndHash.Bytes, model.VersionAndHashFromBytes),
		poolRewards:           epochstore.NewEpochKVStore(kvstore.Realm{epochPrefixPoolRewards}, kvstore.Realm{lastPrunedEpochKey}, semiPermanentDB.KVStore(), pruningDelayPoolRewards),
		poolStats:             epochstore.NewStore(kvstore.Realm{epochPrefixPoolStats}, kvstore.Realm{lastPrunedEpochKey}, semiPermanentDB.KVStore(), pruningDelayPoolStats, (*model.PoolsStats).Bytes, model.PoolsStatsFromBytes),
		committee:             epochstore.NewStore(kvstore.Realm{epochPrefixCommittee}, kvstore.Realm{lastPrunedEpochKey}, semiPermanentDB.KVStore(), pruningDelayCommittee, (*account.Accounts).Bytes, account.AccountsFromBytes),
	}
}

func Clone(source *Prunable, dbConfig database.Config, apiProvider api.Provider, errorHandler func(error), opts ...options.Option[BucketManager]) (*Prunable, error) {
	// Lock semi-permanent DB and prunable slot store so that nobody can try to use or open them while cloning.
	source.semiPermanentDB.Lock()
	defer source.semiPermanentDB.Unlock()

	source.prunableSlotStore.mutex.Lock()
	defer source.prunableSlotStore.mutex.Unlock()

	// Close forked prunable storage before copying its contents.
	source.semiPermanentDB.CloseWithoutLocking()
	source.prunableSlotStore.Shutdown()

	// Copy the storage on disk to new location.
	if err := copydir.Copy(source.prunableSlotStore.dbConfig.Directory, dbConfig.Directory); err != nil {
		return nil, ierrors.Wrap(err, "failed to copy prunable storage directory to new storage path")
	}

	source.semiPermanentDB.Open()

	return New(dbConfig, apiProvider, errorHandler, opts...), nil
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

	return p.prunableSlotStore.TotalSize() + semiSize
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

func (p *Prunable) Rollback(targetSlotIndex iotago.SlotIndex) error {
	timeProvider := p.apiProvider.APIForSlot(targetSlotIndex).TimeProvider()
	targetSlotEpoch := timeProvider.EpochFromSlot(targetSlotIndex)
	lastCommittedEpoch := targetSlotEpoch
	// if the target index is the last slot of the epoch, the epoch was committed
	if timeProvider.EpochEnd(targetSlotEpoch) != targetSlotIndex {
		lastCommittedEpoch--
	}

	if err := p.prunableSlotStore.RollbackBucket(targetSlotEpoch, targetSlotIndex, timeProvider.EpochEnd(targetSlotEpoch)); err != nil {
		return ierrors.Wrapf(err, "error while rolling back slots in a bucket for epoch %d", targetSlotEpoch)
	}

	// Shut down the prunableSlotStore in order to flush and get consistent state on disk after reopening.
	p.prunableSlotStore.Shutdown()

	// Removed entries that belong to the old fork and cannot be re-used.
	for epochIdx := lastCommittedEpoch + 1; ; epochIdx++ {
		if epochIdx > targetSlotEpoch {
			shouldRollback, err := p.shouldRollbackCommittee(epochIdx, targetSlotIndex)
			if err != nil {
				return ierrors.Wrapf(err, "error while checking if committee for epoch %d should be rolled back", epochIdx)
			}

			fmt.Println("rollback committee", shouldRollback, "epochIdx", epochIdx, "lastCommittedEpoch", lastCommittedEpoch, "targetSlotEpoch", targetSlotEpoch)
			if shouldRollback {
				if err := p.committee.DeleteEpoch(epochIdx); err != nil {
					return ierrors.Wrapf(err, "error while deleting committee for epoch %d", epochIdx)
				}
			}

			if deleted := p.prunableSlotStore.DeleteBucket(epochIdx); !deleted {
				break
			}
		}

		if err := p.poolRewards.DeleteEpoch(epochIdx); err != nil {
			return ierrors.Wrapf(err, "error while deleting pool rewards for epoch %d", epochIdx)
		}
		if err := p.poolStats.DeleteEpoch(epochIdx); err != nil {
			return ierrors.Wrapf(err, "error while deleting pool stats for epoch %d", epochIdx)
		}

		if err := p.decidedUpgradeSignals.DeleteEpoch(epochIdx); err != nil {
			return ierrors.Wrapf(err, "error while deleting decided upgrade signals for epoch %d", epochIdx)
		}
	}

	return nil
}

// Remove committee for the next epoch only if forking point is before point of no return and committee is reused.
// Always remove committees for epochs that are newer than targetSlotEpoch+1.
func (p *Prunable) shouldRollbackCommittee(epochIndex iotago.EpochIndex, targetSlotIndex iotago.SlotIndex) (bool, error) {
	timeProvider := p.apiProvider.APIForSlot(targetSlotIndex).TimeProvider()
	targetSlotEpoch := timeProvider.EpochFromSlot(targetSlotIndex)
	pointOfNoReturn := timeProvider.EpochEnd(targetSlotEpoch) - p.apiProvider.APIForSlot(targetSlotIndex).ProtocolParameters().MaxCommittableAge()

	if epochIndex >= targetSlotEpoch+1 {
		if targetSlotIndex < pointOfNoReturn {
			committee, err := p.committee.Load(targetSlotEpoch + 1)
			if err != nil {
				return false, err
			}

			return committee.IsReused(), nil
		}

		return false, nil
	}

	return true, nil
}
