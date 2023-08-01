package protocol

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/ringbuffer"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	iotago "github.com/iotaledger/iota.go/v4"
)

type UnsolidCommitmentBlocks struct {
	mutex           sync.RWMutex
	lastEvictedSlot iotago.SlotIndex
	blockBuffer     *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *ringbuffer.RingBuffer[*types.Tuple[*model.Block, network.PeerID]]]
}

func newUnsolidCommitmentBlocks() *UnsolidCommitmentBlocks {
	return &UnsolidCommitmentBlocks{
		blockBuffer: shrinkingmap.New[iotago.SlotIndex, *ringbuffer.RingBuffer[*types.Tuple[*model.Block, network.PeerID]]](),
	}
}

func (u *UnsolidCommitmentBlocks) AddBlock(block *model.Block, src network.PeerID) bool {
	u.mutex.RLock()
	defer u.mutex.RUnlock()
	commitmentSlot := block.ProtocolBlock().SlotCommitmentID.Index()

	if commitmentSlot < u.lastEvictedSlot {
		return false
	}

	// TODO: protect blockBuffer so that not arbitrary future blocks can be added to them
	buffer, _ := u.blockBuffer.GetOrCreate(commitmentSlot, func() *ringbuffer.RingBuffer[*types.Tuple[*model.Block, network.PeerID]] {
		return ringbuffer.NewRingBuffer[*types.Tuple[*model.Block, network.PeerID]](100)
	})
	buffer.Add(types.NewTuple(block, src))

	return true
}

func (u *UnsolidCommitmentBlocks) evict(lastEvictedSlot iotago.SlotIndex) {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	u.lastEvictedSlot = lastEvictedSlot
	u.blockBuffer.Delete(lastEvictedSlot)
}

func (u *UnsolidCommitmentBlocks) GetBlocks(slot iotago.SlotIndex) []*types.Tuple[*model.Block, network.PeerID] {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	buffer, deleted := u.blockBuffer.DeleteAndReturn(slot)
	if !deleted {
		return nil
	}

	return buffer.Elements()
}
