package epochstore

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Store[V any] struct {
	realm        kvstore.Realm
	kv           *kvstore.TypedStore[iotago.EpochIndex, V]
	pruningDelay iotago.EpochIndex

	lastPrunedEpoch *model.PruningIndex
	// lastPrunedMutex syncutils.RWMutex
}

func NewStore[V any](storeRealm kvstore.Realm, pruningRealm kvstore.Realm, kv kvstore.KVStore, pruningDelay iotago.EpochIndex, vToBytes kvstore.ObjectToBytes[V], bytesToV kvstore.BytesToObject[V]) *Store[V] {
	return &Store[V]{
		realm:           storeRealm,
		kv:              kvstore.NewTypedStore(lo.PanicOnErr(kv.WithExtendedRealm(storeRealm)), iotago.EpochIndex.Bytes, iotago.EpochIndexFromBytes, vToBytes, bytesToV),
		pruningDelay:    pruningDelay,
		lastPrunedEpoch: model.NewPruningIndex(lo.PanicOnErr(kv.WithExtendedRealm(pruningRealm)), storeRealm),
	}
}

func (s *Store[V]) isTooOld(epoch iotago.EpochIndex) bool {
	s.lastPrunedMutex.RLock()
	defer s.lastPrunedMutex.RUnlock()

	prunedEpoch, hasPruned := s.lastPrunedEpoch.Index()

	return hasPruned && epoch <= prunedEpoch
}

func (s *Store[V]) RestoreLastPrunedEpoch() error {
	s.lastPrunedMutex.Lock()
	defer s.lastPrunedMutex.Unlock()

	var lowestEpoch *iotago.EpochIndex
	if err := s.streamEpochs(func(epoch iotago.EpochIndex) error {
		if lowestEpoch == nil || epoch < *lowestEpoch {
			lowestEpoch = &epoch
		}

		return nil
	}); err != nil {
		return err
	}

	if lowestEpoch != nil && *lowestEpoch > 0 {
		// We need to subtract 1 as lowestEpoch is the last epoch we have stored. Thus, the last pruned is -1.
		s.lastPrunedEpoch.MarkEvicted(*lowestEpoch - 1)
	}

	return nil
}

func (s *Store[V]) LastPrunedEpoch() (iotago.EpochIndex, bool) {
	s.lastPrunedMutex.RLock()
	defer s.lastPrunedMutex.RUnlock()

	return s.lastPrunedEpoch.Index()
}

// Load loads the value for the given epoch.
func (s *Store[V]) Load(epoch iotago.EpochIndex) (V, error) {
	var zeroValue V

	if s.isTooOld(epoch) {
		return zeroValue, ierrors.Wrapf(database.ErrEpochPruned, "epoch %d is too old", epoch)
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

func (s *Store[V]) Store(epoch iotago.EpochIndex, value V) error {
	if s.isTooOld(epoch) {
		return ierrors.Wrapf(database.ErrEpochPruned, "epoch %d is too old", epoch)
	}

	return s.kv.Set(epoch, value)
}

func (s *Store[V]) Stream(consumer func(epoch iotago.EpochIndex, value V) error) error {
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

func (s *Store[V]) StreamBytes(consumer func([]byte, []byte) error) error {
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

func (s *Store[V]) streamEpochs(consumer func(epoch iotago.EpochIndex) error) error {
	var innerErr error
	if storageErr := s.kv.IterateKeys(kvstore.EmptyPrefix, func(epoch iotago.EpochIndex) (advance bool) {
		innerErr = consumer(epoch)

		return innerErr == nil
	}); storageErr != nil {
		return ierrors.Wrap(storageErr, "failed to iterate over epochs")
	}

	if innerErr != nil {
		return ierrors.Wrap(innerErr, "failed to stream epochs")
	}

	return nil
}

func (s *Store[V]) Prune(epoch iotago.EpochIndex, defaultPruningDelay iotago.EpochIndex) error {
	s.lastPrunedMutex.Lock()
	defer s.lastPrunedMutex.Unlock()

	// The epoch we're trying to prune already takes into account the defaultPruningDelay.
	// Therefore, we don't need to do anything if it is greater equal s.pruningDelay and take the difference otherwise.
	var pruningDelay iotago.EpochIndex
	if defaultPruningDelay >= s.pruningDelay {
		pruningDelay = 0
	} else {
		pruningDelay = s.pruningDelay - defaultPruningDelay
	}

	// No need to prune.
	if epoch < pruningDelay {
		return nil
	}

	for i := s.lastPrunedEpoch.NextIndex(); i <= epoch-pruningDelay; i++ {
		err := s.kv.DeletePrefix(i.MustBytes())
		if err != nil {
			return ierrors.Wrapf(err, "failed to prune epoch store for realm %v at epoch %d", s.realm, i)
		}
		s.lastPrunedEpoch.MarkEvicted(i)
	}

	return nil
}
