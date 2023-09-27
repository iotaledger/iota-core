package epochstore

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	iotago "github.com/iotaledger/iota.go/v4"
)

type EpochKVStore struct {
	realm        kvstore.Realm
	kv           kvstore.KVStore
	pruningDelay iotago.EpochIndex

	lastPrunedEpoch *model.PruningIndex
}

func NewEpochKVStore(storeRealm, pruningRealm kvstore.Realm, kv kvstore.KVStore, pruningDelay iotago.EpochIndex) *EpochKVStore {

	return &EpochKVStore{
		realm:           storeRealm,
		kv:              lo.PanicOnErr(kv.WithExtendedRealm(storeRealm)),
		pruningDelay:    pruningDelay,
		lastPrunedEpoch: model.NewPruningIndex(lo.PanicOnErr(kv.WithExtendedRealm(pruningRealm)), storeRealm),
	}
}

func (e *EpochKVStore) isTooOld(epoch iotago.EpochIndex) bool {
	prunedEpoch, hasPruned := e.lastPrunedEpoch.Index()

	return hasPruned && epoch <= prunedEpoch
}

func (e *EpochKVStore) RestoreLastPrunedEpoch() error {
	return e.lastPrunedEpoch.RestoreFromDisk()
}

func (e *EpochKVStore) LastPrunedEpoch() (iotago.EpochIndex, bool) {
	return e.lastPrunedEpoch.Index()
}

func (e *EpochKVStore) GetEpoch(epoch iotago.EpochIndex) (kvstore.KVStore, error) {
	if e.isTooOld(epoch) {
		return nil, ierrors.Wrapf(database.ErrEpochPruned, "epoch %d is too old", epoch)
	}

	return lo.PanicOnErr(e.kv.WithExtendedRealm(epoch.MustBytes())), nil
}

func (e *EpochKVStore) DeleteEpoch(epoch iotago.EpochIndex) error {
	return e.kv.DeletePrefix(epoch.MustBytes())
}

func (e *EpochKVStore) Prune(epoch iotago.EpochIndex, defaultPruningDelay iotago.EpochIndex) error {
	// The epoch we're trying to prune already takes into account the defaultPruningDelay.
	// Therefore, we don't need to do anything if it is greater equal e.pruningDelay and take the difference otherwise.
	var pruningDelay iotago.EpochIndex
	if defaultPruningDelay >= e.pruningDelay {
		pruningDelay = 0
	} else {
		pruningDelay = e.pruningDelay - defaultPruningDelay
	}

	// No need to prune.
	if epoch < pruningDelay {
		return nil
	}

	for i := e.lastPrunedEpoch.NextIndex(); i <= epoch-pruningDelay; i++ {
		err := e.kv.DeletePrefix(i.MustBytes())
		if err != nil {
			return ierrors.Wrapf(err, "failed to prune epoch store for realm %v at epoch %d", e.realm, i)
		}
		err = e.lastPrunedEpoch.MarkEvicted(i)
		if err != nil {
			return ierrors.Wrapf(err, "failed to store lastPrunedEpoch for epoch %d in Prune", i)
		}
	}

	return nil
}
