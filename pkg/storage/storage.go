package storage

import (
	"sync"

	hivedb "github.com/iotaledger/hive.go/kvstore/database"
	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	"github.com/iotaledger/iota-core/pkg/storage/permanent"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
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

	optsDBEngine               hivedb.Engine
	optsAllowedDBEngines       []hivedb.Engine
	optsLogger                 *logger.Logger
	optsPrunableManagerOptions []options.Option[prunable.Manager]
}

// New creates a new storage instance with the named database version in the given directory.
func New(directory string, dbVersion byte, opts ...options.Option[Storage]) *Storage {
	return options.Apply(&Storage{
		dir:          utils.NewDirectory(directory, true),
		optsDBEngine: hivedb.EngineRocksDB,
	}, opts,
		func(s *Storage) {
			s.Permanent = permanent.New(s.dir, database.Config{
				Engine:       s.optsDBEngine,
				Directory:    s.dir.PathWithCreate(permanentDirName),
				Version:      dbVersion,
				PrefixHealth: []byte{storePrefixHealth},
			})

			s.Prunable = prunable.New(database.Config{
				Engine:       s.optsDBEngine,
				Directory:    s.dir.PathWithCreate(prunableDirName),
				Version:      dbVersion,
				PrefixHealth: []byte{storePrefixHealth},
			}, s.optsPrunableManagerOptions...)

			s.Permanent.Settings().HookInitialized(func() {
				// TODO: do we make sure this is set before "running" the node?
				s.Prunable.Initialize(s.Settings().API())
			})
		})
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

// Shutdown shuts down the storage.
func (s *Storage) Shutdown() {
	s.shutdownOnce.Do(func() {
		s.Permanent.Shutdown()
		s.Prunable.Shutdown()
	})
}
