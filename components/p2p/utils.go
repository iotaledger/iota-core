package p2p

import (
	"path/filepath"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/rocksdb"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/storage/database"
)

// inits the peer database.
func initPeerDB() (peerDB *network.DB, peerDBKVStore kvstore.KVStore, err error) {
	if err = checkValidPeerDBPath(); err != nil {
		return nil, nil, ierrors.Wrap(err, "invalid peer database path")
	}

	db, err := database.NewRocksDB(ParamsP2P.Database.Path)
	if err != nil {
		return nil, nil, ierrors.Wrap(err, "error creating peer database")
	}

	peerDBKVStore = rocksdb.New(db)

	return network.NewDB(peerDBKVStore), peerDBKVStore, nil
}

// checks that the peer database path does not reside within the main database directory.
func checkValidPeerDBPath() error {
	_, err := filepath.Abs(ParamsP2P.Database.Path)
	if err != nil {
		return ierrors.Wrapf(err, "cannot resolve absolute path of %s", ParamsP2P.Database.Path)
	}

	return nil
}
