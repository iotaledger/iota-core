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

type BucketManager struct {
	openDBs      *cache.Cache[iotago.EpochIndex, *database.DBInstance]
	openDBsMutex syncutils.RWMutex

	lastPrunedEpoch *model.EvictionIndex[iotago.EpochIndex]
	lastPrunedMutex syncutils.RWMutex

	dbConfig     database.Config
	errorHandler func(error)

	dbSizes *shrinkingmap.ShrinkingMap[iotago.EpochIndex, int64]

	optsMaxOpenDBs int

	mutex syncutils.RWMutex
}

func NewBucketManager(dbConfig database.Config, errorHandler func(error), opts ...options.Option[BucketManager]) *BucketManager {
	return options.Apply(&BucketManager{
		optsMaxOpenDBs:  5,
		dbConfig:        dbConfig,
		errorHandler:    errorHandler,
		dbSizes:         shrinkingmap.New[iotago.EpochIndex, int64](),
		lastPrunedEpoch: model.NewEvictionIndex[iotago.EpochIndex](),
	}, opts, func(m *BucketManager) {
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
func (b *BucketManager) IsTooOld(index iotago.EpochIndex) (isTooOld bool) {
	b.lastPrunedMutex.RLock()
	defer b.lastPrunedMutex.RUnlock()

	return index < b.lastPrunedEpoch.NextIndex()
}

func (b *BucketManager) Get(index iotago.EpochIndex, realm kvstore.Realm) (kvstore.KVStore, error) {
	if b.IsTooOld(index) {
		return nil, ierrors.Wrapf(database.ErrEpochPruned, "epoch %d", index)
	}

	kv := b.getDBInstance(index).KVStore()

	return lo.PanicOnErr(kv.WithExtendedRealm(realm)), nil
}

func (b *BucketManager) Shutdown() {
	b.openDBsMutex.Lock()
	defer b.openDBsMutex.Unlock()

	b.openDBs.Each(func(index iotago.EpochIndex, db *database.DBInstance) {
		db.Close()
		b.openDBs.Remove(index)
	})
}

// TotalSize returns the size of the prunable storage containing all db instances.
func (b *BucketManager) TotalSize() int64 {
	// Sum up all the evicted databases
	var sum int64
	b.dbSizes.ForEach(func(index iotago.EpochIndex, i int64) bool {
		sum += i
		return true
	})

	b.openDBsMutex.Lock()
	defer b.openDBsMutex.Unlock()

	// Add up all the open databases
	b.openDBs.Each(func(key iotago.EpochIndex, val *database.DBInstance) {
		size, err := dbPrunableDirectorySize(b.dbConfig.Directory, key)
		if err != nil {
			b.errorHandler(ierrors.Wrapf(err, "dbPrunableDirectorySize failed for key %s: %s", b.dbConfig.Directory, key))

			return
		}
		sum += size
	})

	return sum
}

func (b *BucketManager) BucketSize(epoch iotago.EpochIndex) (int64, error) {
	b.openDBsMutex.RLock()
	defer b.openDBsMutex.RUnlock()

	size, exists := b.dbSizes.Get(epoch)
	if exists {
		return size, nil
	}

	_, exists = b.openDBs.Get(epoch)
	if !exists {
		return 0, nil
		// TODO: this should be fixed by https://github.com/iotaledger/iota.go/pull/480
		//  return 0, ierrors.Errorf("bucket does not exists: %d", epoch)
	}

	size, err := dbPrunableDirectorySize(b.dbConfig.Directory, epoch)
	if err != nil {
		return 0, ierrors.Wrapf(err, "dbPrunableDirectorySize failed for epoch %s: %s", b.dbConfig.Directory, epoch)
	}

	return size, nil
}

func (b *BucketManager) LastPrunedEpoch() (index iotago.EpochIndex, hasPruned bool) {
	b.lastPrunedMutex.RLock()
	defer b.lastPrunedMutex.RUnlock()

	return b.lastPrunedEpoch.Index()
}

func (b *BucketManager) RestoreFromDisk() (lastPrunedEpoch iotago.EpochIndex) {
	b.lastPrunedMutex.Lock()
	defer b.lastPrunedMutex.Unlock()

	dbInfos := getSortedDBInstancesFromDisk(b.dbConfig.Directory)

	// There are no dbInstances on disk -> nothing to restore.
	if len(dbInfos) == 0 {
		return
	}

	// Set the maxPruned epoch to the baseIndex-1 of the oldest dbInstance.
	// Leave the lastPrunedEpoch at the default value if the oldest dbInstance is at baseIndex 0, which is not pruned yet.
	if dbInfos[0].baseIndex > 0 {
		lastPrunedEpoch = dbInfos[0].baseIndex - 1
		b.lastPrunedEpoch.MarkEvicted(lastPrunedEpoch)
	}

	// Open all the dbInstances (perform health checks) and add them to the openDBs cache. Also fills the dbSizes map (when evicted from the cache).
	for _, dbInfo := range dbInfos {
		b.getDBInstance(dbInfo.baseIndex)
	}

	return
}

// getDBInstance returns the DB instance for the given epochIndex or creates a new one if it does not yet exist.
// DBs are created as follows where each db is located in m.basedir/<starting epochIndex>/
//
//	epochIndex 0 -> db 0
//	epochIndex 1 -> db 1
//	epochIndex 2 -> db 2
func (b *BucketManager) getDBInstance(index iotago.EpochIndex) (db *database.DBInstance) {
	// Lock global mutex to prevent closing and copying storage data on disk during engine switching.
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	b.openDBsMutex.Lock()
	defer b.openDBsMutex.Unlock()

	// check if exists again, as other goroutine might have created it in parallel
	db, exists := b.openDBs.Get(index)
	if !exists {
		db = database.NewDBInstance(b.dbConfig.WithDirectory(dbPathFromIndex(b.dbConfig.Directory, index)))

		// Remove the cached db size since we will open the db
		b.dbSizes.Delete(index)
		b.openDBs.Put(index, db)
	}

	return db
}

func (b *BucketManager) Prune(epoch iotago.EpochIndex) error {
	b.lastPrunedMutex.Lock()
	defer b.lastPrunedMutex.Unlock()

	if epoch < lo.Return1(b.lastPrunedEpoch.Index()) {
		return ierrors.Wrapf(database.ErrNoPruningNeeded, "epoch %d is already pruned", epoch)
	}

	b.DeleteBucket(epoch)

	b.lastPrunedEpoch.MarkEvicted(epoch)

	return nil
}

// DeleteBucket deletes directory that stores the data for the given bucket and returns boolean
// flag indicating whether a directory for that bucket existed.
func (b *BucketManager) DeleteBucket(epoch iotago.EpochIndex) (deleted bool) {
	b.openDBsMutex.Lock()
	defer b.openDBsMutex.Unlock()

	if exists, err := PathExists(dbPathFromIndex(b.dbConfig.Directory, epoch)); err != nil {
		panic(err)
	} else if !exists {
		return false
	}

	db, exists := b.openDBs.Get(epoch)
	if exists {
		db.Close()
		b.openDBs.Remove(epoch)
	}

	if err := os.RemoveAll(dbPathFromIndex(b.dbConfig.Directory, epoch)); err != nil {
		panic(err)
	}

	// Delete the db size since we pruned the whole directory
	b.dbSizes.Delete(epoch)

	return true
}

// RollbackBucket removes data in the bucket in slots [targetSlotIndex+1; epochEndSlot].
func (b *BucketManager) RollbackBucket(epochIndex iotago.EpochIndex, targetSlotIndex, epochEndSlot iotago.SlotIndex) error {
	oldBucketKvStore := b.getDBInstance(epochIndex).KVStore()
	for clearSlot := targetSlotIndex + 1; clearSlot <= epochEndSlot; clearSlot++ {
		// delete slot prefix from forkedPrunable storage that will be eventually copied into the new engine
		if err := oldBucketKvStore.DeletePrefix(clearSlot.MustBytes()); err != nil {
			return ierrors.Wrapf(err, "error while clearing slot %d in bucket for epoch %d", clearSlot, epochIndex)
		}
	}

	return nil
}
func (b *BucketManager) Flush() error {
	b.openDBsMutex.RLock()
	defer b.openDBsMutex.RUnlock()

	var err error
	b.openDBs.Each(func(epoch iotago.EpochIndex, db *database.DBInstance) {
		if err = db.KVStore().Flush(); err != nil {
			return
		}
	})

	return err
}

func PathExists(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}
