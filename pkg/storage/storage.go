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
	*permanent.Permanent

	// Prunable is the section of the storage that is pruned regularly (holds the history of the ledger state).
	*prunable.Prunable

	shutdownOnce sync.Once
	errorHandler func(error)

	isPruning  bool
	statusLock sync.RWMutex

	optsDBEngine                           hivedb.Engine
	optsAllowedDBEngines                   []hivedb.Engine
	optsPruningDelay                       iotago.EpochIndex
	optPruningSizeMaxTargetSizeBytes       int64
	optStartPruningSizeThresholdPercentage float64
	optStopPruningSizeThresholdPercentage  float64
	optsPrunableManagerOptions             []options.Option[prunable.PrunableSlotManager]
}

// New creates a new storage instance with the named database version in the given directory.
func New(directory string, dbVersion byte, errorHandler func(error), opts ...options.Option[Storage]) *Storage {
	return options.Apply(&Storage{
		dir:                                    utils.NewDirectory(directory, true),
		errorHandler:                           errorHandler,
		optsDBEngine:                           hivedb.EngineRocksDB,
		optsPruningDelay:                       2,       // TODO: what's the default now?
		optPruningSizeMaxTargetSizeBytes:       1 << 30, // 1GB, TODO: what's the default?
		optStartPruningSizeThresholdPercentage: 0.9,     // TODO: what's the default?
		optStopPruningSizeThresholdPercentage:  0.2,
	}, opts,
		func(s *Storage) {
			dbConfig := database.Config{
				Engine:       s.optsDBEngine,
				Directory:    s.dir.PathWithCreate(permanentDirName),
				Version:      dbVersion,
				PrefixHealth: []byte{storePrefixHealth},
			}

			s.Permanent = permanent.New(dbConfig, errorHandler)
			s.Prunable = prunable.New(dbConfig.WithDirectory(s.dir.PathWithCreate(prunableDirName)), s.optsPruningDelay, s.Settings().APIProvider(), errorHandler, s.optsPrunableManagerOptions...)
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
	return s.Prunable.Size()
}

// PermanentDatabaseSize returns the size of the underlying permanent database and files.
func (s *Storage) PermanentDatabaseSize() int64 {
	return s.Permanent.Size()
}

func (s *Storage) Size() int64 {
	return s.Permanent.Size() + s.Prunable.Size()
}

func (s *Storage) PruneByEpochIndex(index iotago.EpochIndex) {
	s.setIsPruning(true)
	defer s.setIsPruning(false)

	s.Prunable.PruneUntilEpoch(index)
}

func (s *Storage) PruneByDepth(depth iotago.EpochIndex) {
	s.setIsPruning(true)
	defer s.setIsPruning(false)

	start := s.Settings().APIProvider().CurrentAPI().TimeProvider().EpochFromSlot(s.Settings().LatestCommitment().Index())
	if start < depth {
		s.errorHandler(ierrors.Errorf("pruning depth %d is greater than the current epoch %d", depth, start))
		return
	}

	s.Prunable.PruneUntilEpoch(start - depth)
	// double check permanent Ledger
}

// This is now similar to how hornet pruneBySize, by calculating the ratio of the current size to the target size then calculate the number of epochs to prune. So it's not precise.
func (s *Storage) PruneBySize(targetSizeBytes ...int64) {
	s.setIsPruning(true)
	defer s.setIsPruning(false)

	targetDatabaseSizeBytes := s.optPruningSizeMaxTargetSizeBytes
	if len(targetSizeBytes) > 0 {
		targetDatabaseSizeBytes = targetSizeBytes[0]
	}

	currentSize := s.Size()
	if targetDatabaseSizeBytes < 0 || currentSize < targetDatabaseSizeBytes {
		return
	}

	latestEpoch := s.Settings().APIProvider().CurrentAPI().TimeProvider().EpochFromSlot(s.Settings().LatestCommitment().Index())
	lastPrunedEpoch := lo.Return1(s.LastPrunedEpoch())

	pruningRange := latestEpoch - lastPrunedEpoch
	numOfEpochToPrune := math.Ceil(float64(pruningRange) * (1.0 - float64(targetDatabaseSizeBytes/currentSize)))

	s.Prunable.PruneUntilEpoch(lastPrunedEpoch + iotago.EpochIndex(numOfEpochToPrune))
}

// Shutdown shuts down the storage.
func (s *Storage) Shutdown() {
	s.shutdownOnce.Do(func() {
		s.Permanent.Shutdown()
		s.Prunable.Shutdown()
	})
}
