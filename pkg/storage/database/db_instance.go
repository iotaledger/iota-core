package database

import (
	"sync/atomic"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
)

type DBInstance struct {
	store         *lockedKVStore // KVStore that is used to access the DB instance
	healthTracker *kvstore.StoreHealthTracker
	dbConfig      Config
	isClosed      atomic.Bool
}

func NewDBInstance(dbConfig Config) *DBInstance {
	db, err := StoreWithDefaultSettings(dbConfig.Directory, true, dbConfig.Engine)
	if err != nil {
		panic(err)
	}

	dbInstance := &DBInstance{
		dbConfig: dbConfig,
	}

	lockableKVStore := newLockedKVStore(db, dbInstance)

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

func (d *DBInstance) Close() {
	d.store.Lock()
	defer d.store.Unlock()

	d.CloseWithoutLocking()

	d.isClosed.Store(true)
}

func (d *DBInstance) CloseWithoutLocking() {
	if err := d.healthTracker.MarkHealthy(); err != nil {
		panic(err)
	}

	if err := FlushAndClose(d.store); err != nil {
		panic(err)
	}

	d.isClosed.Store(true)
}

// Open re-opens a closed DBInstance. It must only be called while holding a lock on DBInstance,
// otherwise it might cause a race condition and corruption of node's state.
func (d *DBInstance) Open() {
	d.store.Replace(lo.PanicOnErr(StoreWithDefaultSettings(d.dbConfig.Directory, false, d.dbConfig.Engine)))

	if err := d.healthTracker.MarkCorrupted(); err != nil {
		panic(err)
	}
}

func (d *DBInstance) Lock() {
	d.store.Lock()
}

func (d *DBInstance) Unlock() {
	d.store.Unlock()
}

func (d *DBInstance) KVStore() kvstore.KVStore {
	return d.store
}
