package database

import (
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/syncutils"
)

type synchedKVStore struct {
	store kvstore.KVStore // KVStore that is used to access the DB instance

	syncutils.RWMutex
}

func (s *synchedKVStore) Replace(newKVStore kvstore.KVStore) {
	s.store = newKVStore
}

func (s *synchedKVStore) WithRealm(realm kvstore.Realm) (kvstore.KVStore, error) {
	s.RLock()
	defer s.RUnlock()

	return s.store.WithRealm(realm)
}

func (s *synchedKVStore) WithExtendedRealm(realm kvstore.Realm) (kvstore.KVStore, error) {
	s.RLock()
	defer s.RUnlock()

	return s.store.WithExtendedRealm(realm)
}

func (s *synchedKVStore) Realm() kvstore.Realm {
	return s.store.Realm()
}

func (s *synchedKVStore) Iterate(prefix kvstore.KeyPrefix, kvConsumerFunc kvstore.IteratorKeyValueConsumerFunc, direction ...kvstore.IterDirection) error {
	s.RLock()
	defer s.RUnlock()

	return s.store.Iterate(prefix, kvConsumerFunc, direction...)
}

func (s *synchedKVStore) IterateKeys(prefix kvstore.KeyPrefix, consumerFunc kvstore.IteratorKeyConsumerFunc, direction ...kvstore.IterDirection) error {
	s.RLock()
	defer s.RUnlock()

	return s.store.IterateKeys(prefix, consumerFunc, direction...)
}

func (s *synchedKVStore) Clear() error {
	s.RLock()
	defer s.RUnlock()

	return s.store.Clear()
}

func (s *synchedKVStore) Get(key kvstore.Key) (value kvstore.Value, err error) {
	s.RLock()
	defer s.RUnlock()

	return s.store.Get(key)
}

func (s *synchedKVStore) Set(key kvstore.Key, value kvstore.Value) error {
	s.RLock()
	defer s.RUnlock()

	return s.store.Set(key, value)
}

func (s *synchedKVStore) Has(key kvstore.Key) (bool, error) {
	s.RLock()
	defer s.RUnlock()

	return s.store.Has(key)
}

func (s *synchedKVStore) Delete(key kvstore.Key) error {
	s.RLock()
	defer s.RUnlock()

	return s.store.Delete(key)
}

func (s *synchedKVStore) DeletePrefix(prefix kvstore.KeyPrefix) error {
	s.RLock()
	defer s.RUnlock()

	return s.store.DeletePrefix(prefix)
}

func (s *synchedKVStore) Flush() error {
	return s.store.Flush()
}

func (s *synchedKVStore) Close() error {
	return s.store.Close()
}

func (s *synchedKVStore) Batched() (kvstore.BatchedMutations, error) {
	s.RLock()
	defer s.RUnlock()

	return s.store.Batched()
}
