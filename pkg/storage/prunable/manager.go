package prunable

import (
	"os"

	"github.com/zyedidia/generic/cache"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	iotago "github.com/iotaledger/iota.go/v4"
)

type PrunableSlotManager struct {
	openDBs      *cache.Cache[iotago.EpochIndex, *database.DBInstance]
	openDBsMutex syncutils.RWMutex

	lastPrunedEpoch *model.EvictionIndex[iotago.EpochIndex]
	lastPrunedMutex syncutils.RWMutex

	dbConfig     database.Config
	errorHandler func(error)

	dbSizes *shrinkingmap.ShrinkingMap[iotago.EpochIndex, int64]

	optsMaxOpenDBs int
}

func NewPrunableSlotManager(dbConfig database.Config, errorHandler func(error), opts ...options.Option[PrunableSlotManager]) *PrunableSlotManager {
	return options.Apply(&PrunableSlotManager{
		optsMaxOpenDBs:  10,
		dbConfig:        dbConfig,
		errorHandler:    errorHandler,
		dbSizes:         shrinkingmap.New[iotago.EpochIndex, int64](),
		lastPrunedEpoch: model.NewEvictionIndex[iotago.EpochIndex](),
	}, opts, func(m *PrunableSlotManager) {
		m.openDBs = cache.New[iotago.EpochIndex, *database.DBInstance](m.optsMaxOpenDBs)
		m.openDBs.SetEvictCallback(func(baseIndex iotago.EpochIndex, db *database.DBInstance) {
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
func (m *PrunableSlotManager) IsTooOld(index iotago.EpochIndex) (isTooOld bool) {
	m.lastPrunedMutex.RLock()
	defer m.lastPrunedMutex.RUnlock()

	return index < m.lastPrunedEpoch.NextIndex()
}

func (m *PrunableSlotManager) Get(index iotago.EpochIndex, realm kvstore.Realm) (kvstore.KVStore, error) {
	if m.IsTooOld(index) {
		return nil, ierrors.Wrapf(ErrEpochPruned, "epoch %d", index)
	}

	kv := m.getDBInstance(index).KVStore()

	return lo.PanicOnErr(kv.WithExtendedRealm(realm)), nil
}

func (m *PrunableSlotManager) PruneUntilEpoch(index iotago.EpochIndex) error {
	m.lastPrunedMutex.Lock()
	defer m.lastPrunedMutex.Unlock()

	if index < m.lastPrunedEpoch.NextIndex() {
		return ErrNoPruningNeeded
	}

	for currentIndex := m.lastPrunedEpoch.NextIndex(); currentIndex <= index; currentIndex++ {
		m.prune(currentIndex)
		m.lastPrunedEpoch.MarkEvicted(currentIndex)
	}

	return nil
}

func (m *PrunableSlotManager) Shutdown() {
	m.openDBsMutex.Lock()
	defer m.openDBsMutex.Unlock()

	m.openDBs.Each(func(index iotago.EpochIndex, db *database.DBInstance) {
		db.Close()
	})
}

// PrunableSlotStorageSize returns the size of the prunable storage containing all db instances.
func (m *PrunableSlotManager) PrunableSlotStorageSize() int64 {
	// Sum up all the evicted databases
	var sum int64
	m.dbSizes.ForEach(func(index iotago.EpochIndex, i int64) bool {
		sum += i
		return true
	})

	m.openDBsMutex.Lock()
	defer m.openDBsMutex.Unlock()

	// Add up all the open databases
	m.openDBs.Each(func(key iotago.EpochIndex, val *database.DBInstance) {
		size, err := dbPrunableDirectorySize(m.dbConfig.Directory, key)
		if err != nil {
			m.errorHandler(ierrors.Wrapf(err, "dbPrunableDirectorySize failed for %s: %s", m.dbConfig.Directory, key))
			return
		}
		sum += size
	})

	return sum
}

func (m *PrunableSlotManager) LastPrunedEpoch() (index iotago.EpochIndex, hasPruned bool) {
	m.lastPrunedMutex.RLock()
	defer m.lastPrunedMutex.RUnlock()

	return m.lastPrunedEpoch.Index()
}

func (m *PrunableSlotManager) RestoreFromDisk() {
	m.lastPrunedMutex.Lock()
	defer m.lastPrunedMutex.Unlock()

	dbInfos := getSortedDBInstancesFromDisk(m.dbConfig.Directory)

	// There are no dbInstances on disk -> nothing to restore.
	if len(dbInfos) == 0 {
		return
	}

	// Set the maxPruned epoch to the baseIndex-1 of the oldest dbInstance.
	if dbInfos[0].baseIndex > 0 {
		m.lastPrunedEpoch.MarkEvicted(dbInfos[0].baseIndex - 1)
	} else {
		m.lastPrunedEpoch.MarkEvicted(0)
	}

	// Open all the dbInstances (perform health checks) and add them to the openDBs cache. Also fills the dbSizes map (when evicted from the cache).
	for _, dbInfo := range dbInfos {
		m.getDBInstance(dbInfo.baseIndex)
	}

	return
}

// getDBInstance returns the DB instance for the given epochIndex or creates a new one if it does not yet exist.
// DBs are created as follows where each db is located in m.basedir/<starting epochIndex>/
//
//	epochIndex 0 -> db 0
//	epochIndex 1 -> db 1
//	epochIndex 2 -> db 2
func (m *PrunableSlotManager) getDBInstance(index iotago.EpochIndex) (db *database.DBInstance) {
	m.openDBsMutex.Lock()
	defer m.openDBsMutex.Unlock()

	// check if exists again, as other goroutine might have created it in parallel
	db, exists := m.openDBs.Get(index)
	if !exists {
		db = database.NewDBInstance(m.dbConfig.WithDirectory(dbPathFromIndex(m.dbConfig.Directory, index)))

		// Remove the cached db size since we will open the db
		m.dbSizes.Delete(index)
		m.openDBs.Put(index, db)
	}

	return db
}

func (m *PrunableSlotManager) prune(dbBaseIndex iotago.EpochIndex) {
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

func (m *PrunableSlotManager) BucketSize(epoch iotago.EpochIndex) (int64, error) {
	size, exists := m.dbSizes.Get(epoch)
	if exists {
		return size, nil
	}

	_, exists = m.openDBs.Get(epoch)
	if !exists {
		return 0, ierrors.Errorf("bucket does not exists: %d", epoch)
	}

	size, err := dbPrunableDirectorySize(m.dbConfig.Directory, epoch)
	if err != nil {
		return 0, ierrors.Wrapf(err, "dbPrunableDirectorySize failed for %s: %s", m.dbConfig.Directory, epoch)
	}

	return size, nil
}

func (m *PrunableSlotManager) Flush() error {
	m.openDBsMutex.RLock()
	defer m.openDBsMutex.RUnlock()

	var err error
	m.openDBs.Each(func(epoch iotago.EpochIndex, db *database.DBInstance) {
		if err = db.KVStore().Flush(); err != nil {
			return
		}
	})

	return err
}
