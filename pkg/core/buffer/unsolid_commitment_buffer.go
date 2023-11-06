package buffer

import (
	"sync"

	"github.com/zyedidia/generic/cache"

	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/ringbuffer"
	"github.com/iotaledger/hive.go/ds/types"
	iotago "github.com/iotaledger/iota.go/v4"
)

type optionalSizedBuffer[V comparable] interface {
	ToSlice() []V
	Add(value V) bool
}

type UnsolidCommitmentBuffer[V comparable] struct {
	blockBufferMaxSize int

	mutex           sync.RWMutex
	lastEvictedSlot iotago.SlotIndex
	blockBuffers    *memstorage.IndexedStorage[iotago.SlotIndex, iotago.CommitmentID, optionalSizedBuffer[V]]

	commitmentBuffer      *cache.Cache[iotago.CommitmentID, types.Empty]
	commitmentBufferMutex sync.Mutex
}

func NewUnsolidCommitmentBuffer[V comparable](commitmentBufferMaxSize int, blockBufferMaxSize ...int) *UnsolidCommitmentBuffer[V] {
	u := &UnsolidCommitmentBuffer[V]{
		blockBuffers:     memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.CommitmentID, optionalSizedBuffer[V]](),
		commitmentBuffer: cache.New[iotago.CommitmentID, types.Empty](commitmentBufferMaxSize),
	}

	if len(blockBufferMaxSize) > 0 {
		u.blockBufferMaxSize = blockBufferMaxSize[0]
	}

	u.commitmentBuffer.SetEvictCallback(func(commitmentID iotago.CommitmentID, _ types.Empty) {
		if blockBufferForCommitments := u.blockBuffers.Get(commitmentID.Slot(), false); blockBufferForCommitments != nil {
			blockBufferForCommitments.Delete(commitmentID)
		}
	})

	return u
}

func (u *UnsolidCommitmentBuffer[V]) Add(commitmentID iotago.CommitmentID, value V) bool {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	if commitmentID.Slot() <= u.lastEvictedSlot {
		return false
	}

	u.add(commitmentID, value)

	return true
}

func (u *UnsolidCommitmentBuffer[V]) add(commitmentID iotago.CommitmentID, value V) {
	buffer, _ := u.blockBuffers.Get(commitmentID.Slot(), true).GetOrCreate(commitmentID, func() optionalSizedBuffer[V] {
		if u.blockBufferMaxSize > 0 {
			return ringbuffer.NewRingBuffer[V](u.blockBufferMaxSize)
		}

		return ds.NewSet[V]()
	})
	buffer.Add(value)

	u.commitmentBufferMutex.Lock()
	defer u.commitmentBufferMutex.Unlock()

	u.commitmentBuffer.Put(commitmentID, types.Void)
}

func (u *UnsolidCommitmentBuffer[V]) AddWithFunc(commitmentID iotago.CommitmentID, value V, addFunc func() bool) (added bool) {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	if commitmentID.Slot() <= u.lastEvictedSlot {
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

	if blockBufferForCommitments := u.blockBuffers.Get(commitmentID.Slot()); blockBufferForCommitments != nil {
		if buffer, exists := blockBufferForCommitments.Get(commitmentID); exists {
			return buffer.ToSlice()
		}
	}

	return nil
}

func (u *UnsolidCommitmentBuffer[V]) GetValuesAndEvict(commitmentID iotago.CommitmentID) []V {
	u.mutex.Lock() // Need to attain write lock to make sure that no value is added while we are reading and evicting.
	defer u.mutex.Unlock()

	var values []V
	if blockBufferForCommitments := u.blockBuffers.Get(commitmentID.Slot()); blockBufferForCommitments != nil {
		if buffer, exists := blockBufferForCommitments.Get(commitmentID); exists {
			values = buffer.ToSlice()
		}
	}

	u.evictUntil(commitmentID.Slot())

	return values
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (u *UnsolidCommitmentBuffer[V]) Reset() {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	u.blockBuffers.Clear()
	u.commitmentBuffer.Each(func(key iotago.CommitmentID, _ types.Empty) { u.commitmentBuffer.Remove(key) })
}
