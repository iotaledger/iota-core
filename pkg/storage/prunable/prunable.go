package prunable

import (
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
)

type Prunable struct {
	apiProvider       iotago.APIProvider
	prunableSlotStore *BucketManager
	errorHandler      func(error)

	semiPermanentDBConfig database.Config
	semiPermanentDB       *database.DBInstance

	decidedUpgradeSignals *epochstore.Store[model.VersionAndHash]
	poolRewards           *epochstore.EpochKVStore
	poolStats             *epochstore.Store[*model.PoolsStats]
	committee             *epochstore.Store[*account.Accounts]
}

func New(dbConfig database.Config, apiProvider iotago.APIProvider, errorHandler func(error), opts ...options.Option[BucketManager]) *Prunable {
	dir := utils.NewDirectory(dbConfig.Directory, true)
	semiPermanentDBConfig := dbConfig.WithDirectory(dir.PathWithCreate("semipermanent"))
	// openedCallback is nil because we don't need to do anything when reopening the store.
	semiPermanentDB := database.NewDBInstance(semiPermanentDBConfig, nil)

	return &Prunable{
		apiProvider:       apiProvider,
		errorHandler:      errorHandler,
		prunableSlotStore: NewBucketManager(dbConfig, errorHandler, opts...),

		semiPermanentDBConfig: semiPermanentDBConfig,
		semiPermanentDB:       semiPermanentDB,
		decidedUpgradeSignals: epochstore.NewStore(kvstore.Realm{epochPrefixDecidedUpgradeSignals}, semiPermanentDB.KVStore(), pruningDelayDecidedUpgradeSignals, model.VersionAndHash.Bytes, model.VersionAndHashFromBytes),
		poolRewards:           epochstore.NewEpochKVStore(kvstore.Realm{epochPrefixPoolRewards}, semiPermanentDB.KVStore(), pruningDelayPoolRewards),
		poolStats:             epochstore.NewStore(kvstore.Realm{epochPrefixPoolStats}, semiPermanentDB.KVStore(), pruningDelayPoolStats, (*model.PoolsStats).Bytes, model.PoolsStatsFromBytes),
		committee:             epochstore.NewStore(kvstore.Realm{epochPrefixCommittee}, semiPermanentDB.KVStore(), pruningDelayCommittee, (*account.Accounts).Bytes, account.AccountsFromBytes),
	}
}

