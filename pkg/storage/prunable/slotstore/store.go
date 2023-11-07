package slotstore

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Store[K, V any] struct {
	slot iotago.SlotIndex
	kv   *kvstore.TypedStore[K, V]
}

func NewStore[K, V any](
	slot iotago.SlotIndex,
	kv kvstore.KVStore,
	keyToBytes kvstore.ObjectToBytes[K],
	bytesToKey kvstore.BytesToObject[K],
	vToBytes kvstore.ObjectToBytes[V],
	bytesToV kvstore.BytesToObject[V],
) *Store[K, V] {
	return &Store[K, V]{
		slot: slot,
		kv:   kvstore.NewTypedStore(kv, keyToBytes, bytesToKey, vToBytes, bytesToV),
	}
}

func (s *Store[K, V]) Load(key K) (value V, exists bool, err error) {
	value, err = s.kv.Get(key)
	if err != nil {
		var zeroValue V
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return zeroValue, false, nil
		}

		return zeroValue, false, ierrors.Wrapf(err, "failed to get value for key %v", key)
	}

	return value, true, nil
}

func (s *Store[K, V]) Store(key K, value V) error {
	return s.kv.Set(key, value)
}

// Has returns true if the given blockID is a root block.
func (s *Store[K, V]) Has(key K) (has bool, err error) {
	return s.kv.Has(key)
}

func (s *Store[K, V]) Delete(key K) (err error) {
	return s.kv.Delete(key)
}

func (s *Store[K, V]) StreamKeys(consumer func(key K) error) error {
	var innerErr error
	if storageErr := s.kv.IterateKeys(kvstore.EmptyPrefix, func(key K) (advance bool) {
		innerErr = consumer(key)

		return innerErr == nil
	}); storageErr != nil {
		return ierrors.Wrapf(storageErr, "failed to iterate over keys for slot %s", s.slot)
	}

	if innerErr != nil {
		return ierrors.Wrapf(innerErr, "failed to stream keys for slot %s", s.slot)
	}

	return nil
}

func (s *Store[K, V]) Stream(consumer func(key K, value V) error) error {
	var innerErr error
	if storageErr := s.kv.Iterate(kvstore.EmptyPrefix, func(key K, value V) (advance bool) {
		innerErr = consumer(key, value)

		return innerErr == nil
	}); storageErr != nil {
		return ierrors.Wrapf(storageErr, "failed to iterate over store for slot %s", s.slot)
	}

	if innerErr != nil {
		return ierrors.Wrapf(innerErr, "failed to stream store for slot %s", s.slot)
	}

	return nil
}

func (s *Store[K, V]) StreamBytes(consumer func([]byte, []byte) error) error {
	var innerErr error
	if storageErr := s.kv.KVStore().Iterate(kvstore.EmptyPrefix, func(key kvstore.Key, value kvstore.Value) (advance bool) {
		innerErr = consumer(key, value)
		return innerErr == nil
	}); storageErr != nil {
		return ierrors.Wrapf(storageErr, "failed to iterate over store for slot %s", s.slot)
	}

	if innerErr != nil {
		return ierrors.Wrapf(innerErr, "failed to stream bytes in store for slot %s", s.slot)
	}

	return innerErr
}
