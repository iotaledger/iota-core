package storage

import (
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (s *Storage) setIsPruning(value bool) {
	s.statusLock.Lock()
	defer s.statusLock.Unlock()

	s.isPruning = value
}

func (s *Storage) IsPruning() bool {
	s.statusLock.RLock()
	defer s.statusLock.RUnlock()

	return s.isPruning
}

func (s *Storage) LastPrunedEpoch() (epoch iotago.EpochIndex, hasPruned bool) {
	s.pruningLock.RLock()
	defer s.pruningLock.RUnlock()

	return s.lastPrunedEpoch.Index()
}

func (s *Storage) TryPrune() error {
	// Prune finalizedEpoch - s.optsPruningDelay if possible.
	if _, _, err := s.PruneByDepth(s.optsPruningDelay); err != nil {
		if ierrors.Is(err, database.ErrNoPruningNeeded) || ierrors.Is(err, database.ErrEpochPruned) {
			return nil
		}

		return ierrors.Wrap(err, "failed to prune with PruneByDepth")
	}

	// Disk could still be full after PruneByDepth, thus need to check by size again and prune if needed.
	if err := s.PruneBySize(); err != nil {
		if ierrors.Is(err, database.ErrNoPruningNeeded) {
			return nil
		}

		return ierrors.Wrap(err, "failed to prune with PruneBySize for")
	}

	return nil
}

// PruneByEpochIndex prunes the database until the given epoch. It returns an error if the epoch is too old or too new.
// It is to be called by the user e.g. via the WebAPI.
func (s *Storage) PruneByEpochIndex(epoch iotago.EpochIndex) error {
	// Make sure epoch is not too recent or not yet finalized.
	latestPrunableEpoch := s.latestPrunableEpoch()
	if epoch > latestPrunableEpoch {
		return ierrors.Errorf("epoch %d is too new, latest prunable epoch is %d", epoch, latestPrunableEpoch)
	}

	s.pruningLock.Lock()
	defer s.pruningLock.Unlock()

	// Make sure epoch is not already pruned.
	start, canPrune := s.getPruningStart(epoch)
	if !canPrune {
		return ierrors.Errorf("epoch %d is too old, last pruned epoch is %d", epoch, lo.Return1(s.lastPrunedEpoch.Index()))
	}

	s.setIsPruning(true)
	defer s.setIsPruning(false)

	if err := s.pruneUntilEpoch(start, epoch, 0); err != nil {
		return ierrors.Wrapf(err, "failed to prune from epoch %d to %d", start, epoch)
	}

	return nil
}

func (s *Storage) PruneByDepth(epochDepth iotago.EpochIndex) (firstPruned iotago.EpochIndex, lastPruned iotago.EpochIndex, err error) {
	// Depth of 0 and 1 means we prune to the latestPrunableEpoch.
	if epochDepth == 0 {
		epochDepth = 1
	}

	latestPrunableEpoch := s.latestPrunableEpoch()
	if epochDepth > latestPrunableEpoch {
		return 0, 0, ierrors.WithMessagef(database.ErrNoPruningNeeded, "epochDepth %d is too big, latest prunable epoch is %d", epochDepth, latestPrunableEpoch)
	}

	// We need to do (epochDepth-1) because latestPrunableEpoch is already making sure that we keep at least one full epoch.
	end := latestPrunableEpoch - (epochDepth - 1)

	s.pruningLock.Lock()
	defer s.pruningLock.Unlock()

	// Make sure epoch is not already pruned.
	start, canPrune := s.getPruningStart(end)
	if !canPrune {
		return 0, 0, ierrors.WithMessagef(database.ErrEpochPruned, "epochDepth %d is too big, want to prune until %d but pruned epoch is already %d", epochDepth, end, lo.Return1(s.lastPrunedEpoch.Index()))
	}

	s.setIsPruning(true)
	defer s.setIsPruning(false)

	if err := s.pruneUntilEpoch(start, end, epochDepth); err != nil {
		return 0, 0, ierrors.Wrapf(err, "failed to prune from epoch %d to %d", start, end)
	}

	return start, end, nil
}

func (s *Storage) PruneBySize(targetSizeMaxBytes ...int64) error {
	// pruning by size deactivated
	if !s.optPruningSizeEnabled && len(targetSizeMaxBytes) == 0 {
		return database.ErrNoPruningNeeded
	}

	s.pruningLock.Lock()
	defer s.pruningLock.Unlock()

	if time.Since(s.lastPrunedSizeTime) < s.optsPruningSizeCooldownTime {
		return ierrors.WithMessagef(database.ErrNoPruningNeeded, "last pruning by size was %s ago, cooldown time is %s", time.Since(s.lastPrunedSizeTime), s.optsPruningSizeCooldownTime)
	}

	// The target size is the maximum size of the database after pruning.
	dbMaxSize := s.optsPruningSizeMaxTargetSizeBytes
	dbTargetSizeAfterPruning := int64(float64(s.optsPruningSizeMaxTargetSizeBytes) * (1 - s.optsPruningSizeReductionPercentage))

	// The function has been called by the user explicitly with a target size.
	if len(targetSizeMaxBytes) > 0 {
		dbMaxSize = targetSizeMaxBytes[0]
		dbTargetSizeAfterPruning = targetSizeMaxBytes[0]
	}

	// No need to prune. The database is already smaller than the start threshold size.
	currentDBSize := s.Size()
	if currentDBSize < dbMaxSize {
		return database.ErrNoPruningNeeded
	}

	latestPrunableEpoch := s.latestPrunableEpoch()

	// Make sure epoch is not already pruned.
	start, canPrune := s.getPruningStart(latestPrunableEpoch)
	if !canPrune {
		return ierrors.WithMessagef(database.ErrEpochPruned, "can't prune any more data: latest prunable epoch is %d but pruned epoch is already %d", latestPrunableEpoch, lo.Return1(s.lastPrunedEpoch.Index()))
	}

	s.setIsPruning(true)
	defer func() {
		s.setIsPruning(false)
		s.lastPrunedSizeTime = time.Now()
	}()

	totalBytesToPrune := currentDBSize - dbTargetSizeAfterPruning

	for prunedEpoch := start; prunedEpoch <= latestPrunableEpoch; prunedEpoch++ {
		bucketSize, err := s.prunable.BucketSize(prunedEpoch)
		if err != nil {
			return ierrors.Wrapf(err, "failed to get bucket size for epoch %d", prunedEpoch)
		}

		// add 10% as an estimate for semiPermanentDB and 10% for ledger state: calculating the exact size would be too heavy.
		totalBytesToPrune -= int64(float64(bucketSize) * 1.2)

		// Actually prune the epoch.
		if err := s.pruneUntilEpoch(prunedEpoch, prunedEpoch, 0); err != nil {
			return ierrors.Wrapf(err, "failed to prune epoch %d", prunedEpoch)
		}

		// We have pruned sufficiently.
		if totalBytesToPrune <= 0 {
			return nil
		}
	}

	// If the size of the database is still bigger than the max size, after we tried to prune everything possible,
	// we return an error so that the user can be notified about a potentially full disk.
	if currentDBSize = s.Size(); currentDBSize > dbMaxSize {
		return ierrors.WithMessagef(database.ErrDatabaseFull, "database size is still bigger than the start threshold size after pruning: %d > %d", currentDBSize, dbMaxSize)
	}

	return nil
}

func (s *Storage) getPruningStart(epoch iotago.EpochIndex) (iotago.EpochIndex, bool) {
	lastPrunedEpoch, hasPruned := s.lastPrunedEpoch.Index()
	if hasPruned && epoch <= lastPrunedEpoch {
		return 0, false
	}

	return s.lastPrunedEpoch.NextIndex(), true
}

func (s *Storage) latestPrunableEpoch() iotago.EpochIndex {
	latestFinalizedSlot := s.Settings().LatestFinalizedSlot()
	currentFinalizedEpoch := s.Settings().APIProvider().APIForSlot(latestFinalizedSlot).TimeProvider().EpochFromSlot(latestFinalizedSlot)

	// We always want at least 1 full epoch of history. Thus, the latest prunable epoch is the current finalized epoch - 2.
	if currentFinalizedEpoch < 2 {
		return 0
	}

	return currentFinalizedEpoch - 2
}

// PruneUntilEpoch prunes the database until the given epoch.
// The caller needs to make sure that the start and target epoch take into account the specified pruning delay.
func (s *Storage) pruneUntilEpoch(startEpoch iotago.EpochIndex, targetEpoch iotago.EpochIndex, pruningDelay iotago.EpochIndex) error {
	for currentEpoch := startEpoch; currentEpoch <= targetEpoch; currentEpoch++ {
		if err := s.prunable.Prune(currentEpoch, pruningDelay); err != nil {
			return ierrors.Wrapf(err, "failed to prune epoch %d in prunable", currentEpoch)
		}

		if err := s.permanent.PruneUTXOLedger(currentEpoch); err != nil {
			return ierrors.Wrapf(err, "failed to prune epoch %d in permanent", currentEpoch)
		}
	}

	s.lastPrunedEpoch.MarkEvicted(targetEpoch)

	s.Pruned.Trigger(targetEpoch)

	return nil
}
