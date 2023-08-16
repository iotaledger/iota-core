package buffer

import (
	"sync"

	"github.com/zyedidia/generic/cache"

	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds/ringbuffer"
	"github.com/iotaledger/hive.go/ds/types"
	iotago "github.com/iotaledger/iota.go/v4"
)

type UnsolidCommitmentBuffer[V any] struct {
	blockBufferMaxSize int

	mutex           sync.RWMutex
	lastEvictedSlot iotago.SlotIndex
	blockBuffers    *memstorage.IndexedStorage[iotago.SlotIndex, iotago.CommitmentID, *ringbuffer.RingBuffer[V]]

	commitmentBuffer      *cache.Cache[iotago.CommitmentID, types.Empty]
	commitmentBufferMutex sync.Mutex
}

func NewUnsolidCommitmentBuffer[V any](commitmentBufferMaxSize int, blockBufferMaxSize int) *UnsolidCommitmentBuffer[V] {
	u := &UnsolidCommitmentBuffer[V]{
		blockBufferMaxSize: blockBufferMaxSize,
		blockBuffers:       memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.CommitmentID, *ringbuffer.RingBuffer[V]](),
		commitmentBuffer:   cache.New[iotago.CommitmentID, types.Empty](commitmentBufferMaxSize),
	}

	u.commitmentBuffer.SetEvictCallback(func(commitmentID iotago.CommitmentID, _ types.Empty) {
		if blockBufferForCommitments := u.blockBuffers.Get(commitmentID.Index(), false); blockBufferForCommitments != nil {
			blockBufferForCommitments.Delete(commitmentID)
		}
	})

	return u
}

func (u *UnsolidCommitmentBuffer[V]) Add(commitmentID iotago.CommitmentID, value V) bool {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	if commitmentID.Index() <= u.lastEvictedSlot {
		return false
	}

	u.add(commitmentID, value)

	return true
}

func (u *UnsolidCommitmentBuffer[V]) add(commitmentID iotago.CommitmentID, value V) {
	buffer, _ := u.blockBuffers.Get(commitmentID.Index(), true).GetOrCreate(commitmentID, func() *ringbuffer.RingBuffer[V] {
		return ringbuffer.NewRingBuffer[V](u.blockBufferMaxSize)
	})
	buffer.Add(value)

	u.commitmentBufferMutex.Lock()
	defer u.commitmentBufferMutex.Unlock()

	u.commitmentBuffer.Put(commitmentID, types.Void)
}

func (u *UnsolidCommitmentBuffer[V]) AddWithFunc(commitmentID iotago.CommitmentID, value V, addFunc func() bool) (added bool) {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	if commitmentID.Index() <= u.lastEvictedSlot {
		return false
	}

	if !addFunc() {
		return false
	}

	u.add(commitmentID, value)

	return true
}

func (u *UnsolidCommitmentBuffer[V]) EvictUntil(slotToEvict iotago.SlotIndex) {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	u.evictUntil(slotToEvict)
}

func (u *UnsolidCommitmentBuffer[V]) evictUntil(slotToEvict iotago.SlotIndex) {
	for slot := u.lastEvictedSlot + 1; slot <= slotToEvict; slot++ {
		u.blockBuffers.Evict(slot)
	}

	u.lastEvictedSlot = slotToEvict
}

func (u *UnsolidCommitmentBuffer[V]) GetValues(commitmentID iotago.CommitmentID) []V {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	if blockBufferForCommitments := u.blockBuffers.Get(commitmentID.Index()); blockBufferForCommitments != nil {
		if buffer, exists := blockBufferForCommitments.Get(commitmentID); exists {
			return buffer.Elements()
		}
	}

	return nil
}

func (u *UnsolidCommitmentBuffer[V]) GetValuesAndEvict(commitmentID iotago.CommitmentID) []V {
	u.mutex.Lock() // Need to attain write lock to make sure that no value is added while we are reading and evicting.
	defer u.mutex.Unlock()

	var values []V
	if blockBufferForCommitments := u.blockBuffers.Get(commitmentID.Index()); blockBufferForCommitments != nil {
		if buffer, exists := blockBufferForCommitments.Get(commitmentID); exists {
			values = buffer.Elements()
		}
	}

	u.evictUntil(commitmentID.Index())

	return values
}
