package prunable

import (
	"os"

	"github.com/zyedidia/generic/cache"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Manager struct {
	openDBs      *cache.Cache[iotago.EpochIndex, *dbInstance]
	openDBsMutex syncutils.Mutex

	lastPrunedEpoch *model.EvictionIndex[iotago.EpochIndex]
	pruningMutex    syncutils.RWMutex

	dbConfig     database.Config
	errorHandler func(error)

	dbSizes *shrinkingmap.ShrinkingMap[iotago.EpochIndex, int64]

	optsMaxOpenDBs int
}

func NewManager(dbConfig database.Config, errorHandler func(error), opts ...options.Option[Manager]) *Manager {
	return options.Apply(&Manager{
		optsMaxOpenDBs:  10,
		dbConfig:        dbConfig,
		errorHandler:    errorHandler,
		dbSizes:         shrinkingmap.New[iotago.EpochIndex, int64](),
		lastPrunedEpoch: model.NewEvictionIndex[iotago.EpochIndex](),
	}, opts, func(m *Manager) {
		m.openDBs = cache.New[iotago.EpochIndex, *dbInstance](m.optsMaxOpenDBs)
		m.openDBs.SetEvictCallback(func(baseIndex iotago.EpochIndex, db *dbInstance) {
			db.Close()

			size, err := dbPrunableDirectorySize(dbConfig.Directory, baseIndex)
			if err != nil {
				errorHandler(ierrors.Wrapf(err, "failed to get size of prunable directory for base index %d", baseIndex))
			}

			m.dbSizes.Set(baseIndex, size)
		})
	})
}

// IsTooOld checks if the index is in a pruned epoch.
func (m *Manager) IsTooOld(index iotago.EpochIndex) (isTooOld bool) {
	m.pruningMutex.RLock()
	defer m.pruningMutex.RUnlock()

	return index < m.lastPrunedEpoch.NextIndex()
}

func (m *Manager) Get(index iotago.EpochIndex, realm kvstore.Realm) kvstore.KVStore {
	if m.IsTooOld(index) {
		return nil
	}

	bucket := m.getBucket(index)
	withRealm, err := bucket.WithExtendedRealm(realm)
	if err != nil {
		panic(err)
	}

	return withRealm
}

func (m *Manager) PruneUntilEpoch(index iotago.EpochIndex) {
	m.pruningMutex.Lock()
	defer m.pruningMutex.Unlock()

	if index < m.lastPrunedEpoch.NextIndex() {
		return
	}

	for currentIndex := m.lastPrunedEpoch.NextIndex(); currentIndex <= index; currentIndex++ {
		m.prune(currentIndex)
		m.lastPrunedEpoch.MarkEvicted(currentIndex)
	}
}

func (m *Manager) Shutdown() {
	m.openDBsMutex.Lock()
	defer m.openDBsMutex.Unlock()

	m.openDBs.Each(func(index iotago.EpochIndex, db *dbInstance) {
		db.Close()
	})
}

func (m *Manager) LastPrunedEpoch() (index iotago.EpochIndex, hasPruned bool) {
	m.pruningMutex.RLock()
	defer m.pruningMutex.RUnlock()

	return m.lastPrunedEpoch.Index()
}

// PrunableStorageSize returns the size of the prunable storage containing all db instances.
func (m *Manager) PrunableStorageSize() int64 {
	// Sum up all the evicted databases
	var sum int64
	m.dbSizes.ForEach(func(index iotago.EpochIndex, i int64) bool {
		sum += i
		return true
	})

	m.openDBsMutex.Lock()
	defer m.openDBsMutex.Unlock()

	// Add up all the open databases
	m.openDBs.Each(func(key iotago.EpochIndex, val *dbInstance) {
		size, err := dbPrunableDirectorySize(m.dbConfig.Directory, key)
		if err != nil {
			m.errorHandler(ierrors.Wrapf(err, "dbPrunableDirectorySize failed for %s: %s", m.dbConfig.Directory, key))
			return
		}
		sum += size
	})

	return sum
}

func (m *Manager) RestoreFromDisk() {
	dbInfos := getSortedDBInstancesFromDisk(m.dbConfig.Directory)

	// There are no dbInstances on disk -> nothing to restore.
	if len(dbInfos) == 0 {
		return
	}

	// Set the maxPruned epoch to the baseIndex-1 of the oldest dbInstance.
	m.pruningMutex.Lock()
	if dbInfos[0].baseIndex > 0 {
		m.lastPrunedEpoch.MarkEvicted(dbInfos[0].baseIndex - 1)
	} else {
		m.lastPrunedEpoch.MarkEvicted(0)
	}
	m.pruningMutex.Unlock()

	// Open all the dbInstances (perform health checks) and add them to the openDBs cache. Also fills the dbSizes map (when evicted from the cache).
	for _, dbInfo := range dbInfos {
		m.getDBInstance(dbInfo.baseIndex)
	}
}

// getDBInstance returns the DB instance for the given epochIndex or creates a new one if it does not yet exist.
// DBs are created as follows where each db is located in m.basedir/<starting epochIndex>/
//
//	epochIndex 0 -> db 0
//	epochIndex 1 -> db 1
//	epochIndex 2 -> db 2
func (m *Manager) getDBInstance(index iotago.EpochIndex) (db *dbInstance) {
	m.openDBsMutex.Lock()
	defer m.openDBsMutex.Unlock()

	// check if exists again, as other goroutine might have created it in parallel
	db, exists := m.openDBs.Get(index)
	if !exists {
		db = newDBInstance(index, m.dbConfig.WithDirectory(dbPathFromIndex(m.dbConfig.Directory, index)))

		// Remove the cached db size since we will open the db
		m.dbSizes.Delete(index)
		m.openDBs.Put(index, db)
	}

	return db
}

// getBucket returns the bucket for the given epochIndex or creates a new one if it does not yet exist.
// A bucket is marked as dirty by default.
// Buckets are created as follows:
//
//	epochIndex 0 -> db 0 / bucket 0
//	epochIndex 1 -> db 1 / bucket 1
//	epochIndex 2 -> db 2 / bucket 2
//	epochIndex 3 -> db 3 / bucket 3
func (m *Manager) getBucket(index iotago.EpochIndex) (bucket kvstore.KVStore) {
	db := m.getDBInstance(index)
	return db.store
}

func (m *Manager) prune(dbBaseIndex iotago.EpochIndex) {
	m.openDBsMutex.Lock()
	defer m.openDBsMutex.Unlock()

	db, exists := m.openDBs.Get(dbBaseIndex)
	if exists {
		db.Close()

		m.openDBs.Remove(dbBaseIndex)
	}

	if err := os.RemoveAll(dbPathFromIndex(m.dbConfig.Directory, dbBaseIndex)); err != nil {
		panic(err)
	}

	// Delete the db size since we pruned the whole directory
	m.dbSizes.Delete(dbBaseIndex)
}
