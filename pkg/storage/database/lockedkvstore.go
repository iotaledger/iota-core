package database

import (
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/serializer/v2/byteutils"
)

type lockedKVStore struct {
	*openableKVStore

	accessMutex *syncutils.StarvingMutex
}

func newLockedKVStore(storeInstance kvstore.KVStore, openStoreIfNecessary func() error, closeStore func()) *lockedKVStore {
	return &lockedKVStore{
		openableKVStore: newOpenableKVStore(storeInstance, openStoreIfNecessary, closeStore),
		accessMutex:     syncutils.NewStarvingMutex(),
	}
}

func (s *lockedKVStore) LockAccess() {
	s.accessMutex.Lock()
}

func (s *lockedKVStore) UnlockAccess() {
	s.accessMutex.Unlock()
}

func (s *lockedKVStore) WithRealm(realm kvstore.Realm) (kvstore.KVStore, error) {
	s.accessMutex.RLock()
	defer s.accessMutex.RUnlock()

	return s.withRealm(realm)
}

func (s *lockedKVStore) withRealm(realm kvstore.Realm) (kvstore.KVStore, error) {
	return &lockedKVStore{
		openableKVStore: &openableKVStore{
			storeInstance: nil,
			parentStore:   s.openableKVStore,
			dbPrefix:      realm,
		},

		accessMutex: s.accessMutex,
	}, nil
}

func (s *lockedKVStore) WithExtendedRealm(realm kvstore.Realm) (kvstore.KVStore, error) {
	s.accessMutex.RLock()
	defer s.accessMutex.RUnlock()

	return s.withRealm(s.buildKeyPrefix(realm))
}

func (s *lockedKVStore) Iterate(prefix kvstore.KeyPrefix, kvConsumerFunc kvstore.IteratorKeyValueConsumerFunc, direction ...kvstore.IterDirection) error {
	s.accessMutex.RLock()
	defer s.accessMutex.RUnlock()

	return s.openableKVStore.Iterate(prefix, kvConsumerFunc, direction...)
}

func (s *lockedKVStore) IterateKeys(prefix kvstore.KeyPrefix, consumerFunc kvstore.IteratorKeyConsumerFunc, direction ...kvstore.IterDirection) error {
	s.accessMutex.RLock()
	defer s.accessMutex.RUnlock()

	return s.openableKVStore.IterateKeys(prefix, consumerFunc, direction...)
}

func (s *lockedKVStore) Clear() error {
	s.accessMutex.RLock()
	defer s.accessMutex.RUnlock()

	return s.openableKVStore.Clear()
}

func (s *lockedKVStore) Get(key kvstore.Key) (value kvstore.Value, err error) {
	s.accessMutex.RLock()
	defer s.accessMutex.RUnlock()

	return s.openableKVStore.Get(key)
}

func (s *lockedKVStore) Set(key kvstore.Key, value kvstore.Value) error {
	s.accessMutex.RLock()
	defer s.accessMutex.RUnlock()

	return s.openableKVStore.Set(key, value)
}

func (s *lockedKVStore) Has(key kvstore.Key) (bool, error) {
	s.accessMutex.RLock()
	defer s.accessMutex.RUnlock()

	return s.openableKVStore.Has(key)
}

func (s *lockedKVStore) Delete(key kvstore.Key) error {
	s.accessMutex.RLock()
	defer s.accessMutex.RUnlock()

	return s.openableKVStore.Delete(key)
}

func (s *lockedKVStore) DeletePrefix(prefix kvstore.KeyPrefix) error {
	s.accessMutex.RLock()
	defer s.accessMutex.RUnlock()

	return s.openableKVStore.DeletePrefix(prefix)
}

func (s *lockedKVStore) Flush() error {
	s.accessMutex.RLock()
	defer s.accessMutex.RUnlock()

	return s.FlushWithoutLocking()
}

func (s *lockedKVStore) FlushWithoutLocking() error {
	return s.openableKVStore.Flush()
}

func (s *lockedKVStore) Close() error {
	s.accessMutex.RLock()
	defer s.accessMutex.RUnlock()

	return s.CloseWithoutLocking()
}

func (s *lockedKVStore) CloseWithoutLocking() error {
	return s.openableKVStore.Close()
}

func (s *lockedKVStore) Batched() (kvstore.BatchedMutations, error) {
	s.accessMutex.RLock()
	defer s.accessMutex.RUnlock()

	return &syncedBatchedMutations{
		openableKVStoreBatchedMutations: &openableKVStoreBatchedMutations{
			parentStore:      s.openableKVStore,
			dbPrefix:         s.dbPrefix,
			setOperations:    make(map[string]kvstore.Value),
			deleteOperations: make(map[string]types.Empty),
		},

		parentStore: s,
	}, nil
}

// builds a key usable using the realm and the given prefix.
func (s *lockedKVStore) buildKeyPrefix(prefix kvstore.KeyPrefix) kvstore.KeyPrefix {
	return byteutils.ConcatBytes(s.dbPrefix, prefix)
}

type syncedBatchedMutations struct {
	*openableKVStoreBatchedMutations

	parentStore *lockedKVStore
}

func (s *syncedBatchedMutations) Commit() error {
	s.parentStore.accessMutex.RLock()
	defer s.parentStore.accessMutex.RUnlock()

	return s.openableKVStoreBatchedMutations.Commit()
}
