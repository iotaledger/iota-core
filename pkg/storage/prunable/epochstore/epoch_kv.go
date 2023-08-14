package epochstore

import (
	"github.com/iotaledger/hive.go/ierrors"
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

func (e *EpochKVStore) Prune(epochIndex iotago.EpochIndex) error {
	if epochIndex < e.pruningDelay {
		return ierrors.Errorf("epoch index %d is smaller than pruning delay %d", epochIndex, e.pruningDelay)
	}

	targetIndex := epochIndex - e.pruningDelay

	return e.kv.DeletePrefix(targetIndex.MustBytes())
}
