package drr

import (
	"container/heap"
	"fmt"
	"time"

	"go.uber.org/atomic"

	"github.com/iotaledger/hive.go/ds/generalheap"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/timed"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"

	iotago "github.com/iotaledger/iota.go/v4"
)

type ValidatorQueue struct {
	accountID iotago.AccountID
	submitted *shrinkingmap.ShrinkingMap[iotago.BlockID, *blocks.Block]
	inbox     generalheap.Heap[timed.HeapKey, *blocks.Block]
	size      atomic.Int64

	tokenBucket      float64
	lastScheduleTime time.Time

	blockChan      chan *blocks.Block
	shutdownSignal chan struct{}
}

func NewValidatorQueue(accountID iotago.AccountID) *ValidatorQueue {
	return &ValidatorQueue{
		accountID:        accountID,
		submitted:        shrinkingmap.New[iotago.BlockID, *blocks.Block](),
		blockChan:        make(chan *blocks.Block, 1),
		shutdownSignal:   make(chan struct{}),
		tokenBucket:      1,
		lastScheduleTime: time.Now(),
	}
}

func (q *ValidatorQueue) Size() int {
	if q == nil {
		return 0
	}

	return int(q.size.Load())
}

func (q *ValidatorQueue) AccountID() iotago.AccountID {
	return q.accountID
}

func (q *ValidatorQueue) Submit(block *blocks.Block, maxBuffer int) (*blocks.Block, bool) {
	if blkAccountID := block.ProtocolBlock().IssuerID; q.accountID != blkAccountID {
		panic(fmt.Sprintf("issuerqueue: queue issuer ID(%x) and issuer ID(%x) does not match.", q.accountID, blkAccountID))
	}

	if _, submitted := q.submitted.Get(block.ID()); submitted {
		return nil, false
	}

	q.submitted.Set(block.ID(), block)
	q.size.Inc()

	if int(q.size.Load()) > maxBuffer {
		return q.RemoveTail(), true
	}

	return nil, true
}

func (q *ValidatorQueue) Unsubmit(block *blocks.Block) bool {
	if _, submitted := q.submitted.Get(block.ID()); !submitted {
		return false
	}

	q.submitted.Delete(block.ID())
	q.size.Dec()

	return true
}

func (q *ValidatorQueue) Ready(block *blocks.Block) bool {
	if _, submitted := q.submitted.Get(block.ID()); !submitted {
		return false
	}

	q.submitted.Delete(block.ID())
	heap.Push(&q.inbox, &generalheap.HeapElement[timed.HeapKey, *blocks.Block]{Value: block, Key: timed.HeapKey(block.IssuingTime())})

	return true
}

// PopFront removes the first ready block from the queue.
func (q *ValidatorQueue) PopFront() *blocks.Block {
	if q.inbox.Len() == 0 {
		return nil
	}

	heapElement, isHeapElement := heap.Pop(&q.inbox).(*generalheap.HeapElement[timed.HeapKey, *blocks.Block])
	if !isHeapElement {
		return nil
	}
	blk := heapElement.Value
	q.size.Dec()

	return blk
}

func (q *ValidatorQueue) RemoveTail() *blocks.Block {
	var oldestSubmittedBlock *blocks.Block
	q.submitted.ForEach(func(_ iotago.BlockID, block *blocks.Block) bool {
		if oldestSubmittedBlock == nil || oldestSubmittedBlock.IssuingTime().After(block.IssuingTime()) {
			oldestSubmittedBlock = block
		}

		return true
	})

	tail := q.tail()
	// if heap tail does not exist or tail is newer than oldest submitted block, unsubmit oldest block
	if oldestSubmittedBlock != nil && (tail < 0 || q.inbox[tail].Key.CompareTo(timed.HeapKey(oldestSubmittedBlock.IssuingTime())) > 0) {
		q.Unsubmit(oldestSubmittedBlock)

		return oldestSubmittedBlock
	} else if tail < 0 {
		// should never happen that the oldest submitted block does not exist and the tail does not exist.
		return nil
	}

	// if the tail exists and is older than the oldest submitted block, drop it
	heapElement, isHeapElement := heap.Remove(&q.inbox, tail).(*generalheap.HeapElement[timed.HeapKey, *blocks.Block])
	if !isHeapElement {
		return nil
	}
	blk := heapElement.Value
	q.size.Dec()

	return blk
}

func (q *ValidatorQueue) tail() int {
	h := q.inbox
	if h.Len() <= 0 {
		return -1
	}
	tail := 0
	for i := range h {
		if !h.Less(i, tail) { // less means older issue time
			tail = i
		}
	}

	return tail
}

func (q *ValidatorQueue) waitTime(rate float64) time.Duration {
	tokensRequired := 1 - (q.tokenBucket + rate*time.Since(q.lastScheduleTime).Seconds())

	return lo.Max(0, time.Duration(tokensRequired/rate))
}

func (q *ValidatorQueue) updateTokenBucket(rate float64, tokenBucketSize float64) {
	q.tokenBucket = lo.Min(
		tokenBucketSize,
		q.tokenBucket+rate*time.Since(q.lastScheduleTime).Seconds(),
	)
	q.lastScheduleTime = time.Now()
}

func (q *ValidatorQueue) deductTokens(tokens float64) {
	q.tokenBucket -= tokens
}

type ValidatorBuffer struct {
	buffer *shrinkingmap.ShrinkingMap[iotago.AccountID, *ValidatorQueue]
	size   int
}

func NewValidatorBuffer() *ValidatorBuffer {
	return &ValidatorBuffer{
		buffer: shrinkingmap.New[iotago.AccountID, *ValidatorQueue](),
	}
}

func (b *ValidatorBuffer) Size() int {
	if b == nil {
		return 0
	}

	return b.size
}

func (b *ValidatorBuffer) Get(accountID iotago.AccountID) (*ValidatorQueue, bool) {
	return b.buffer.Get(accountID)
}

func (b *ValidatorBuffer) Set(accountID iotago.AccountID, validatorQueue *ValidatorQueue) bool {
	return b.buffer.Set(accountID, validatorQueue)
}

func (b *ValidatorBuffer) Submit(block *blocks.Block, maxBuffer int) (*blocks.Block, bool) {
	validatorQueue, exists := b.buffer.Get(block.ProtocolBlock().IssuerID)
	if !exists {
		return nil, false
	}
	droppedBlock, submitted := validatorQueue.Submit(block, maxBuffer)
	if submitted {
		b.size++
	}
	if droppedBlock != nil {
		b.size--
	}

	return droppedBlock, submitted
}

func (b *ValidatorBuffer) Delete(accountID iotago.AccountID) {
	validatorQueue, exists := b.buffer.Get(accountID)
	if !exists {
		return
	}
	b.size -= validatorQueue.Size()

	b.buffer.Delete(accountID)
}