func Clone(source *Prunable, dbConfig database.Config, apiProvider iotago.APIProvider, errorHandler func(error), opts ...options.Option[BucketManager]) (*Prunable, error) {
	// Lock semi-permanent DB and prunable slot store so that nobody can try to use or open them while cloning.
	source.semiPermanentDB.LockAccess()
	defer source.semiPermanentDB.UnlockAccess()

	source.prunableSlotStore.Lock()
	defer source.prunableSlotStore.Unlock()

	// Close forked prunable storage before copying its contents. All necessary locks are already acquired.
	source.semiPermanentDB.CloseWithoutLocking()
	source.prunableSlotStore.CloseWithoutLocking()

	// Copy the storage on disk to new location.
	if err := copydir.Copy(source.prunableSlotStore.dbConfig.Directory, dbConfig.Directory); err != nil {
		return nil, ierrors.Wrap(err, "failed to copy prunable storage directory to new storage path")
	}

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

func (p *Prunable) Rollback(targetEpoch iotago.EpochIndex, startPruneRange iotago.SlotIndex, endPruneRange iotago.SlotIndex) error {
	if err := p.prunableSlotStore.PruneSlots(targetEpoch, startPruneRange, endPruneRange); err != nil {
		return ierrors.Wrapf(err, "failed to prune slots in range [%d, %d] from target epoch %d", startPruneRange, endPruneRange, targetEpoch)
	}

	if err := p.rollbackCommitteesCandidates(targetEpoch, startPruneRange); err != nil {
		return ierrors.Wrapf(err, "failed to rollback committee candidates to target epoch %d", targetEpoch)
	}

	lastPrunedCommitteeEpoch, err := p.rollbackCommitteeEpochs(targetEpoch+1, startPruneRange-1)
	if err != nil {
		return ierrors.Wrapf(err, "failed to rollback committee epochs to target epoch %d", targetEpoch)
	}

	lastPrunedPoolStatsEpoch, err := p.poolStats.RollbackEpochs(targetEpoch)
	if err != nil {
		return ierrors.Wrapf(err, "failed to rollback pool stats epochs to target epoch %d", targetEpoch)
	}

	lastPrunedDecidedUpgradeSignalsEpoch, err := p.decidedUpgradeSignals.RollbackEpochs(targetEpoch)
	if err != nil {
		return ierrors.Wrapf(err, "failed to rollback decided upgrade signals epochs to target epoch %d", targetEpoch)
	}

	lastPrunedPoolRewardsEpoch, err := p.poolRewards.RollbackEpochs(targetEpoch)
	if err != nil {
		return ierrors.Wrapf(err, "failed to rollback pool rewards epochs to target epoch %d", targetEpoch)
	}

	for epochToPrune := targetEpoch + 1; epochToPrune <= max(
		lastPrunedCommitteeEpoch,
		lastPrunedPoolStatsEpoch,
		lastPrunedDecidedUpgradeSignalsEpoch,
		lastPrunedPoolRewardsEpoch,
	); epochToPrune++ {
		p.prunableSlotStore.DeleteBucket(epochToPrune)
	}

	return nil
}

func (p *Prunable) rollbackCommitteeEpochs(epoch iotago.EpochIndex, targetSlot iotago.SlotIndex) (lastPrunedEpoch iotago.EpochIndex, err error) {
	lastAccessedEpoch, err := p.committee.LastAccessedEpoch()
	if err != nil {
		return lastAccessedEpoch, ierrors.Wrap(err, "failed to get last accessed committee epoch")
	}

	for epochToPrune := epoch; epochToPrune <= lastAccessedEpoch; epochToPrune++ {
		if shouldRollback, rollbackErr := p.shouldRollbackCommittee(epochToPrune, targetSlot); rollbackErr != nil {
			return epochToPrune, ierrors.Wrapf(rollbackErr, "error while checking if committee for epoch %d should be rolled back", epochToPrune)
		} else if shouldRollback {
			if err = p.committee.DeleteEpoch(epochToPrune); err != nil {
				return epochToPrune, ierrors.Wrapf(err, "error while deleting committee for epoch %d", epochToPrune)
			}
		}
	}

	return lastAccessedEpoch, nil
}

// Remove committee for the next epoch only if forking point is before point of no return and committee is reused.
// Always remove committees for epochs that are newer than targetSlotEpoch+1.
func (p *Prunable) shouldRollbackCommittee(epoch iotago.EpochIndex, targetSlot iotago.SlotIndex) (bool, error) {
	timeProvider := p.apiProvider.APIForSlot(targetSlot).TimeProvider()
	targetSlotEpoch := timeProvider.EpochFromSlot(targetSlot)
	pointOfNoReturn := timeProvider.EpochEnd(targetSlotEpoch) - p.apiProvider.APIForSlot(targetSlot).ProtocolParameters().MaxCommittableAge()

	if epoch >= targetSlotEpoch+1 {
		if targetSlot < pointOfNoReturn {
			committee, err := p.committee.Load(targetSlotEpoch + 1)
			if err != nil {
				return false, err
			}

			if committee == nil {
				return false, nil
			}

			return committee.IsReused(), nil
		}

		return false, nil
	}

	return true, nil
}

func (p *Prunable) rollbackCommitteesCandidates(targetSlotEpoch iotago.EpochIndex, deletionStartSlot iotago.SlotIndex) error {
	candidatesToRollback := make([]iotago.AccountID, 0)

	candidates, err := p.CommitteeCandidates(targetSlotEpoch)
	if err != nil {
		return ierrors.Wrap(err, "failed to get candidates store")
	}

	if err = candidates.Iterate(kvstore.EmptyPrefix, func(accountID iotago.AccountID, candidacySlot iotago.SlotIndex) bool {
		if candidacySlot >= deletionStartSlot {
			candidatesToRollback = append(candidatesToRollback, accountID)
		}

		return true
	}); err != nil {
		return ierrors.Wrap(err, "failed to collect candidates to rollback")
	}

	for _, candidateToRollback := range candidatesToRollback {
		if err = candidates.Delete(candidateToRollback); err != nil {
			return ierrors.Wrapf(err, "failed to rollback candidate %s", candidateToRollback)
		}
	}

	return nil
}
