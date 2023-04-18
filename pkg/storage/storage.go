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
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	permanentDirName = "permanent"
	prunableDirName  = "prunable"
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
func New(directory string, version database.Version, opts ...options.Option[Storage]) *Storage {
	return options.Apply(&Storage{
		dir:          utils.NewDirectory(directory, true),
		optsDBEngine: hivedb.EngineRocksDB,
	}, opts,
		func(s *Storage) {
			// TODO: create different folders for settings and stuff
			s.Permanent = permanent.New(utils.NewDirectory(s.dir.Path(permanentDirName), true), version, s.optsDBEngine)
			s.Prunable = prunable.New(utils.NewDirectory(s.dir.Path(prunableDirName), true), version, s.optsDBEngine)

			// TODO: this should only be done once no?
			if err := s.Commitments.Store(iotago.NewEmptyCommitment()); err != nil {
				panic(err)
			}

		})
}

func (s *Storage) Directory() string {
	return s.dir.Path()
}

// PruneUntilSlot prunes storage slots less than and equal to the given index.
func (s *Storage) PruneUntilSlot(index iotago.SlotIndex) {
	// s.databaseManager.PruneUntilSlot(index)
}

// PrunableDatabaseSize returns the size of the underlying prunable databases.
func (s *Storage) PrunableDatabaseSize() int64 {
	// return s.databaseManager.PrunableStorageSize()
	return 0
}

// PermanentDatabaseSize returns the size of the underlying permanent database and files.
func (s *Storage) PermanentDatabaseSize() int64 {
	return s.Permanent.Size()
}

// Shutdown shuts down the storage.
func (s *Storage) Shutdown() {
	s.shutdownOnce.Do(func() {
		s.Permanent.Shutdown()
		s.Prunable.Shutdown()
	})
}
