package epochstore

import (
	"github.com/iotaledger/hive.go/ds/queue"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	iotago "github.com/iotaledger/iota.go/v4"
)

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
	if removedEpoch, wasRemoved := c.addedQueue.ForceOffer(epoch); wasRemoved {
		c.cache.Delete(removedEpoch)
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

	// As loading from the cache is a hot path (frequent reading) and a read-only operation,
	// we can use a read-lock to reduce contention.
	c.mutex.RLock()
	if value, exists := c.cache.Get(epoch); exists {
		c.mutex.RUnlock()
		return value, nil
	}
	c.mutex.RUnlock()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check again if the value has been added in the meantime.
	if value, exists := c.cache.Get(epoch); exists {
		return value, nil
	}

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

func (c *CachedStore[V]) RollbackEpochs(epoch iotago.EpochIndex) (lastPrunedEpoch iotago.EpochIndex, rollbackedEpochs []iotago.EpochIndex, err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	lastPrunedEpoch, rollbackedEpochs, err = c.store.RollbackEpochs(epoch)
	for _, rollbackEpoch := range rollbackedEpochs {
		c.removeFromCache(rollbackEpoch)
	}

	if err != nil {
		return lastPrunedEpoch, rollbackedEpochs, ierrors.Wrapf(err, "failed to rollback epoch %d", epoch)
	}

	return lastPrunedEpoch, rollbackedEpochs, nil
}
