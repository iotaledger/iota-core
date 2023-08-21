package storage

import (
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

func (s *Storage) LastPrunedEpoch() (index iotago.EpochIndex, hasPruned bool) {
	s.pruningLock.RLock()
	defer s.pruningLock.RUnlock()

	return s.lastPrunedEpoch.Index()
}

func (s *Storage) TryPrune() error {
	// prune finalizedEpoch - s.optsPruningDelay if possible
	if _, _, err := s.PruneByDepth(s.optsPruningDelay); err != nil {
		if ierrors.As(err, database.ErrNoPruningNeeded) {
			return nil
		}
		return ierrors.Wrap(err, "failed to prune with PruneByDepth in TryPrune")
	}

	// the disk could still be full after PruneByDepth, thus need to check by size again and prune if needed.
	if err := s.PruneBySize(); err != nil {
		if ierrors.As(err, database.ErrNoPruningNeeded) {
			return nil
		}
		return ierrors.Wrap(err, "failed to prune with PruneBySize in TryPrune")
	}

	return nil
}

// PruneByEpochIndex prunes the database until the given epoch. It returns an error if the epoch is too old or too new.
// It is to be called by the user e.g. via the WebAPI.
func (s *Storage) PruneByEpochIndex(epoch iotago.EpochIndex) error {
	s.pruningLock.Lock()
	defer s.pruningLock.Unlock()

	s.setIsPruning(true)
	defer s.setIsPruning(false)

	// Make sure epoch is not too recent or not yet finalized.
	latestPrunableEpoch := s.latestPrunableEpoch()
	if epoch > latestPrunableEpoch {
		return ierrors.Errorf("epoch %d is too new, latest prunable epoch is %d", epoch, latestPrunableEpoch)
	}

	// Make sure epoch is not already pruned.
	start, canPrune := s.getPruningStart(epoch)
	if !canPrune {
		return ierrors.Errorf("epoch %d is too old, last pruned epoch is %d", epoch, lo.Return1(s.lastPrunedEpoch.Index()))
	}

	if err := s.pruneUntilEpoch(start, epoch, 0); err != nil {
		return ierrors.Wrapf(err, "failed to prune from epoch %d to %d", start, epoch)
	}

	return nil
}

func (s *Storage) PruneByDepth(depth iotago.EpochIndex) (firstPruned, lastPruned iotago.EpochIndex, err error) {
	s.pruningLock.Lock()
	defer s.pruningLock.Unlock()

	s.setIsPruning(true)
	defer s.setIsPruning(false)

	if depth == 0 {
		return 0, 0, database.ErrNoPruningNeeded
	}

	latestPrunableEpoch := s.latestPrunableEpoch()
	if depth > latestPrunableEpoch {
		return 0, 0, ierrors.Errorf("depth %d is too big, latest prunable epoch is %d", depth, latestPrunableEpoch)
	}

	// We need to do (depth-1) because latestPrunableEpoch is already making sure that we keep at least one full epoch.
	end := latestPrunableEpoch - (depth - 1)

	// Make sure epoch is not already pruned.
	start, canPrune := s.getPruningStart(end)
	if !canPrune {
		return 0, 0, ierrors.Wrapf(database.ErrEpochPruned, "depth %d is too big, want to prune until %d but pruned epoch is already %d", depth, end, lo.Return1(s.lastPrunedEpoch.Index()))
	}

	if err := s.pruneUntilEpoch(start, end, depth); err != nil {
		return 0, 0, ierrors.Wrapf(err, "failed to prune from epoch %d to %d", start, end)
	}

	return start, end, nil
}

func (s *Storage) PruneBySize(targetSizeMaxBytes ...int64) error {
	s.pruningLock.Lock()
	defer s.pruningLock.Unlock()

	s.setIsPruning(true)
	defer s.setIsPruning(false)

	// pruning by size deactivated
	if !s.optPruningSizeEnabled && len(targetSizeMaxBytes) == 0 {
		return database.ErrNoPruningNeeded
	}

	targetDatabaseSizeMaxBytes := s.optsPruningSizeMaxTargetSizeBytes
	if len(targetSizeMaxBytes) > 0 {
		targetDatabaseSizeMaxBytes = targetSizeMaxBytes[0]
	}

	// No need to prune. The database is already smaller than the start threshold size.
	if s.Size() < int64(float64(targetDatabaseSizeMaxBytes)*s.optsPruningSizeStartThresholdPercentage) {
		return database.ErrNoPruningNeeded
	}

	latestPrunableEpoch := s.latestPrunableEpoch()

	// Make sure epoch is not already pruned.
	start, canPrune := s.getPruningStart(latestPrunableEpoch)
	if !canPrune {
		return ierrors.Wrapf(database.ErrEpochPruned, "can't prune any more data: latest prunable epoch is %d but pruned epoch is already %d", latestPrunableEpoch, lo.Return1(s.lastPrunedEpoch.Index()))
	}

	targetDatabaseSizeBytes := int64(float64(targetDatabaseSizeMaxBytes) * s.optsPruningSizeTargetThresholdPercentage)

	var prunedEpoch iotago.EpochIndex
	for prunedEpoch = start; prunedEpoch <= latestPrunableEpoch; prunedEpoch++ {
		bucketSize, err := s.prunable.BucketSize(prunedEpoch)
		if err != nil {
			return ierrors.Wrapf(err, "failed to get bucket size for epoch %d", prunedEpoch)
		}

		// add 10% as an estimate for semiPermanentDB and 20% for ledger state: calculating the exact size would be too heavy.
		targetDatabaseSizeBytes -= int64(float64(bucketSize) * 1.2)

		// Actually prune the epoch.
		if err := s.pruneUntilEpoch(prunedEpoch, prunedEpoch, 0); err != nil {
			return ierrors.Wrapf(err, "failed to prune epoch %d", prunedEpoch)
		}

		// We have pruned sufficiently.
		if targetDatabaseSizeBytes <= 0 {
			return nil
		}
	}

	// If the size of the database is still bigger than the target size, after we tried to prune everything possible,
	// we return an error so that the user can be notified about a potentially full disk.
	if s.Size() < int64(float64(targetDatabaseSizeMaxBytes)*s.optsPruningSizeStartThresholdPercentage) {
		return ierrors.Wrapf(database.ErrDatabaseFull, "database size is still bigger than the start threshold size after pruning: %d > %d", s.Size(), int64(float64(targetDatabaseSizeMaxBytes)*s.optsPruningSizeStartThresholdPercentage))
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
func (s *Storage) pruneUntilEpoch(start iotago.EpochIndex, target iotago.EpochIndex, pruningDelay iotago.EpochIndex) error {
	for currentIndex := start; currentIndex <= target; currentIndex++ {
		if err := s.prunable.Prune(currentIndex, pruningDelay); err != nil {
			return ierrors.Wrapf(err, "failed to prune epoch in prunable %d", currentIndex)
		}

		if err := s.permanent.PruneUTXOLedger(currentIndex); err != nil {
			return ierrors.Wrapf(err, "failed to prune epoch in permanent %d", currentIndex)
		}
	}

	s.lastPrunedEpoch.MarkEvicted(target)

	return nil
}
