package epochstore

import (
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type EpochKVStore struct {
	realm        kvstore.Realm
	kv           kvstore.KVStore
	pruningDelay iotago.EpochIndex
}

func NewEpochKVStore(realm kvstore.Realm, kv kvstore.KVStore, pruningDelay iotago.EpochIndex) *EpochKVStore {
	kv = lo.PanicOnErr(kv.WithExtendedRealm(realm))

	return &EpochKVStore{
		realm:        realm,
		kv:           kv,
		pruningDelay: pruningDelay,
	}
}

func (e *EpochKVStore) GetEpoch(epoch iotago.EpochIndex) kvstore.KVStore {
	return lo.PanicOnErr(e.kv.WithExtendedRealm(epoch.MustBytes()))
}
