package epochstore

import (
	"github.com/iotaledger/hive.go/ds/queue"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Store[V any] interface {
	RestoreLastPrunedEpoch() error
	LastAccessedEpoch() (iotago.EpochIndex, error)
	LastPrunedEpoch() (iotago.EpochIndex, bool)
	Load(epoch iotago.EpochIndex) (V, error)
	Store(epoch iotago.EpochIndex, value V) error
	Stream(consumer func(epoch iotago.EpochIndex, value V) error) error
	StreamBytes(consumer func([]byte, []byte) error) error
	DeleteEpoch(epoch iotago.EpochIndex) error
	Prune(epoch iotago.EpochIndex, defaultPruningDelay iotago.EpochIndex) ([]iotago.EpochIndex, error)
	RollbackEpochs(epoch iotago.EpochIndex) (iotago.EpochIndex, []iotago.EpochIndex, error)
}

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

func (s *BaseStore[V]) Store(epoch iotago.EpochIndex, value V) error {
	//nolint:revive
	_, _ = s.lastAccessedEpoch.Compute(func(lastAccessedEpoch iotago.EpochIndex, exists bool) (newValue iotago.EpochIndex, err error) {
		if lastAccessedEpoch >= epoch {
			return lastAccessedEpoch, kvstore.ErrTypedValueNotChanged
		}

		return epoch, nil
	})

	if s.isTooOld(epoch) {
		return ierrors.Wrapf(database.ErrEpochPruned, "epoch %d is too old", epoch)
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

func (s *BaseStore[V]) RollbackEpochs(epoch iotago.EpochIndex) (lastPrunedEpoch iotago.EpochIndex, rollbackEpochs []iotago.EpochIndex, err error) {
	rollbackEpochs = make([]iotago.EpochIndex, 0)

	lastAccessedEpoch, err := s.LastAccessedEpoch()
	if err != nil {
		return lastAccessedEpoch, rollbackEpochs, ierrors.Wrap(err, "failed to get last accessed epoch")
	}

	for epochToPrune := epoch; epochToPrune <= lastAccessedEpoch; epochToPrune++ {
		if err = s.DeleteEpoch(epochToPrune); err != nil {
			return epochToPrune, rollbackEpochs, ierrors.Wrapf(err, "error while deleting epoch %d", epochToPrune)
		}

		rollbackEpochs = append(rollbackEpochs, epochToPrune)
	}

	return lastAccessedEpoch, rollbackEpochs, nil
}

func (s *BaseStore[V]) isTooOld(epoch iotago.EpochIndex) bool {
	prunedEpoch, hasPruned := s.lastPrunedEpoch.Index()

	return hasPruned && epoch <= prunedEpoch
}

type CachedStore[V any] struct {
	store *BaseStore[V]

	cache      *shrinkingmap.ShrinkingMap[iotago.EpochIndex, V]
	addedQueue *queue.Queue[iotago.EpochIndex]

	mutex syncutils.RWMutex
}

func NewCachedStore[V any](store *BaseStore[V], maxCacheSize int) *CachedStore[V] {
	return &CachedStore[V]{
		store:      store,
		addedQueue: queue.New[iotago.EpochIndex](maxCacheSize),
		cache:      shrinkingmap.New[iotago.EpochIndex, V](),
	}
}

func (c *CachedStore[V]) addToCache(epoch iotago.EpochIndex, value V) {
	if !c.addedQueue.Offer(epoch) {
		removedEpoch, _ := c.addedQueue.Poll()
		c.cache.Delete(removedEpoch)

		c.addedQueue.Offer(epoch)
	}

	c.cache.Set(epoch, value)
}

func (c *CachedStore[V]) removeFromCache(epoch iotago.EpochIndex) {
	// removing from addedQueue is not necessary, as it will be overwritten anyway when the cache gets full.
	c.cache.Delete(epoch)
}

func (c *CachedStore[V]) RestoreLastPrunedEpoch() error {
	return c.store.RestoreLastPrunedEpoch()
}

func (c *CachedStore[V]) LastAccessedEpoch() (lastAccessedEpoch iotago.EpochIndex, err error) {
	return c.store.LastAccessedEpoch()
}

func (c *CachedStore[V]) LastPrunedEpoch() (iotago.EpochIndex, bool) {
	return c.store.LastPrunedEpoch()
}

func (c *CachedStore[V]) Load(epoch iotago.EpochIndex) (V, error) {
	var zeroValue V

	c.mutex.RLock()
	if value, exists := c.cache.Get(epoch); exists {
		c.mutex.RUnlock()
		return value, nil
	}
	c.mutex.RUnlock()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	committee, err := c.store.Load(epoch)
	if err != nil {
		return zeroValue, ierrors.Wrapf(err, "failed to load value for epoch %d", epoch)
	}

	c.addToCache(epoch, committee)

	return committee, nil
}

func (c *CachedStore[V]) Store(epoch iotago.EpochIndex, value V) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	err := c.store.Store(epoch, value)
	if err != nil {
		return ierrors.Wrapf(err, "failed to store value for epoch %d", epoch)
	}

	c.addToCache(epoch, value)

	return nil
}

func (c *CachedStore[V]) Stream(consumer func(epoch iotago.EpochIndex, value V) error) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.store.Stream(consumer)
}

func (c *CachedStore[V]) StreamBytes(consumer func([]byte, []byte) error) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.store.StreamBytes(consumer)
}

func (c *CachedStore[V]) DeleteEpoch(epoch iotago.EpochIndex) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	err := c.store.DeleteEpoch(epoch)
	if err != nil {
		return ierrors.Wrapf(err, "failed to delete epoch %d", epoch)
	}

	c.removeFromCache(epoch)

	return nil
}

func (c *CachedStore[V]) Prune(epoch iotago.EpochIndex, defaultPruningDelay iotago.EpochIndex) ([]iotago.EpochIndex, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	prunedEpochs, err := c.store.Prune(epoch, defaultPruningDelay)
	for _, prunedEpoch := range prunedEpochs {
		c.removeFromCache(prunedEpoch)
	}

	if err != nil {
		return prunedEpochs, ierrors.Wrapf(err, "failed to prune epoch %d", epoch)
	}

	return prunedEpochs, nil
}

func (c *CachedStore[V]) RollbackEpochs(epoch iotago.EpochIndex) (lastPrunedEpoch iotago.EpochIndex, rollbackEpochs []iotago.EpochIndex, err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	lastPrunedEpoch, rollbackEpochs, err = c.store.RollbackEpochs(epoch)
	for _, rollbackEpoch := range rollbackEpochs {
		c.removeFromCache(rollbackEpoch)
	}

	if err != nil {
		return lastPrunedEpoch, rollbackEpochs, ierrors.Wrapf(err, "failed to rollback epoch %d", epoch)
	}

	return lastPrunedEpoch, rollbackEpochs, nil
}
