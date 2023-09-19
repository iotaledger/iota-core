package storage

import (
	"sync"
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	hivedb "github.com/iotaledger/hive.go/kvstore/database"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
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
	permanent *permanent.Permanent

	// Prunable is the section of the storage that is pruned regularly (holds the history of the ledger state).
	prunable *prunable.Prunable

	shutdownOnce sync.Once
	errorHandler func(error)

	isPruning          bool
	statusLock         sync.RWMutex
	pruningLock        sync.RWMutex
	lastPrunedEpoch    *model.EvictionIndex[iotago.EpochIndex]
	lastPrunedSizeTime time.Time

	optsDBEngine                       hivedb.Engine
	optsAllowedDBEngines               []hivedb.Engine
	optsPruningDelay                   iotago.EpochIndex
	optPruningSizeEnabled              bool
	optsPruningSizeMaxTargetSizeBytes  int64
	optsPruningSizeReductionPercentage float64
	optsBucketManagerOptions           []options.Option[prunable.BucketManager]
	optsPruningSizeCooldownTime        time.Duration
	optsPermanent                      []options.Option[permanent.Permanent]
}

// New creates a new storage instance with the named database version in the given directory.
func New(directory string, errorHandler func(error), opts ...options.Option[Storage]) *Storage {
	return options.Apply(&Storage{
		dir:                                utils.NewDirectory(directory, true),
		errorHandler:                       errorHandler,
		lastPrunedEpoch:                    model.NewEvictionIndex[iotago.EpochIndex](),
		optsDBEngine:                       hivedb.EngineRocksDB,
		optsPruningDelay:                   30,
		optPruningSizeEnabled:              false,
		optsPruningSizeMaxTargetSizeBytes:  30 * 1024 * 1024 * 1024, // 30GB
		optsPruningSizeReductionPercentage: 0.1,
		optsPruningSizeCooldownTime:        5 * time.Minute,
	}, opts)
}

// Create creates a new storage instance with the named database version in the given directory and initializes its permanent
// and prunable counterparts.
func Create(directory string, dbVersion byte, errorHandler func(error), opts ...options.Option[Storage]) *Storage {
	s := New(directory, errorHandler, opts...)
	dbConfig := database.Config{
		Engine:       s.optsDBEngine,
		Directory:    s.dir.PathWithCreate(permanentDirName),
		Version:      dbVersion,
		PrefixHealth: []byte{storePrefixHealth},
	}

	s.permanent = permanent.New(dbConfig, errorHandler, s.optsPermanent...)
	s.prunable = prunable.New(dbConfig.WithDirectory(s.dir.PathWithCreate(prunableDirName)), s.Settings().APIProvider(), s.errorHandler, s.optsBucketManagerOptions...)

	return s
}

// Clone creates a new storage instance with the named database version in the given directory and cloning the permannent
// and prunable counterparts from the given source storage.
func Clone(source *Storage, directory string, dbVersion byte, errorHandler func(error), opts ...options.Option[Storage]) (*Storage, error) {
	s := New(directory, errorHandler, opts...)

	dbConfig := database.Config{
		Engine:       s.optsDBEngine,
		Directory:    s.dir.PathWithCreate(permanentDirName),
		Version:      dbVersion,
		PrefixHealth: []byte{storePrefixHealth},
	}

	permanentClone, err := permanent.Clone(source.permanent, dbConfig, errorHandler)
	if err != nil {
		return nil, ierrors.Wrap(err, "error while cloning permanent storage")
	}
	prunableClone, err := prunable.Clone(source.prunable, dbConfig.WithDirectory(s.dir.PathWithCreate(prunableDirName)), permanentClone.Settings().APIProvider(), s.errorHandler, s.optsBucketManagerOptions...)
	if err != nil {
		return nil, ierrors.Wrap(err, "error while cloning prunable storage")
	}

	s.permanent = permanentClone
	s.prunable = prunableClone

	return s, nil
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
