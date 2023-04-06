package database

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
	"github.com/zyedidia/generic/cache"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/kvstore"
	hivedb "github.com/iotaledger/hive.go/kvstore/database"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/hive.go/runtime/options"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Manager struct {
	permanentStorage kvstore.KVStore
	permanentBaseDir string

	openDBs         *cache.Cache[iotago.SlotIndex, *dbInstance]
	bucketedBaseDir string
	dbSizes         *shrinkingmap.ShrinkingMap[iotago.SlotIndex, int64]
	openDBsMutex    sync.Mutex

	maxPruned      iotago.SlotIndex
	maxPrunedMutex sync.RWMutex

	logger *logger.WrappedLogger

	// The granularity of the DB instances (i.e. how many buckets/slots are stored in one DB).
	optsGranularity int64
	optsBaseDir     string
	optsMaxOpenDBs  int

	optsDBEngine         hivedb.Engine
	optsAllowedDBEngines []hivedb.Engine
	optsLogger           *logger.Logger
}

func NewManager(version Version, opts ...options.Option[Manager]) *Manager {
	m := options.Apply(&Manager{
		maxPruned:       -1,
		optsGranularity: 10,
		optsBaseDir:     "db",
		optsMaxOpenDBs:  10,
		optsDBEngine:    hivedb.EngineMapDB,
	}, opts, func(m *Manager) {
		m.logger = logger.NewWrappedLogger(m.optsLogger)
		m.bucketedBaseDir = filepath.Join(m.optsBaseDir, "pruned")
		m.permanentBaseDir = filepath.Join(m.optsBaseDir, "permanent")

		permanentStore, err := StoreWithDefaultSettings(m.permanentBaseDir, true, m.optsDBEngine)
		if err != nil {
			panic(err)
		}
		m.permanentStorage = permanentStore

		m.openDBs = cache.New[iotago.SlotIndex, *dbInstance](m.optsMaxOpenDBs)
		m.openDBs.SetEvictCallback(func(baseIndex iotago.SlotIndex, db *dbInstance) {
			err := db.store.Close()
			if err != nil {
				panic(err)
			}
			size, err := dbPrunableDirectorySize(m.bucketedBaseDir, baseIndex)
			if err != nil {
				m.logger.LogError("failed to get size of prunable directory for base index %d: %w", baseIndex, err)
			}

			m.dbSizes.Set(baseIndex, size)
		})

		m.dbSizes = shrinkingmap.New[iotago.SlotIndex, int64]()
	})

	if err := m.checkVersion(version); err != nil {
		panic(err)
	}

	return m
}

// checkVersion checks whether the database is compatible with the current schema version.
// also automatically sets the version if the database is new.
func (m *Manager) checkVersion(version Version) error {
	entry, err := m.permanentStorage.Get(dbVersionKey)
	if errors.Is(err, kvstore.ErrKeyNotFound) {
		// set the version in an empty DB
		return m.permanentStorage.Set(dbVersionKey, lo.PanicOnErr(version.Bytes()))
	}
	if err != nil {
		return err
	}
	if len(entry) == 0 {
		return errors.Errorf("no database version was persisted")
	}
	var storedVersion Version
	if _, err := storedVersion.FromBytes(entry); err != nil {
		return err
	}
	if storedVersion != version {
		return errors.Errorf("incompatible database versions: supported version: %d, version of database: %d", version, storedVersion)
	}
	return nil
}

func (m *Manager) PermanentStorage() kvstore.KVStore {
	return m.permanentStorage
}

func (m *Manager) RestoreFromDisk() (latestBucketIndex iotago.SlotIndex) {
	dbInfos := getSortedDBInstancesFromDisk(m.bucketedBaseDir)

	// TODO: what to do if dbInfos is empty? -> start with a fresh DB?

	for _, dbInfo := range dbInfos {
		size, err := dbPrunableDirectorySize(m.bucketedBaseDir, dbInfo.baseIndex)
		if err != nil {
			panic(err)
		}
		m.dbSizes.Set(dbInfo.baseIndex, size)
	}

	m.maxPrunedMutex.Lock()
	m.maxPruned = dbInfos[len(dbInfos)-1].baseIndex - 1
	m.maxPrunedMutex.Unlock()

	for _, dbInfo := range dbInfos {
		dbIndex := dbInfo.baseIndex
		var healthy bool
		for bucketIndex := dbIndex + iotago.SlotIndex(m.optsGranularity) - 1; bucketIndex >= dbIndex; bucketIndex-- {
			bucket := m.getBucket(bucketIndex)
			healthy = lo.PanicOnErr(bucket.Has(healthKey))
			if healthy {
				return bucketIndex
			}

			m.removeBucket(bucket)
		}
		m.removeDBInstance(dbIndex)
	}

	return 0
}

