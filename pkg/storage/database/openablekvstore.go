package database

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/utils"
	"github.com/iotaledger/hive.go/serializer/v2/byteutils"
)

type openableKVStore struct {
	storeInstance kvstore.KVStore // KVStore that is used to access the DB instance
	parentStore   *openableKVStore
	dbPrefix      kvstore.KeyPrefix
}

func newOpenableKVStore(storeInstance kvstore.KVStore) *openableKVStore {
	return &openableKVStore{
		storeInstance: storeInstance,
		parentStore:   nil,
		dbPrefix:      kvstore.EmptyPrefix,
	}
}

func (s *openableKVStore) instance() kvstore.KVStore {
	if s.storeInstance != nil {
		return s.storeInstance
	}

	return s.parentStore.instance()
}

func (s *openableKVStore) Replace(newKVStore kvstore.KVStore) {
	if s.storeInstance == nil {
		s.parentStore.Replace(newKVStore)

		return
	}

	s.storeInstance = newKVStore
}

func (s *openableKVStore) WithRealm(realm kvstore.Realm) (kvstore.KVStore, error) {
	return s.withRealm(realm)
}
func (s *openableKVStore) withRealm(realm kvstore.Realm) (kvstore.KVStore, error) {
	return &openableKVStore{
		storeInstance: nil,
		parentStore:   s,
		dbPrefix:      realm,
	}, nil
}
func (s *openableKVStore) WithExtendedRealm(realm kvstore.Realm) (kvstore.KVStore, error) {
	return s.withRealm(s.buildKeyPrefix(realm))
}

func (s *openableKVStore) Realm() kvstore.Realm {
	return s.dbPrefix
}

func (s *openableKVStore) Iterate(prefix kvstore.KeyPrefix, kvConsumerFunc kvstore.IteratorKeyValueConsumerFunc, direction ...kvstore.IterDirection) error {
	return s.instance().Iterate(s.buildKeyPrefix(prefix), func(key kvstore.Key, value kvstore.Value) bool {
		return kvConsumerFunc(utils.CopyBytes(key)[len(s.dbPrefix):], value)
	}, direction...)
}

func (s *openableKVStore) IterateKeys(prefix kvstore.KeyPrefix, consumerFunc kvstore.IteratorKeyConsumerFunc, direction ...kvstore.IterDirection) error {
	return s.instance().IterateKeys(s.buildKeyPrefix(prefix), func(key kvstore.Key) bool {
		return consumerFunc(utils.CopyBytes(key)[len(s.dbPrefix):])
	}, direction...)
}

func (s *openableKVStore) Clear() error {
	return s.instance().DeletePrefix(s.dbPrefix)
}

func (s *openableKVStore) Get(key kvstore.Key) (value kvstore.Value, err error) {
	return s.instance().Get(byteutils.ConcatBytes(s.dbPrefix, key))
}

func (s *openableKVStore) Set(key kvstore.Key, value kvstore.Value) error {
	return s.instance().Set(byteutils.ConcatBytes(s.dbPrefix, key), value)
}

func (s *openableKVStore) Has(key kvstore.Key) (bool, error) {
	return s.instance().Has(byteutils.ConcatBytes(s.dbPrefix, key))
}

func (s *openableKVStore) Delete(key kvstore.Key) error {
	return s.instance().Delete(byteutils.ConcatBytes(s.dbPrefix, key))
}

func (s *openableKVStore) DeletePrefix(prefix kvstore.KeyPrefix) error {
	return s.instance().DeletePrefix(s.buildKeyPrefix(prefix))
}

func (s *openableKVStore) Flush() error {
	return s.instance().Flush()
}
func (s *openableKVStore) Close() error {
	return s.instance().Close()
}

func (s *openableKVStore) Batched() (kvstore.BatchedMutations, error) {
	return &openableKVStoreBatchedMutations{
		parentStore:      s,
		dbPrefix:         s.dbPrefix,
		setOperations:    make(map[string]kvstore.Value),
		deleteOperations: make(map[string]types.Empty),
	}, nil
}

// builds a key usable using the realm and the given prefix.
func (s *openableKVStore) buildKeyPrefix(prefix kvstore.KeyPrefix) kvstore.KeyPrefix {
	return byteutils.ConcatBytes(s.dbPrefix, prefix)
}

type openableKVStoreBatchedMutations struct {
	parentStore      *openableKVStore
	dbPrefix         kvstore.KeyPrefix
	setOperations    map[string]kvstore.Value
	deleteOperations map[string]types.Empty
	operationsMutex  sync.Mutex
}

func (s *openableKVStoreBatchedMutations) Set(key kvstore.Key, value kvstore.Value) error {
	stringKey := byteutils.ConcatBytesToString(s.dbPrefix, key)

	s.operationsMutex.Lock()
	defer s.operationsMutex.Unlock()

	delete(s.deleteOperations, stringKey)
	s.setOperations[stringKey] = value

	return nil
}

func (s *openableKVStoreBatchedMutations) Delete(key kvstore.Key) error {
	stringKey := byteutils.ConcatBytesToString(s.dbPrefix, key)

	s.operationsMutex.Lock()
	defer s.operationsMutex.Unlock()

	delete(s.setOperations, stringKey)
	s.deleteOperations[stringKey] = types.Void

	return nil
}

func (s *openableKVStoreBatchedMutations) Cancel() {
	s.operationsMutex.Lock()
	defer s.operationsMutex.Unlock()

	s.setOperations = make(map[string]kvstore.Value)
	s.deleteOperations = make(map[string]types.Empty)
}

func (s *openableKVStoreBatchedMutations) Commit() error {
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
