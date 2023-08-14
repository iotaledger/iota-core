package prunable

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	iotago "github.com/iotaledger/iota.go/v4"
)

type dbInstance struct {
	index         iotago.EpochIndex
	store         kvstore.KVStore // KVStore that is used to access the DB instance
	healthTracker *kvstore.StoreHealthTracker
}

func newDBInstance(index iotago.EpochIndex, dbConfig database.Config) *dbInstance {
	db, err := database.StoreWithDefaultSettings(dbConfig.Directory, true, dbConfig.Engine)
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

	return &dbInstance{
		index:         index,
		store:         db,
		healthTracker: storeHealthTracker,
	}
}

func (db *dbInstance) Close() {
	if err := db.healthTracker.MarkHealthy(); err != nil {
		panic(err)
	}
	if err := database.FlushAndClose(db.store); err != nil {
		panic(err)
	}
}
