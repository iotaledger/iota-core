package database

import (
	"sync/atomic"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
)

type DBInstance struct {
	store         *lockedKVStore // KVStore that is used to access the DB instance
	healthTracker *kvstore.StoreHealthTracker
	dbConfig      Config
	isClosed      atomic.Bool
	isShutdown    atomic.Bool
}

func NewDBInstance(dbConfig Config, openedCallback func(d *DBInstance)) *DBInstance {
	db, err := StoreWithDefaultSettings(dbConfig.Directory, true, dbConfig.Engine)
	if err != nil {
		panic(err)
	}

	dbInstance := &DBInstance{
		dbConfig: dbConfig,
	}

	// Create a storeInstanceMutex that will be used to lock access to Open() method.
	// This allows us to avoid contention upon write-locking access to the KVStore upon all operations.
	storeInstanceMutex := new(syncutils.Mutex)

	// lockedKVStore and the underlying openableKVStore don't handle opening and closing the underlying store by themselves,
	// but delegate that to the entity that constructed it. It needs to be done like that, because opening and closing the underlying store also
	// modifies the state of the DBInstance. Other methods (e.g. Flush) that don't modify the state of DBInstance can be handled directly.
	lockableKVStore := newLockedKVStore(db, func() error {
		if dbInstance.isClosed.Load() {
			storeInstanceMutex.Lock()
			defer storeInstanceMutex.Unlock()

			if dbInstance.isClosed.Load() {
				if err := dbInstance.Open(); err != nil {
					return err
				}

				if openedCallback != nil {
					openedCallback(dbInstance)
				}
			}
		}

		return nil
	}, func() {
		dbInstance.CloseWithoutLocking()
	})

	dbInstance.store = lockableKVStore

	// HealthTracker state is only modified while holding the lock on the lockableKVStore;
	//  that's why it needs to use openableKVStore (which does not lock) instead of lockableKVStore to avoid a deadlock.
	storeHealthTracker, err := kvstore.NewStoreHealthTracker(lockableKVStore.openableKVStore, dbConfig.PrefixHealth, dbConfig.Version, nil)
	if err != nil {
		panic(ierrors.Wrapf(err, "database in %s is corrupted, delete database and resync node", dbConfig.Directory))
	}
	if err = storeHealthTracker.MarkCorrupted(); err != nil {
		panic(err)
	}

	dbInstance.healthTracker = storeHealthTracker

	return dbInstance
}

func (d *DBInstance) Shutdown() {
	d.isShutdown.Store(true)

	d.Close()
}

func (d *DBInstance) Flush() {
	d.store.LockAccess()
	defer d.store.UnlockAccess()

	if !d.isClosed.Load() {
		if instance, err := d.store.instance(); err == nil {
			_ = instance.Flush()
		}
	}
}

func (d *DBInstance) Close() {
	d.store.LockAccess()
	defer d.store.UnlockAccess()

	d.CloseWithoutLocking()
}

func (d *DBInstance) CloseWithoutLocking() {
	if !d.isClosed.Load() {
		if err := d.healthTracker.MarkHealthy(); err != nil {
			panic(err)
		}

		if err := d.store.topParent().storeInstance.Flush(); err != nil {
			panic(err)
		}

		if err := d.store.topParent().storeInstance.Close(); err != nil {
			panic(err)
		}

		d.isClosed.Store(true)
	}
}

// Open re-opens a closed DBInstance. It must only be called while holding a lock on DBInstance,
// otherwise it might cause a race condition and corruption of node's state.
func (d *DBInstance) Open() error {
	if !d.isClosed.Load() {
		panic(ErrDatabaseNotClosed)
	}

	if d.isShutdown.Load() {
		return ErrDatabaseShutdown
	}

	d.store.Replace(lo.PanicOnErr(StoreWithDefaultSettings(d.dbConfig.Directory, false, d.dbConfig.Engine)))

	d.isClosed.Store(false)

	if err := d.healthTracker.MarkCorrupted(); err != nil {
		// panic immediately as in this case the database state is corrupted
		panic(err)
	}

	return nil
}

func (d *DBInstance) LockAccess() {
	d.store.LockAccess()
}

func (d *DBInstance) UnlockAccess() {
	d.store.UnlockAccess()
}

func (d *DBInstance) KVStore() kvstore.KVStore {
	return d.store
}
