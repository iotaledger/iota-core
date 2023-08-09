package blockbuffer

import (
	"sync"

	"github.com/zyedidia/generic/cache"

	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds/ringbuffer"
	"github.com/iotaledger/hive.go/ds/types"
	iotago "github.com/iotaledger/iota.go/v4"
)

type UnsolidCommitmentBlocks[V any] struct {
	blockBufferMaxSize int

	mutex           sync.RWMutex
	lastEvictedSlot iotago.SlotIndex
	blockBuffers    *memstorage.IndexedStorage[iotago.SlotIndex, iotago.CommitmentID, *ringbuffer.RingBuffer[V]]

	commitmentBuffer      *cache.Cache[iotago.CommitmentID, types.Empty]
	commitmentBufferMutex sync.Mutex
}

func NewUnsolidCommitmentBlocks[V any](commitmentBufferMaxSize int, blockBufferMaxSize int) *UnsolidCommitmentBlocks[V] {
	u := &UnsolidCommitmentBlocks[V]{
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

func (u *UnsolidCommitmentBlocks[V]) Add(commitmentID iotago.CommitmentID, value V) bool {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	if commitmentID.Index() < u.lastEvictedSlot {
		return false
	}

	blockBufferForCommitments := u.blockBuffers.Get(commitmentID.Index(), true)

	buffer, _ := blockBufferForCommitments.GetOrCreate(commitmentID, func() *ringbuffer.RingBuffer[V] {
		return ringbuffer.NewRingBuffer[V](u.blockBufferMaxSize)
	})
	buffer.Add(value)

	u.commitmentBufferMutex.Lock()
	defer u.commitmentBufferMutex.Unlock()

	u.commitmentBuffer.Put(commitmentID, types.Void)

	return true
}

func (u *UnsolidCommitmentBlocks[V]) Evict(lastEvictedSlot iotago.SlotIndex) {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	u.lastEvictedSlot = lastEvictedSlot
	u.blockBuffers.Evict(lastEvictedSlot)
}

func (u *UnsolidCommitmentBlocks[V]) GetBlocks(commitmentID iotago.CommitmentID) []V {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	if blockBufferForCommitments := u.blockBuffers.Get(commitmentID.Index()); blockBufferForCommitments != nil {
		if buffer, exists := blockBufferForCommitments.Get(commitmentID); exists {
			return buffer.Elements()
		}
	}

	return nil
}