func (m *Manager) MaxPrunedSlot() iotago.SlotIndex {
	m.maxPrunedMutex.RLock()
	defer m.maxPrunedMutex.RUnlock()

	return m.maxPruned
}

// IsTooOld checks if the Block associated with the given id is too old (in a pruned slot).
func (m *Manager) IsTooOld(index iotago.SlotIndex) (isTooOld bool) {
	m.maxPrunedMutex.RLock()
	defer m.maxPrunedMutex.RUnlock()

	return index <= m.maxPruned
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

func (m *Manager) Flush(index iotago.SlotIndex) {
	// Flushing works on DB level
	db := m.getDBInstance(index)
	err := db.store.Flush()
	if err != nil {
		panic(err)
	}

	// Mark as healthy.
	bucket := m.getBucket(index)
	err = bucket.Set(healthKey, []byte{1})
	if err != nil {
		panic(err)
	}
}

func (m *Manager) PruneUntilSlot(index iotago.SlotIndex) {
	var baseIndexToPrune iotago.SlotIndex
	if m.computeDBBaseIndex(index)+iotago.SlotIndex(m.optsGranularity)-1 == index {
		// Upper bound of the DB instance should be pruned. So we can delete the entire DB file.
		baseIndexToPrune = index
	} else {
		baseIndexToPrune = m.computeDBBaseIndex(index) - 1
	}

	currentPrunedIndex := m.setMaxPruned(baseIndexToPrune)
	for currentPrunedIndex+iotago.SlotIndex(m.optsGranularity) <= baseIndexToPrune {
		currentPrunedIndex += iotago.SlotIndex(m.optsGranularity)
		m.prune(currentPrunedIndex)
	}
}

func (m *Manager) setMaxPruned(index iotago.SlotIndex) (previous iotago.SlotIndex) {
	m.maxPrunedMutex.Lock()
	defer m.maxPrunedMutex.Unlock()

	if previous = m.maxPruned; previous >= index {
		return
	}

	m.maxPruned = index
	return
}

func (m *Manager) Shutdown() {
	m.openDBsMutex.Lock()
	defer m.openDBsMutex.Unlock()

	m.openDBs.Each(func(index iotago.SlotIndex, db *dbInstance) {
		err := db.store.Close()
		if err != nil {
			panic(err)
		}
	})

	err := m.permanentStorage.Close()
	if err != nil {
		panic(err)
	}
}

// TotalStorageSize returns the combined size of the permanent and prunable storage.
func (m *Manager) TotalStorageSize() int64 {
	return m.PermanentStorageSize() + m.PrunableStorageSize()
}

// PermanentStorageSize returns the size of the permanent storage.
func (m *Manager) PermanentStorageSize() int64 {
	size, err := dbDirectorySize(m.permanentBaseDir)
	if err != nil {
		m.logger.LogError("dbDirectorySize failed for %s: %w", m.permanentBaseDir, err)
		return 0
	}
	return size
}

// PrunableStorageSize returns the size of the prunable storage containing all db instances.
func (m *Manager) PrunableStorageSize() int64 {
	m.openDBsMutex.Lock()
	defer m.openDBsMutex.Unlock()

	// Sum up all the evicted databases
	var sum int64
	m.dbSizes.ForEach(func(index iotago.SlotIndex, i int64) bool {
		sum += i
		return true
	})

	// Add up all the open databases
	m.openDBs.Each(func(key iotago.SlotIndex, val *dbInstance) {
		size, err := dbPrunableDirectorySize(m.bucketedBaseDir, key)
		if err != nil {
			m.logger.LogError("dbPrunableDirectorySize failed for %s%s: %w", m.bucketedBaseDir, key, err)
			return
		}
		sum += size
	})

	return sum
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
		db = m.createDBInstance(baseIndex)

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

// createDBInstance creates a new DB instance for the given baseIndex.
// If a folder/DB for the given baseIndex already exists, it is opened.
func (m *Manager) createDBInstance(index iotago.SlotIndex) (newDBInstance *dbInstance) {
	db, err := StoreWithDefaultSettings(dbPathFromIndex(m.bucketedBaseDir, index), true, m.optsDBEngine)
	if err != nil {
		panic(err)
	}

	return &dbInstance{
		index: index,
		store: db,
	}
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
		err := db.store.Close()
		if err != nil {
			panic(err)
		}
		m.openDBs.Remove(dbBaseIndex)
	}

	if err := os.RemoveAll(dbPathFromIndex(m.bucketedBaseDir, dbBaseIndex)); err != nil {
		panic(err)
	}

	// Delete the db size since we pruned the whole directory
	m.dbSizes.Delete(dbBaseIndex)
}

func (m *Manager) removeBucket(bucket kvstore.KVStore) {
	err := bucket.Clear()
	if err != nil {
		panic(err)
	}
}

type dbInstance struct {
	index iotago.SlotIndex
	store kvstore.KVStore // KVStore that is used to access the DB instance
}
