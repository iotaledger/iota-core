package database

import (
	"fmt"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
)

type DBInstance struct {
	store         *synchedKVStore // KVStore that is used to access the DB instance
	healthTracker *kvstore.StoreHealthTracker
	dbConfig      Config
}

func NewDBInstance(dbConfig Config) *DBInstance {
	db, err := StoreWithDefaultSettings(dbConfig.Directory, true, dbConfig.Engine)
	if err != nil {
		panic(err)
	}
	storeHealthTracker, err := kvstore.NewStoreHealthTracker(db, dbConfig.PrefixHealth, dbConfig.Version, nil)
	if err != nil {
		panic(ierrors.Wrapf(err, "database in %s is corrupted, delete database and resync node", dbConfig.Directory))
	}
	if err = storeHealthTracker.MarkCorrupted(); err != nil {
		panic(err)
	}

	return &DBInstance{
		store:         &synchedKVStore{store: db},
		healthTracker: storeHealthTracker,
		dbConfig:      dbConfig,
	}
}

func (d *DBInstance) Close() {
	fmt.Println("close kvstore", d.dbConfig.Directory)

	if err := d.healthTracker.MarkHealthy(); err != nil {
		panic(err)
	}
	if err := FlushAndClose(d.store); err != nil {
		panic(err)
	}
}

func (d *DBInstance) Open() {
	fmt.Println("open kvstore", d.dbConfig.Directory)
	d.store.Replace(lo.PanicOnErr(StoreWithDefaultSettings(d.dbConfig.Directory, false, d.dbConfig.Engine)))
	_, err := d.store.store.WithRealm(kvstore.EmptyPrefix)
	if err != nil {
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
