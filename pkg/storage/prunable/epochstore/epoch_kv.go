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
	realm            kvstore.Realm
	kv               kvstore.KVStore
	prunginDelayFunc func(iotago.EpochIndex) iotago.EpochIndex

	lastAccessedEpoch *kvstore.TypedValue[iotago.EpochIndex]
	lastPrunedEpoch   *model.PruningIndex
}

func NewEpochKVStore(storeRealm kvstore.Realm, kv kvstore.KVStore, prunginDelayFunc func(iotago.EpochIndex) iotago.EpochIndex) *EpochKVStore {
	return &EpochKVStore{
		realm:             storeRealm,
		kv:                lo.PanicOnErr(kv.WithExtendedRealm(append(storeRealm, entriesKey))),
		prunginDelayFunc:  prunginDelayFunc,
		lastAccessedEpoch: kvstore.NewTypedValue(kv, append(storeRealm, lastAccessedEpochKey), iotago.EpochIndex.Bytes, iotago.EpochIndexFromBytes),
		lastPrunedEpoch:   model.NewPruningIndex(lo.PanicOnErr(kv.WithExtendedRealm(storeRealm)), kvstore.Realm{lastPrunedEpochKey}),
	}
}

func (e *EpochKVStore) isTooOld(epoch iotago.EpochIndex) bool {
	prunedEpoch, hasPruned := e.lastPrunedEpoch.Index()

	return hasPruned && epoch <= prunedEpoch
}

func (e *EpochKVStore) RestoreLastPrunedEpoch() error {
	return e.lastPrunedEpoch.RestoreFromDisk()
}

func (e *EpochKVStore) LastAccessedEpoch() (lastAccessedEpoch iotago.EpochIndex, err error) {
	if lastAccessedEpoch, err = e.lastAccessedEpoch.Get(); err != nil {
		if !ierrors.Is(err, kvstore.ErrKeyNotFound) {
			err = ierrors.Wrap(err, "failed to get last accessed epoch")
		} else {
			err = nil
		}
	}

	return lastAccessedEpoch, err
}

func (e *EpochKVStore) LastPrunedEpoch() (iotago.EpochIndex, bool) {
	return e.lastPrunedEpoch.Index()
}

func (e *EpochKVStore) GetEpoch(epoch iotago.EpochIndex) (kvstore.KVStore, error) {
	//nolint:revive
	_, _ = e.lastAccessedEpoch.Compute(func(lastAccessedEpoch iotago.EpochIndex, exists bool) (newValue iotago.EpochIndex, err error) {
		if lastAccessedEpoch >= epoch {
			return lastAccessedEpoch, kvstore.ErrTypedValueNotChanged
		}

		return epoch, nil
	})

	if e.isTooOld(epoch) {
		return nil, ierrors.WithMessagef(database.ErrEpochPruned, "epoch %d is too old", epoch)
	}

	return lo.PanicOnErr(e.kv.WithExtendedRealm(epoch.MustBytes())), nil
}

func (e *EpochKVStore) DeleteEpoch(epoch iotago.EpochIndex) error {
	return e.kv.DeletePrefix(epoch.MustBytes())
}

func (e *EpochKVStore) Prune(epoch iotago.EpochIndex, defaultPruningDelay iotago.EpochIndex) error {
	minPruningDelay := e.prunginDelayFunc(epoch)

	// The epoch we're trying to prune already takes into account the defaultPruningDelay.
	// Therefore, we don't need to do anything if it is greater equal minPruningDelay and take the difference otherwise.
	var pruningDelay iotago.EpochIndex
	if defaultPruningDelay >= minPruningDelay {
		pruningDelay = 0
	} else {
		pruningDelay = minPruningDelay - defaultPruningDelay
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

func (e *EpochKVStore) RollbackEpochs(epoch iotago.EpochIndex) (lastPrunedEpoch iotago.EpochIndex, err error) {
	lastAccessedEpoch, err := e.LastAccessedEpoch()
	if err != nil {
		return lastAccessedEpoch, ierrors.Wrap(err, "failed to get last accessed epoch")
	}

	for epochToPrune := epoch; epochToPrune <= lastAccessedEpoch; epochToPrune++ {
		if err = e.DeleteEpoch(epochToPrune); err != nil {
			return epochToPrune, ierrors.Wrapf(err, "error while deleting epoch %d", epochToPrune)
		}
	}

	return lastAccessedEpoch, nil
}
