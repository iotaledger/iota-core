package prunable

import (
	"os"
	"sync"

	"github.com/zyedidia/generic/cache"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Manager struct {
	openDBs      *cache.Cache[iotago.SlotIndex, *dbInstance]
	openDBsMutex sync.Mutex

	lastPrunedSlot *model.EvictionIndex
	pruningMutex   sync.RWMutex

	dbConfig database.Config

	dbSizes *shrinkingmap.ShrinkingMap[iotago.SlotIndex, int64]

	optsLogger *logger.Logger
	logger     *logger.WrappedLogger

	// The granularity of the DB instances (i.e. how many buckets/slots are stored in one DB).
	optsGranularity int64
	optsMaxOpenDBs  int
}

func NewManager(dbConfig database.Config, opts ...options.Option[Manager]) *Manager {
	return options.Apply(&Manager{
		optsGranularity: 10,
		optsMaxOpenDBs:  10,
		dbConfig:        dbConfig,
		dbSizes:         shrinkingmap.New[iotago.SlotIndex, int64](),
		lastPrunedSlot:  model.NewEvictionIndex(),
	}, opts, func(m *Manager) {
		m.logger = logger.NewWrappedLogger(m.optsLogger)

		m.openDBs = cache.New[iotago.SlotIndex, *dbInstance](m.optsMaxOpenDBs)
		m.openDBs.SetEvictCallback(func(baseIndex iotago.SlotIndex, db *dbInstance) {
			db.Close()

			size, err := dbPrunableDirectorySize(dbConfig.Directory, baseIndex)
			if err != nil {
				m.logger.LogError("failed to get size of prunable directory for base index %d: %w", baseIndex, err)
			}

			m.dbSizes.Set(baseIndex, size)
		})
	}, (*Manager).restoreFromDisk)
}

// IsTooOld checks if the index is in a pruned slot.
func (m *Manager) IsTooOld(index iotago.SlotIndex) (isTooOld bool) {
	m.pruningMutex.RLock()
	defer m.pruningMutex.RUnlock()

	return index < m.lastPrunedSlot.NextIndex()
}

func (m *Manager) Get(index iotago.SlotIndex, realm kvstore.Realm) kvstore.KVStore {
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

func (m *Manager) PruneUntilSlot(index iotago.SlotIndex) {
	m.pruningMutex.Lock()
	defer m.pruningMutex.Unlock()

	computedIndex := m.computeDBBaseIndex(index)
	var baseIndexToPrune iotago.SlotIndex

	if computedIndex+iotago.SlotIndex(m.optsGranularity)-1 == index {
		baseIndexToPrune = index
	} else if computedIndex > 1 {
		baseIndexToPrune = computedIndex - 1
	} else {
		return
	}

	for currentIndex := m.lastPrunedSlot.NextIndex(); currentIndex <= baseIndexToPrune; currentIndex += iotago.SlotIndex(m.optsGranularity) {
		m.prune(currentIndex)
		m.lastPrunedSlot.MarkEvicted(currentIndex)
	}
}

func (m *Manager) Shutdown() {
	m.openDBsMutex.Lock()
	defer m.openDBsMutex.Unlock()

	m.openDBs.Each(func(index iotago.SlotIndex, db *dbInstance) {
		db.Close()
	})
}

func (m *Manager) LastPrunedSlot() (index iotago.SlotIndex, hasPruned bool) {
	m.pruningMutex.RLock()
	defer m.pruningMutex.RUnlock()

	return m.lastPrunedSlot.Index()
}

// PrunableStorageSize returns the size of the prunable storage containing all db instances.
func (m *Manager) PrunableStorageSize() int64 {
	// Sum up all the evicted databases
	var sum int64
	m.dbSizes.ForEach(func(index iotago.SlotIndex, i int64) bool {
		sum += i
		return true
	})

	m.openDBsMutex.Lock()
	defer m.openDBsMutex.Unlock()

	// Add up all the open databases
	m.openDBs.Each(func(key iotago.SlotIndex, val *dbInstance) {
		size, err := dbPrunableDirectorySize(m.dbConfig.Directory, key)
		if err != nil {
			m.logger.LogError("dbPrunableDirectorySize failed for %s%s: %w", m.dbConfig.Directory, key, err)
			return
		}
		sum += size
	})

	return sum
}

func (m *Manager) restoreFromDisk() {
	dbInfos := getSortedDBInstancesFromDisk(m.dbConfig.Directory)

	// There are no dbInstances on disk -> nothing to restore.
	if len(dbInfos) == 0 {
		return
	}

	// Set the maxPruned slot to the baseIndex-1 of the oldest dbInstance.
	m.pruningMutex.Lock()
	m.lastPrunedSlot.MarkEvicted(dbInfos[0].baseIndex - 1)
	m.pruningMutex.Unlock()

	// Open all the dbInstances (perform health checks) and add them to the openDBs cache. Also fills the dbSizes map (when evicted from the cache).
	for _, dbInfo := range dbInfos {
		m.getDBInstance(dbInfo.baseIndex)
	}
}

// getDBInstance returns the DB instance for the given baseIndex or creates a new one if it does not yet exist.
// DBs are created as follows where each db is located in m.basedir/<starting baseIndex>/
// (assuming a bucket granularity=2):
//
//	baseIndex 0 -> db 0
//	baseIndex 1 -> db 0
//	baseIndex 2 -> db 2
func (m *Manager) getDBInstance(index iotago.SlotIndex) (db *dbInstance) {
	baseIndex := m.computeDBBaseIndex(index)

	m.openDBsMutex.Lock()
	defer m.openDBsMutex.Unlock()

	// check if exists again, as other goroutine might have created it in parallel
	db, exists := m.openDBs.Get(baseIndex)
	if !exists {
		db = newDBInstance(baseIndex, m.dbConfig.WithDirectory(dbPathFromIndex(m.dbConfig.Directory, baseIndex)))

		// Remove the cached db size since we will open the db
		m.dbSizes.Delete(baseIndex)
		m.openDBs.Put(baseIndex, db)
	}

	return db
}

// getBucket returns the bucket for the given baseIndex or creates a new one if it does not yet exist.
// A bucket is marked as dirty by default.
// Buckets are created as follows (assuming a bucket granularity=2):
//
//	baseIndex 0 -> db 0 / bucket 0
//	baseIndex 1 -> db 0 / bucket 1
//	baseIndex 2 -> db 2 / bucket 2
//	baseIndex 3 -> db 2 / bucket 3
func (m *Manager) getBucket(index iotago.SlotIndex) (bucket kvstore.KVStore) {
	_, bucket = m.getDBAndBucket(index)
	return bucket
}

func (m *Manager) getDBAndBucket(index iotago.SlotIndex) (db *dbInstance, bucket kvstore.KVStore) {
	db = m.getDBInstance(index)
	return db, m.createBucket(db, index)
}

// createBucket creates a new bucket for the given baseIndex. It uses the baseIndex as a realm on the underlying DB.
func (m *Manager) createBucket(db *dbInstance, index iotago.SlotIndex) (bucket kvstore.KVStore) {
	bucket, err := db.store.WithExtendedRealm(indexToRealm(index))
	if err != nil {
		panic(err)
	}

	return bucket
}

func (m *Manager) computeDBBaseIndex(index iotago.SlotIndex) iotago.SlotIndex {
	return index / iotago.SlotIndex(m.optsGranularity) * iotago.SlotIndex(m.optsGranularity)
}

func (m *Manager) prune(index iotago.SlotIndex) {
	dbBaseIndex := m.computeDBBaseIndex(index)
	m.removeDBInstance(dbBaseIndex)
}

func (m *Manager) removeDBInstance(dbBaseIndex iotago.SlotIndex) {
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
