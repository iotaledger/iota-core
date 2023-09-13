package database

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
)

type DBInstance struct {
	store         *syncedKVStore // KVStore that is used to access the DB instance
	healthTracker *kvstore.StoreHealthTracker
	dbConfig      Config
}

func NewDBInstance(dbConfig Config) *DBInstance {
	db, err := StoreWithDefaultSettings(dbConfig.Directory, true, dbConfig.Engine)
	if err != nil {
		panic(err)
	}

	syncedStore := &syncedKVStore{
		storeInstance: db,
		parentStore:   nil,
		dbPrefix:      kvstore.EmptyPrefix,
		instanceMutex: new(syncutils.RWMutex),
	}

	storeHealthTracker, err := kvstore.NewStoreHealthTracker(syncedStore, dbConfig.PrefixHealth, dbConfig.Version, nil)
	if err != nil {
		panic(ierrors.Wrapf(err, "database in %s is corrupted, delete database and resync node", dbConfig.Directory))
	}
	if err = storeHealthTracker.MarkCorrupted(); err != nil {
		panic(err)
	}

	return &DBInstance{
		store:         syncedStore,
		healthTracker: storeHealthTracker,
		dbConfig:      dbConfig,
	}
}

func (d *DBInstance) Close() {
	d.MarkHealthy()

	d.store.Lock()
	defer d.store.Unlock()

	if err := FlushAndClose(d.store); err != nil {
		panic(err)
	}
}

func (d *DBInstance) MarkHealthy() {
	if err := d.healthTracker.MarkHealthy(); err != nil {
		panic(err)
	}
}

// TODO: make markCorrupted and markHealthy not lock the kvstore so that we can first lock the kvstore and then mark as healthy and corrupted without causing a deadlock
func (d *DBInstance) MarkCorrupted() {
	if err := d.healthTracker.MarkCorrupted(); err != nil {
		panic(err)
	}
}
func (d *DBInstance) CloseWithoutLocking() {
	if err := FlushAndClose(d.store); err != nil {
		panic(err)
	}
}

func (d *DBInstance) Open() {
	d.store.Replace(lo.PanicOnErr(StoreWithDefaultSettings(d.dbConfig.Directory, false, d.dbConfig.Engine)))
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
