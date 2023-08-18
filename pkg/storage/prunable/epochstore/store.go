package epochstore

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Store[V any] struct {
	realm        kvstore.Realm
	kv           *kvstore.TypedStore[iotago.EpochIndex, V]
	pruningDelay iotago.EpochIndex

	lastPrunedEpoch *model.EvictionIndex[iotago.EpochIndex]
	lastPrunedMutex syncutils.RWMutex
}

func NewStore[V any](realm kvstore.Realm, kv kvstore.KVStore, pruningDelay iotago.EpochIndex, vToBytes kvstore.ObjectToBytes[V], bytesToV kvstore.BytesToObject[V]) *Store[V] {
	kv = lo.PanicOnErr(kv.WithExtendedRealm(realm))

	return &Store[V]{
		realm:           realm,
		kv:              kvstore.NewTypedStore(kv, iotago.EpochIndex.Bytes, iotago.EpochIndexFromBytes, vToBytes, bytesToV),
		pruningDelay:    pruningDelay,
		lastPrunedEpoch: model.NewEvictionIndex[iotago.EpochIndex](),
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

func (s *Store[V]) PruneUntilEpoch(epoch iotago.EpochIndex) error {
	s.lastPrunedMutex.Lock()
	defer s.lastPrunedMutex.Unlock()

	minStartEpoch := s.lastPrunedEpoch.NextIndex() + s.pruningDelay
	// No need to prune.
	if epoch < minStartEpoch {
		return nil
	}

	for currentIndex := minStartEpoch; currentIndex <= epoch; currentIndex++ {
		if err := s.prune(currentIndex); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store[V]) prune(epoch iotago.EpochIndex) error {
	targetIndex := epoch - s.pruningDelay
	err := s.kv.DeletePrefix(targetIndex.MustBytes())
	if err != nil {
		return ierrors.Wrapf(err, "failed to prune epoch store for realm %v", s.realm)
	}

	s.lastPrunedEpoch.MarkEvicted(targetIndex)

	return nil
}
