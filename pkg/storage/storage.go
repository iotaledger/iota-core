package storage

import (
	"math"
	"sync"

	"github.com/iotaledger/hive.go/ierrors"
	hivedb "github.com/iotaledger/hive.go/kvstore/database"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	"github.com/iotaledger/iota-core/pkg/storage/permanent"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	permanentDirName = "permanent"
	prunableDirName  = "prunable"

	storePrefixHealth byte = 255
)

// Storage is an abstraction around the storage layer of the node.
type Storage struct {
	dir *utils.Directory

	// Permanent is the section of the storage that is maintained forever (holds the current ledger state).
	permanent         *permanent.Permanent
	ledgerPruningFunc func(epoch iotago.EpochIndex)

	// Prunable is the section of the storage that is pruned regularly (holds the history of the ledger state).
	prunable *prunable.Prunable

	shutdownOnce sync.Once
	errorHandler func(error)

	isPruning   bool
	statusLock  sync.RWMutex
	pruningLock sync.Mutex

	optsDBEngine                      hivedb.Engine
	optsAllowedDBEngines              []hivedb.Engine
	optsPruningDelay                  iotago.EpochIndex
	optPruningSizeEnabled             bool
	optPruningSizeMaxTargetSizeBytes  int64
	optPruningSizeThresholdPercentage float64
	optsPrunableManagerOptions        []options.Option[prunable.PrunableSlotManager]
}

// New creates a new storage instance with the named database version in the given directory.
func New(directory string, dbVersion byte, errorHandler func(error), opts ...options.Option[Storage]) *Storage {
	return options.Apply(&Storage{
		dir:                               utils.NewDirectory(directory, true),
		errorHandler:                      errorHandler,
		optsDBEngine:                      hivedb.EngineRocksDB,
		optsPruningDelay:                  2,       // TODO: what's the default now?
		optPruningSizeMaxTargetSizeBytes:  1 << 30, // 1GB, TODO: what's the default?
		optPruningSizeThresholdPercentage: 0.9,     // TODO: what's the default?
	}, opts,
		func(s *Storage) {
			dbConfig := database.Config{
				Engine:       s.optsDBEngine,
				Directory:    s.dir.PathWithCreate(permanentDirName),
				Version:      dbVersion,
				PrefixHealth: []byte{storePrefixHealth},
			}

			s.permanent = permanent.New(dbConfig, errorHandler)
			s.prunable = prunable.New(dbConfig.WithDirectory(s.dir.PathWithCreate(prunableDirName)), s.optsPruningDelay, s.Settings().APIProvider(), errorHandler, s.optsPrunableManagerOptions...)
		})
}

func (s *Storage) setIsPruning(value bool) {
	s.statusLock.Lock()
	s.isPruning = value
	s.statusLock.Unlock()
}

func (s *Storage) IsPruning() bool {
	s.statusLock.RLock()
	defer s.statusLock.RUnlock()

	return s.isPruning
}

func (s *Storage) Directory() string {
	return s.dir.Path()
}

// PrunableDatabaseSize returns the size of the underlying prunable databases.
func (s *Storage) PrunableDatabaseSize() int64 {
	return s.prunable.Size()
}

// PermanentDatabaseSize returns the size of the underlying permanent database and files.
func (s *Storage) PermanentDatabaseSize() int64 {
	return s.permanent.Size()
}

func (s *Storage) Size() int64 {
	return s.PermanentDatabaseSize() + s.PrunableDatabaseSize()
}

func (s *Storage) PruneByEpochIndex(epoch iotago.EpochIndex) {
	s.pruningLock.Lock()
	defer s.pruningLock.Unlock()

	// TODO: check for last finalized epoch

	s.setIsPruning(true)
	defer s.setIsPruning(false)

	s.prunable.PruneUntilEpoch(epoch)

	// TODO: call ledger pruning
}

func (s *Storage) PruneByDepth(depth iotago.EpochIndex) {
	s.pruningLock.Lock()
	defer s.pruningLock.Unlock()

	s.setIsPruning(true)
	defer s.setIsPruning(false)

	latestFinalizedSlot := s.Settings().LatestFinalizedSlot()
	start := s.Settings().APIProvider().APIForSlot(latestFinalizedSlot).TimeProvider().EpochFromSlot(latestFinalizedSlot)
	if start < depth {
		s.errorHandler(ierrors.Errorf("pruning depth %d is greater than the current epoch %d", depth, start))
		return
	}

	s.prunable.PruneUntilEpoch(start - depth)
	// double check permanent Ledger
}

// This is now similar to how hornet pruneBySize, by calculating the ratio of the current size to the target size then calculate the number of epochs to prune. So it's not precise.
func (s *Storage) PruneBySize(targetSizeBytes ...int64) {
	if !s.optPruningSizeEnabled && len(targetSizeBytes) == 0 {
		// pruning by size deactivated
		return
	}

	s.pruningLock.Lock()
	defer s.pruningLock.Unlock()

	s.setIsPruning(true)
	defer s.setIsPruning(false)

	targetDatabaseSizeBytes := s.optPruningSizeMaxTargetSizeBytes
	if len(targetSizeBytes) > 0 {
		targetDatabaseSizeBytes = targetSizeBytes[0]
	}

	currentSize := s.Size()
	// No need to prune. The database is already smaller than the target size.
	if targetDatabaseSizeBytes < 0 || currentSize < targetDatabaseSizeBytes {
		return
	}

	latestEpoch := s.Settings().APIProvider().CurrentAPI().TimeProvider().EpochFromSlot(s.Settings().LatestFinalizedSlot())
	lastPrunedEpoch := lo.Return1(s.prunable.LastPrunedEpoch())

	pruningRange := latestEpoch - lastPrunedEpoch
	prunedDatabaseSizeBytes := float64(targetDatabaseSizeBytes) * (1.0 - s.optPruningSizeThresholdPercentage)
	epochDiff := math.Ceil(float64(pruningRange) * prunedDatabaseSizeBytes / float64(currentSize))

	s.prunable.PruneUntilEpoch(latestEpoch - iotago.EpochIndex(epochDiff))

	// what if permanent is really large? how to prune it?
}

// Shutdown shuts down the storage.
func (s *Storage) Shutdown() {
	s.shutdownOnce.Do(func() {
		s.permanent.Shutdown()
		s.prunable.Shutdown()
	})
}

func (s *Storage) Flush() {
	s.permanent.Flush()
	s.prunable.Flush()
}
