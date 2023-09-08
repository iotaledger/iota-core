package prunable

import (
	"os"

	copydir "github.com/otiai10/copy"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/serializer/v2/byteutils"
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
	// Close forked prunable storage before copying its contents.
	source.semiPermanentDB.Close()
	source.prunableSlotStore.Shutdown()

	// Copy the storage on disk to new location.
	if err := copydir.Copy(source.prunableSlotStore.dbConfig.Directory, dbConfig.Directory); err != nil {
		return nil, ierrors.Wrap(err, "failed to copy prunable storage directory to new storage path")
	}

	// Create a newly opened instance of prunable database.
	// `prunableSlotStore` will be opened automatically as the engine requests it, so no need to open it here.
	source.semiPermanentDB = database.NewDBInstance(source.semiPermanentDBConfig)
	source.decidedUpgradeSignals = epochstore.NewStore(kvstore.Realm{epochPrefixDecidedUpgradeSignals}, kvstore.Realm{lastPrunedEpochKey}, source.semiPermanentDB.KVStore(), pruningDelayDecidedUpgradeSignals, model.VersionAndHash.Bytes, model.VersionAndHashFromBytes)
	source.poolRewards = epochstore.NewEpochKVStore(kvstore.Realm{epochPrefixPoolRewards}, kvstore.Realm{lastPrunedEpochKey}, source.semiPermanentDB.KVStore(), pruningDelayPoolRewards)
	source.poolStats = epochstore.NewStore(kvstore.Realm{epochPrefixPoolStats}, kvstore.Realm{lastPrunedEpochKey}, source.semiPermanentDB.KVStore(), pruningDelayPoolStats, (*model.PoolsStats).Bytes, model.PoolsStatsFromBytes)
	source.committee = epochstore.NewStore(kvstore.Realm{epochPrefixCommittee}, kvstore.Realm{lastPrunedEpochKey}, source.semiPermanentDB.KVStore(), pruningDelayCommittee, (*account.Accounts).Bytes, account.AccountsFromBytes)

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

func (p *Prunable) Rollback(targetSlotIndex iotago.SlotIndex, lastCommittedIndex iotago.SlotIndex) error {
	timeProvider := p.apiProvider.APIForSlot(targetSlotIndex).TimeProvider()
	targetSlotEpoch := timeProvider.EpochFromSlot(targetSlotIndex)
	lastCommittedEpoch := targetSlotEpoch
	// if the target index is the last slot of the epoch, the epoch was committed
	if timeProvider.EpochEnd(targetSlotEpoch) != targetSlotIndex {
		lastCommittedEpoch--
	}
	pointOfNoReturn := timeProvider.EpochEnd(targetSlotEpoch) - p.apiProvider.APIForSlot(targetSlotIndex).ProtocolParameters().MaxCommittableAge()

	for epochIdx := lastCommittedEpoch + 1; ; epochIdx++ {
		// only remove if epochIdx bigger than epoch of target slot index
		if epochIdx > targetSlotEpoch {
			if exists, err := PathExists(dbPathFromIndex(p.prunableSlotStore.dbConfig.Directory, epochIdx)); err != nil {
				return ierrors.Wrapf(err, "failed to check if bucket directory exists in forkedPrunable storage for epoch %d", epochIdx)
			} else if !exists {
				break
			}

			if err := os.RemoveAll(dbPathFromIndex(p.prunableSlotStore.dbConfig.Directory, epochIdx)); err != nil {
				return ierrors.Wrapf(err, "failed to remove bucket directory in forkedPrunable storage for epoch %d", epochIdx)
			}

		}
		// Remove entries for epochs bigger or equal epochFromSlot(forkingPoint+1) in semiPermanent storage.
		// Those entries are part of the fork and values from the old storage should not be used
		// values from the candidate storage should be used in its place; that's why we copy those entries
		// from the candidate engine to old storage.
		if err := p.semiPermanentDB.KVStore().DeletePrefix(byteutils.ConcatBytes(kvstore.Realm{epochPrefixPoolRewards}, epochIdx.MustBytes())); err != nil {
			return err
		}
		if err := p.semiPermanentDB.KVStore().DeletePrefix(byteutils.ConcatBytes(kvstore.Realm{epochPrefixPoolStats}, epochIdx.MustBytes())); err != nil {
			return err
		}
		if epochIdx > targetSlotEpoch && targetSlotIndex < pointOfNoReturn {
			// TODO: rollback committee using
			//committee, _, err := account.AccountsFromBytes(committeeBytes)
			//if err != nil {
			//	innerErr = ierrors.Wrapf(err, "failed to parse committee bytes for epoch %d", epoch)
			//	return innerErr
			//}
			//if committee.IsReused() {
			//	return nil
			//}
			if err := p.semiPermanentDB.KVStore().DeletePrefix(byteutils.ConcatBytes(kvstore.Realm{epochPrefixCommittee}, (epochIdx + 1).MustBytes())); err != nil {
				return err
			}
		}

		if err := p.semiPermanentDB.KVStore().DeletePrefix(byteutils.ConcatBytes(kvstore.Realm{epochPrefixDecidedUpgradeSignals}, epochIdx.MustBytes())); err != nil {
			return err
		}
	}

	// If the forking slot is the last slot of an epoch, then don't need to clean anything as the bucket with the
	// first forked slot contains only forked blocks and was removed in the previous step.
	// We need to remove data in the bucket in slots [forkingSlot+1; bucketEnd].
	if lastCommittedEpoch != targetSlotEpoch {
		oldBucketKvStore := p.prunableSlotStore.getDBInstance(targetSlotEpoch).KVStore()
		for clearSlot := targetSlotIndex + 1; clearSlot <= timeProvider.EpochEnd(targetSlotEpoch); clearSlot++ {
			// delete slot prefix from forkedPrunable storage that will be eventually copied into the new engine
			if err := oldBucketKvStore.DeletePrefix(clearSlot.MustBytes()); err != nil {
				return ierrors.Wrapf(err, "error while clearing slot %d in bucket for epoch %d", clearSlot, targetSlotEpoch)
			}
		}
	}

	return nil
}

func PathExists(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}
