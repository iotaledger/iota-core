package database

import (
	"github.com/iotaledger/hive.go/kvstore"
)

func FlushAndClose(store kvstore.KVStore) error {
	if err := store.Flush(); err != nil {
		return err
	}

	return store.Close()
}
