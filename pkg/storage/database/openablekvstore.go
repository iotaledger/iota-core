package database

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/utils"
	"github.com/iotaledger/hive.go/serializer/v2/byteutils"
)

type openableKVStore struct {
	openIfNecessary func() error // openIfNecessary callback should synchronize itself and make sure that storeInstance is ready to use after.
	closeStore      func()
	storeInstance   kvstore.KVStore // storeInstance is a KVStore that is holding the reference to the underlying database.
	parentStore     *openableKVStore
	dbPrefix        kvstore.KeyPrefix
}

func newOpenableKVStore(storeInstance kvstore.KVStore, openStoreIfNecessary func() error, closeStore func()) *openableKVStore {
	return &openableKVStore{
		openIfNecessary: openStoreIfNecessary,
		closeStore:      closeStore,
		storeInstance:   storeInstance,
		parentStore:     nil,
		dbPrefix:        kvstore.EmptyPrefix,
	}
}

func (s *openableKVStore) topParent() *openableKVStore {
	current := s
	for current.parentStore != nil {
		current = current.parentStore
	}

	return current
}

func (s *openableKVStore) instance() (kvstore.KVStore, error) {
	parent := s.topParent()
	// openIfNecessary callback should synchronize itself and make sure that storeInstance is ready to use after.
	if err := parent.openIfNecessary(); err != nil {
		return nil, err
	}

	return parent.storeInstance, nil
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
	instance, err := s.instance()
	if err != nil {
		return err
	}

	return instance.Iterate(s.buildKeyPrefix(prefix), func(key kvstore.Key, value kvstore.Value) bool {
		return kvConsumerFunc(utils.CopyBytes(key)[len(s.dbPrefix):], value)
	}, direction...)
}

func (s *openableKVStore) IterateKeys(prefix kvstore.KeyPrefix, consumerFunc kvstore.IteratorKeyConsumerFunc, direction ...kvstore.IterDirection) error {
	instance, err := s.instance()
	if err != nil {
		return err
	}

	return instance.IterateKeys(s.buildKeyPrefix(prefix), func(key kvstore.Key) bool {
		return consumerFunc(utils.CopyBytes(key)[len(s.dbPrefix):])
	}, direction...)
}

func (s *openableKVStore) Clear() error {
	instance, err := s.instance()
	if err != nil {
		return err
	}

	return instance.DeletePrefix(s.dbPrefix)
}

func (s *openableKVStore) Get(key kvstore.Key) (value kvstore.Value, err error) {
	instance, err := s.instance()
	if err != nil {
		return nil, err
	}

	return instance.Get(byteutils.ConcatBytes(s.dbPrefix, key))
}

func (s *openableKVStore) Set(key kvstore.Key, value kvstore.Value) error {
	instance, err := s.instance()
	if err != nil {
		return err
	}

	return instance.Set(byteutils.ConcatBytes(s.dbPrefix, key), value)
}

func (s *openableKVStore) Has(key kvstore.Key) (bool, error) {
	instance, err := s.instance()
	if err != nil {
		return false, err
	}

	return instance.Has(byteutils.ConcatBytes(s.dbPrefix, key))
}

func (s *openableKVStore) Delete(key kvstore.Key) error {
	instance, err := s.instance()
	if err != nil {
		return err
	}

	return instance.Delete(byteutils.ConcatBytes(s.dbPrefix, key))
}

func (s *openableKVStore) DeletePrefix(prefix kvstore.KeyPrefix) error {
	instance, err := s.instance()
	if err != nil {
		return err
	}

	return instance.DeletePrefix(s.buildKeyPrefix(prefix))
}

func (s *openableKVStore) Flush() error {
	instance, err := s.instance()
	if err != nil {
		return err
	}

	return instance.Flush()
}

func (s *openableKVStore) Close() error {
	s.topParent().closeStore()

	return nil
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
	instance, err := s.parentStore.instance()
	if err != nil {
		return err
	}

	batched, err := instance.Batched()
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
