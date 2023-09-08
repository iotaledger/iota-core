package database

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
)

type DBInstance struct {
	store         kvstore.KVStore // KVStore that is used to access the DB instance
	healthTracker *kvstore.StoreHealthTracker
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
		store:         db,
		healthTracker: storeHealthTracker,
	}
}

func (d *DBInstance) Close() {
	if err := d.healthTracker.MarkHealthy(); err != nil {
		panic(err)
	}
	if err := FlushAndClose(d.store); err != nil {
		panic(err)
	}
}

//func (d *DBInstance) Open() {
//	d.store.Replace(StoreWithDefaultSettings(dbConfig.Directory, true, dbConfig.Engine))
//}

func (d *DBInstance) KVStore() kvstore.KVStore {
	return d.store
}
