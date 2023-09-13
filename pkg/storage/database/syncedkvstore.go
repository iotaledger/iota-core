package database

import (
	"fmt"
	"sync"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/utils"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/serializer/v2/byteutils"
)

type syncedKVStore struct {
	storeInstance kvstore.KVStore // KVStore that is used to access the DB instance
	parentStore   *syncedKVStore
	dbPrefix      kvstore.KeyPrefix

	instanceMutex *syncutils.RWMutex
}

func (s *syncedKVStore) instance() kvstore.KVStore {
	if s.storeInstance != nil {
		return s.storeInstance
	}

	return s.parentStore.instance()
}

func (s *syncedKVStore) Lock() {
	s.instanceMutex.Lock()
}

func (s *syncedKVStore) Unlock() {
	s.instanceMutex.Unlock()
}

func (s *syncedKVStore) Replace(newKVStore kvstore.KVStore) {
	if s.storeInstance == nil {
		s.parentStore.Replace(newKVStore)

		return
	}

	s.storeInstance = newKVStore
}

func (s *syncedKVStore) WithRealm(realm kvstore.Realm) (kvstore.KVStore, error) {
	s.instanceMutex.RLock()
	defer s.instanceMutex.RUnlock()

	return s.withRealm(realm)
}
func (s *syncedKVStore) withRealm(realm kvstore.Realm) (kvstore.KVStore, error) {
	return &syncedKVStore{
		storeInstance: nil,
		parentStore:   s,
		instanceMutex: s.instanceMutex,
		dbPrefix:      realm,
	}, nil
}
func (s *syncedKVStore) WithExtendedRealm(realm kvstore.Realm) (kvstore.KVStore, error) {
	s.instanceMutex.RLock()
	defer s.instanceMutex.RUnlock()

	return s.withRealm(s.buildKeyPrefix(realm))
}

func (s *syncedKVStore) Realm() kvstore.Realm {
	return s.dbPrefix
}

func (s *syncedKVStore) Iterate(prefix kvstore.KeyPrefix, kvConsumerFunc kvstore.IteratorKeyValueConsumerFunc, direction ...kvstore.IterDirection) error {
	s.instanceMutex.RLock()
	defer s.instanceMutex.RUnlock()
	return s.instance().Iterate(s.buildKeyPrefix(prefix), func(key kvstore.Key, value kvstore.Value) bool {
		fmt.Println("realm", s.dbPrefix, "prefix", prefix, "key", key, "value", value)

		return kvConsumerFunc(utils.CopyBytes(key)[len(s.dbPrefix):], value)
	}, direction...)
}

func (s *syncedKVStore) IterateKeys(prefix kvstore.KeyPrefix, consumerFunc kvstore.IteratorKeyConsumerFunc, direction ...kvstore.IterDirection) error {
	s.instanceMutex.RLock()
	defer s.instanceMutex.RUnlock()

	return s.instance().IterateKeys(s.buildKeyPrefix(prefix), func(key kvstore.Key) bool {
		return consumerFunc(utils.CopyBytes(key)[len(s.dbPrefix):])
	}, direction...)
}

func (s *syncedKVStore) Clear() error {
	s.instanceMutex.RLock()
	defer s.instanceMutex.RUnlock()

	return s.instance().DeletePrefix(s.dbPrefix)
}

func (s *syncedKVStore) Get(key kvstore.Key) (value kvstore.Value, err error) {
	s.instanceMutex.RLock()
	defer s.instanceMutex.RUnlock()

	return s.instance().Get(byteutils.ConcatBytes(s.dbPrefix, key))
}

func (s *syncedKVStore) Set(key kvstore.Key, value kvstore.Value) error {
	s.instanceMutex.RLock()
	defer s.instanceMutex.RUnlock()

	return s.instance().Set(byteutils.ConcatBytes(s.dbPrefix, key), value)
}

func (s *syncedKVStore) Has(key kvstore.Key) (bool, error) {
	s.instanceMutex.RLock()
	defer s.instanceMutex.RUnlock()

	return s.instance().Has(byteutils.ConcatBytes(s.dbPrefix, key))
}

func (s *syncedKVStore) Delete(key kvstore.Key) error {
	s.instanceMutex.RLock()
	defer s.instanceMutex.RUnlock()

	return s.instance().Delete(byteutils.ConcatBytes(s.dbPrefix, key))
}

func (s *syncedKVStore) DeletePrefix(prefix kvstore.KeyPrefix) error {
	s.instanceMutex.RLock()
	defer s.instanceMutex.RUnlock()

	return s.instance().DeletePrefix(s.buildKeyPrefix(prefix))
}

func (s *syncedKVStore) Flush() error {
	s.instanceMutex.RLock()
	defer s.instanceMutex.RUnlock()

	return s.FlushWithoutLocking()
}

func (s *syncedKVStore) FlushWithoutLocking() error {
	return s.instance().Flush()
}
func (s *syncedKVStore) Close() error {
	s.instanceMutex.RLock()
	defer s.instanceMutex.RUnlock()

	return s.CloseWithoutLocking()
}
func (s *syncedKVStore) CloseWithoutLocking() error {
	return s.instance().Close()
}

func (s *syncedKVStore) Batched() (kvstore.BatchedMutations, error) {
	s.instanceMutex.RLock()
	defer s.instanceMutex.RUnlock()

	return &syncedBachedMutations{
		parentStore:      s,
		dbPrefix:         s.dbPrefix,
		setOperations:    make(map[string]kvstore.Value),
		deleteOperations: make(map[string]types.Empty),
	}, nil
}

// builds a key usable using the realm and the given prefix.
func (s *syncedKVStore) buildKeyPrefix(prefix kvstore.KeyPrefix) kvstore.KeyPrefix {
	return byteutils.ConcatBytes(s.dbPrefix, prefix)
}

type syncedBachedMutations struct {
	parentStore      *syncedKVStore
	dbPrefix         kvstore.KeyPrefix
	setOperations    map[string]kvstore.Value
	deleteOperations map[string]types.Empty
	operationsMutex  sync.Mutex
}

func (s *syncedBachedMutations) Set(key kvstore.Key, value kvstore.Value) error {
	stringKey := byteutils.ConcatBytesToString(s.dbPrefix, key)

	s.operationsMutex.Lock()
	defer s.operationsMutex.Unlock()

	delete(s.deleteOperations, stringKey)
	s.setOperations[stringKey] = value

	return nil
}

func (s *syncedBachedMutations) Delete(key kvstore.Key) error {
	stringKey := byteutils.ConcatBytesToString(s.dbPrefix, key)

	s.operationsMutex.Lock()
	defer s.operationsMutex.Unlock()

	delete(s.setOperations, stringKey)
	s.deleteOperations[stringKey] = types.Void

	return nil
}

func (s *syncedBachedMutations) Cancel() {
	s.operationsMutex.Lock()
	defer s.operationsMutex.Unlock()

	s.setOperations = make(map[string]kvstore.Value)
	s.deleteOperations = make(map[string]types.Empty)
}

func (s *syncedBachedMutations) Commit() error {
	s.parentStore.instanceMutex.RLock()
	defer s.parentStore.instanceMutex.RUnlock()

	batched, err := s.parentStore.instance().Batched()
	if err != nil {
		return err
	}

	s.operationsMutex.Lock()
	defer s.operationsMutex.Unlock()

	for key, value := range s.setOperations {
		if err = batched.Set([]byte(key), value); err != nil {
			return err
		}
	}

	for key := range s.deleteOperations {
		if err = batched.Delete([]byte(key)); err != nil {
			return err
		}
	}

	return batched.Commit()
}
