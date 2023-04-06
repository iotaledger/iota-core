package storage

import (
	"sync"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/database"
	"github.com/iotaledger/iota-core/pkg/storage/permanent"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Storage is an abstraction around the storage layer of the node.
type Storage struct {
	// Permanent is the section of the storage that is maintained forever (holds the current ledger state).
	*permanent.Permanent

	// Prunable is the section of the storage that is pruned regularly (holds the history of the ledger state).
	*prunable.Prunable

	// databaseManager is the database manager.
	databaseManager *database.Manager

	shutdownOnce sync.Once

	Directory string
}

// New creates a new storage instance with the named database version in the given directory.
func New(directory string, version database.Version, opts ...options.Option[database.Manager]) (newStorage *Storage) {
	databaseManager := database.NewManager(version, append(opts, database.WithBaseDir(directory))...)

	newStorage = &Storage{
		Permanent: permanent.New(utils.NewDirectory(directory, true), databaseManager),
		Prunable:  prunable.New(databaseManager),

		databaseManager: databaseManager,
		Directory:       directory,
	}

	if err := newStorage.Commitments.Store(iotago.NewEmptyCommitment()); err != nil {
		panic(err)
	}

	return newStorage
}

// PruneUntilSlot prunes storage slots less than and equal to the given index.
func (s *Storage) PruneUntilSlot(index iotago.SlotIndex) {
	s.databaseManager.PruneUntilSlot(index)
}

// PrunableDatabaseSize returns the size of the underlying prunable databases.
func (s *Storage) PrunableDatabaseSize() int64 {
	return s.databaseManager.PrunableStorageSize()
}

// PermanentDatabaseSize returns the size of the underlying permanent database and files.
func (s *Storage) PermanentDatabaseSize() int64 {
	return s.Permanent.SettingsAndCommitmentsSize() + s.databaseManager.PermanentStorageSize()
}

// Shutdown shuts down the storage.
func (s *Storage) Shutdown() {
	s.shutdownOnce.Do(func() {
		if err := s.Permanent.Commitments.Close(); err != nil {
			panic(err)
		}

		s.databaseManager.Shutdown()
	})
}
