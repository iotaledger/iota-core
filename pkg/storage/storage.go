package storage

import (
	"sync"
	"time"

	"github.com/iotaledger/hive.go/db"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/clonablesql"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	"github.com/iotaledger/iota-core/pkg/storage/permanent"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	permanentDirName   = "permanent"
	prunableDirName    = "prunable"
	sqlDirName         = "sql"
	txRetainerFileName = "tx_retainer.db"

	storePrefixHealth byte = 255
)

// Storage is an abstraction around the storage layer of the node.
type Storage struct {
	Pruned *event.Event1[iotago.EpochIndex]

	dir *utils.Directory

	// Permanent is the section of the storage that is maintained forever (holds the current ledger state).
	permanent *permanent.Permanent

	// Prunable is the section of the storage that is pruned regularly (holds the history of the ledger state).
	prunable *prunable.Prunable

	// txRetainerSQL is the SQL database for the transaction retainer (holds the transaction metadata).
	txRetainerSQL *clonablesql.ClonableSQLiteDatabase

	shutdownOnce sync.Once
	errorHandler func(error)

	isPruning          bool
	statusLock         sync.RWMutex
	pruningLock        sync.RWMutex
	lastPrunedEpoch    *model.EvictionIndex[iotago.EpochIndex]
	lastPrunedSizeTime time.Time
	lastAccessedBlocks reactive.Variable[iotago.SlotIndex]

	optsDBEngine                       db.Engine
	optsAllowedDBEngines               []db.Engine
	optsPruningDelay                   iotago.EpochIndex
	optPruningSizeEnabled              bool
	optsPruningSizeMaxTargetSizeBytes  int64
	optsPruningSizeReductionPercentage float64
	optsBucketManagerOptions           []options.Option[prunable.BucketManager]
	optsPruningSizeCooldownTime        time.Duration
	optsPermanent                      []options.Option[permanent.Permanent]
}

// newStorage creates a new storage instance with the named database version in the given directory.
func newStorage(directory string, errorHandler func(error), opts ...options.Option[Storage]) *Storage {
	return options.Apply(&Storage{
		Pruned:                             event.New1[iotago.EpochIndex](),
		dir:                                utils.NewDirectory(directory, true),
		errorHandler:                       errorHandler,
		lastPrunedEpoch:                    model.NewEvictionIndex[iotago.EpochIndex](),
		lastAccessedBlocks:                 reactive.NewVariable[iotago.SlotIndex](),
		optsDBEngine:                       db.EngineRocksDB,
		optsPruningDelay:                   30,
		optPruningSizeEnabled:              false,
		optsPruningSizeMaxTargetSizeBytes:  30 * 1024 * 1024 * 1024, // 30GB
		optsPruningSizeReductionPercentage: 0.1,
		optsPruningSizeCooldownTime:        5 * time.Minute,
	}, opts)
}

// Create creates a new storage instance with the named database version in the given directory and initializes its permanent
// and prunable counterparts.
func Create(parentLogger log.Logger, directory string, dbVersion byte, errorHandler func(error), opts ...options.Option[Storage]) *Storage {
	s := newStorage(directory, errorHandler, opts...)

	dbConfig := database.Config{
		Engine:       s.optsDBEngine,
		Directory:    s.dir.PathWithCreate(permanentDirName),
		Version:      dbVersion,
		PrefixHealth: []byte{storePrefixHealth},
	}

	s.permanent = permanent.New(dbConfig, errorHandler, s.optsPermanent...)
	s.prunable = prunable.New(dbConfig.WithDirectory(s.dir.PathWithCreate(prunableDirName)), s.Settings().APIProvider(), s.errorHandler, s.optsBucketManagerOptions...)
	s.txRetainerSQL = clonablesql.NewClonableSQLiteDatabase(parentLogger.NewChildLogger("tx-retainer-db"), s.dir.PathWithCreate(sqlDirName), txRetainerFileName, s.errorHandler)

	return s
}

// Clone creates a new storage instance with the named database version in the given directory and cloning the permannent
// and prunable counterparts from the given source storage.
func Clone(parentLogger log.Logger, source *Storage, directory string, dbVersion byte, errorHandler func(error), opts ...options.Option[Storage]) (*Storage, error) {
	s := newStorage(directory, errorHandler, opts...)

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

	txRetainerSQLClone, err := clonablesql.Clone(parentLogger.NewChildLogger("tx-retainer-db"), source.txRetainerSQL, s.dir.PathWithCreate(sqlDirName), txRetainerFileName, s.errorHandler)
	if err != nil {
		return nil, ierrors.Wrap(err, "error while cloning transaction retainer SQL storage")
	}

	s.permanent = permanentClone
	s.prunable = prunableClone
	s.txRetainerSQL = txRetainerSQLClone

	return s, nil
}

func (s *Storage) Directory() string {
	return s.dir.Path()
}

// PermanentDatabaseSize returns the size of the underlying permanent database and files.
func (s *Storage) PermanentDatabaseSize() int64 {
	return s.permanent.Size()
}

// PrunableDatabaseSize returns the size of the underlying prunable databases.
func (s *Storage) PrunableDatabaseSize() int64 {
	return s.prunable.Size()
}

// TransactionRetainerDatabaseSize returns the size of the underlying SQL database of the transaction retainer.
func (s *Storage) TransactionRetainerDatabaseSize() int64 {
	return s.txRetainerSQL.Size()
}

func (s *Storage) Size() int64 {
	return s.PermanentDatabaseSize() + s.PrunableDatabaseSize() + s.TransactionRetainerDatabaseSize()
}

func (s *Storage) RestoreFromDisk() {
	s.pruningLock.Lock()
	defer s.pruningLock.Unlock()

	lastPrunedEpoch := s.prunable.RestoreFromDisk()
	// Return if it's epoch 0, leave the lastPrunedEpoch at the default value, indicating epoch 0 is not pruned yet.
	if lastPrunedEpoch == 0 {
		return
	}

	s.lastPrunedEpoch.MarkEvicted(lastPrunedEpoch)
}

func (s *Storage) Rollback(targetSlot iotago.SlotIndex) error {
	return s.prunable.Rollback(s.pruningRange(targetSlot))
}

// Shutdown shuts down the storage.
func (s *Storage) Shutdown() {
	s.shutdownOnce.Do(func() {
		s.permanent.Shutdown()
		s.prunable.Shutdown()
		s.txRetainerSQL.Shutdown()
	})
}

func (s *Storage) Flush() {
	s.permanent.Flush()
	s.prunable.Flush()
}
