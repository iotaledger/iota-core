package epochstore

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	iotago "github.com/iotaledger/iota.go/v4"
)

type BaseStore[V any] struct {
	realm            kvstore.Realm
	kv               *kvstore.TypedStore[iotago.EpochIndex, V]
	prunginDelayFunc func(iotago.EpochIndex) iotago.EpochIndex

	lastAccessedEpoch *kvstore.TypedValue[iotago.EpochIndex]
	lastPrunedEpoch   *model.PruningIndex
}

func NewStore[V any](storeRealm kvstore.Realm, kv kvstore.KVStore, prunginDelayFunc func(iotago.EpochIndex) iotago.EpochIndex, vToBytes kvstore.ObjectToBytes[V], bytesToV kvstore.BytesToObject[V]) *BaseStore[V] {
	return &BaseStore[V]{
		realm:             storeRealm,
		kv:                kvstore.NewTypedStore(lo.PanicOnErr(kv.WithExtendedRealm(append(storeRealm, entriesKey))), iotago.EpochIndex.Bytes, iotago.EpochIndexFromBytes, vToBytes, bytesToV),
		prunginDelayFunc:  prunginDelayFunc,
		lastAccessedEpoch: kvstore.NewTypedValue(kv, append(storeRealm, lastAccessedEpochKey), iotago.EpochIndex.Bytes, iotago.EpochIndexFromBytes),
		lastPrunedEpoch:   model.NewPruningIndex(lo.PanicOnErr(kv.WithExtendedRealm(storeRealm)), kvstore.Realm{lastPrunedEpochKey}),
	}
}

func (s *BaseStore[V]) RestoreLastPrunedEpoch() error {
	return s.lastPrunedEpoch.RestoreFromDisk()
}

func (s *BaseStore[V]) LastAccessedEpoch() (lastAccessedEpoch iotago.EpochIndex, err error) {
	if lastAccessedEpoch, err = s.lastAccessedEpoch.Get(); err != nil {
		if !ierrors.Is(err, kvstore.ErrKeyNotFound) {
			err = ierrors.Wrap(err, "failed to get last accessed epoch")
		} else {
			err = nil
		}
	}

	return lastAccessedEpoch, err
}

func (s *BaseStore[V]) LastPrunedEpoch() (iotago.EpochIndex, bool) {
	return s.lastPrunedEpoch.Index()
}

// Load loads the value for the given epoch.
func (s *BaseStore[V]) Load(epoch iotago.EpochIndex) (V, error) {
	var zeroValue V

	if s.isTooOld(epoch) {
		return zeroValue, ierrors.WithMessagef(database.ErrEpochPruned, "epoch %d is too old", epoch)
	}

	value, err := s.kv.Get(epoch)
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return zeroValue, nil
		}

		return zeroValue, ierrors.Wrapf(err, "failed to get value for epoch %d", epoch)
	}

	return value, nil
}

func (s *BaseStore[V]) Store(epoch iotago.EpochIndex, value V) error {
	//nolint:revive
	_, _ = s.lastAccessedEpoch.Compute(func(lastAccessedEpoch iotago.EpochIndex, exists bool) (newValue iotago.EpochIndex, err error) {
		if lastAccessedEpoch >= epoch {
			return lastAccessedEpoch, kvstore.ErrTypedValueNotChanged
		}

		return epoch, nil
	})

	if s.isTooOld(epoch) {
		return ierrors.WithMessagef(database.ErrEpochPruned, "epoch %d is too old", epoch)
	}

	return s.kv.Set(epoch, value)
}

func (s *BaseStore[V]) Stream(consumer func(epoch iotago.EpochIndex, value V) error) error {
	var innerErr error
	if storageErr := s.kv.Iterate(kvstore.EmptyPrefix, func(epoch iotago.EpochIndex, value V) (advance bool) {
		innerErr = consumer(epoch, value)

		return innerErr == nil
	}); storageErr != nil {
		return ierrors.Wrapf(storageErr, "failed to iterate over store for realm %v", s.realm)
	}

	if innerErr != nil {
		return ierrors.Wrapf(innerErr, "failed to stream store for realm %v", s.realm)
	}

	return nil
}

func (s *BaseStore[V]) StreamBytes(consumer func([]byte, []byte) error) error {
	var innerErr error
	if storageErr := s.kv.KVStore().Iterate(kvstore.EmptyPrefix, func(key kvstore.Key, value kvstore.Value) (advance bool) {
		innerErr = consumer(key, value)
		return innerErr == nil
	}); storageErr != nil {
		return ierrors.Wrapf(storageErr, "failed to iterate over store for realm %v", s.realm)
	}

	if innerErr != nil {
		return ierrors.Wrapf(innerErr, "failed to stream bytes in store for realm %v", s.realm)
	}

	return innerErr
}

func (s *BaseStore[V]) DeleteEpoch(epoch iotago.EpochIndex) error {
	return s.kv.DeletePrefix(epoch.MustBytes())
}

func (s *BaseStore[V]) Prune(epoch iotago.EpochIndex, defaultPruningDelay iotago.EpochIndex) ([]iotago.EpochIndex, error) {
	prunedEpochs := make([]iotago.EpochIndex, 0)
	minPruningDelay := s.prunginDelayFunc(epoch)

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
		return prunedEpochs, nil
	}

	for i := s.lastPrunedEpoch.NextIndex(); i <= epoch-pruningDelay; i++ {
		err := s.kv.DeletePrefix(i.MustBytes())
		if err != nil {
			return prunedEpochs, ierrors.Wrapf(err, "failed to prune epoch store for realm %v at epoch %d", s.realm, i)
		}
		err = s.lastPrunedEpoch.MarkEvicted(i)
		if err != nil {
			return prunedEpochs, ierrors.Wrapf(err, "failed to store lastPrunedEpoch for epoch %d in Prune", i)
		}

		prunedEpochs = append(prunedEpochs, i)
	}

	return prunedEpochs, nil
}

func (s *BaseStore[V]) RollbackEpochs(epoch iotago.EpochIndex) (lastPrunedEpoch iotago.EpochIndex, rolledbackEpochs []iotago.EpochIndex, err error) {
	rolledbackEpochs = make([]iotago.EpochIndex, 0)

	lastAccessedEpoch, err := s.LastAccessedEpoch()
	if err != nil {
		return lastAccessedEpoch, rolledbackEpochs, ierrors.Wrap(err, "failed to get last accessed epoch")
	}

	for epochToPrune := epoch; epochToPrune <= lastAccessedEpoch; epochToPrune++ {
		if err = s.DeleteEpoch(epochToPrune); err != nil {
			return epochToPrune, rolledbackEpochs, ierrors.Wrapf(err, "error while deleting epoch %d", epochToPrune)
		}

		rolledbackEpochs = append(rolledbackEpochs, epochToPrune)
	}

	return lastAccessedEpoch, rolledbackEpochs, nil
}

func (s *BaseStore[V]) isTooOld(epoch iotago.EpochIndex) bool {
	prunedEpoch, hasPruned := s.lastPrunedEpoch.Index()

	return hasPruned && epoch <= prunedEpoch
}
