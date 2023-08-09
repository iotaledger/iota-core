package protocol

import (
	"sync"

	"github.com/zyedidia/generic/cache"

	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds/ringbuffer"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	commitmentBufferMaxSize = 20
	blockBufferMaxSize      = 100
)

type UnsolidCommitmentBlocks struct {
	mutex           sync.RWMutex
	lastEvictedSlot iotago.SlotIndex
	blockBuffers    *memstorage.IndexedStorage[iotago.SlotIndex, iotago.CommitmentID, *ringbuffer.RingBuffer[*types.Tuple[*model.Block, network.PeerID]]]

	commitmentBuffer      *cache.Cache[iotago.CommitmentID, types.Empty]
	commitmentBufferMutex sync.Mutex
}

func newUnsolidCommitmentBlocks() *UnsolidCommitmentBlocks {
	u := &UnsolidCommitmentBlocks{
		blockBuffers:     memstorage.NewIndexedStorage[iotago.SlotIndex, iotago.CommitmentID, *ringbuffer.RingBuffer[*types.Tuple[*model.Block, network.PeerID]]](),
		commitmentBuffer: cache.New[iotago.CommitmentID, types.Empty](commitmentBufferMaxSize),
	}

	u.commitmentBuffer.SetEvictCallback(func(commitmentID iotago.CommitmentID, _ types.Empty) {
		if blockBufferForCommitments := u.blockBuffers.Get(commitmentID.Index(), false); blockBufferForCommitments != nil {
			blockBufferForCommitments.Delete(commitmentID)
		}
	})

	return u
}

func (u *UnsolidCommitmentBlocks) AddBlock(block *model.Block, src network.PeerID) bool {
	u.mutex.RLock()
	defer u.mutex.RUnlock()
	slotCommitmentID := block.ProtocolBlock().SlotCommitmentID

	if slotCommitmentID.Index() < u.lastEvictedSlot {
		return false
	}

	blockBufferForCommitments := u.blockBuffers.Get(slotCommitmentID.Index(), true)

	buffer, _ := blockBufferForCommitments.GetOrCreate(slotCommitmentID, func() *ringbuffer.RingBuffer[*types.Tuple[*model.Block, network.PeerID]] {
		return ringbuffer.NewRingBuffer[*types.Tuple[*model.Block, network.PeerID]](blockBufferMaxSize)
	})
	buffer.Add(types.NewTuple(block, src))

	u.commitmentBufferMutex.Lock()
	defer u.commitmentBufferMutex.Unlock()
	u.commitmentBuffer.Put(slotCommitmentID, types.Void)

	return true
}

func (u *UnsolidCommitmentBlocks) evict(lastEvictedSlot iotago.SlotIndex) {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	u.lastEvictedSlot = lastEvictedSlot
	u.blockBuffers.Evict(lastEvictedSlot)
}

func (u *UnsolidCommitmentBlocks) GetBlocks(commitmentID iotago.CommitmentID) []*types.Tuple[*model.Block, network.PeerID] {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	if blockBufferForCommitments := u.blockBuffers.Get(commitmentID.Index()); blockBufferForCommitments != nil {
		if buffer, exists := blockBufferForCommitments.Get(commitmentID); exists {
			return buffer.Elements()
		}
	}

	return nil
}
