package slotnotarization

import (
	"sync"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

// SlotMutations is an in-memory data structure that enables the collection of mutations for uncommitted slots.
type SlotMutations struct {
	weights *account.Accounts[iotago.AccountID, *iotago.AccountID]

	AcceptedBlockRemoved *event.Event1[iotago.BlockID]

	// acceptedBlocksBySlot stores the accepted blocks per slot.
	acceptedBlocksBySlot *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *ads.Set[iotago.BlockID, *iotago.BlockID]]

	// latestCommittedIndex stores the index of the latest committed slot.
	latestCommittedIndex iotago.SlotIndex

	evictionMutex sync.RWMutex
}

// NewSlotMutations creates a new SlotMutations instance.
func NewSlotMutations(weights *account.Accounts[iotago.AccountID, *iotago.AccountID], lastCommittedSlot iotago.SlotIndex) (newMutationFactory *SlotMutations) {
	return &SlotMutations{
		AcceptedBlockRemoved: event.New1[iotago.BlockID](),
		weights:              weights,
		acceptedBlocksBySlot: shrinkingmap.New[iotago.SlotIndex, *ads.Set[iotago.BlockID, *iotago.BlockID]](),
		latestCommittedIndex: lastCommittedSlot,
	}
}

// AddAcceptedBlock adds the given block to the set of accepted blocks.
func (m *SlotMutations) AddAcceptedBlock(block *blocks.Block) (err error) {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	blockID := block.ID()
	if blockID.Index() <= m.latestCommittedIndex {
		return errors.Errorf("cannot add block %s: slot with %d is already committed", blockID, blockID.Index())
	}

	m.acceptedBlocks(blockID.Index(), true).Add(blockID)

	return
}

// RemoveAcceptedBlock removes the given block from the set of accepted blocks.
func (m *SlotMutations) RemoveAcceptedBlock(block *blocks.Block) (err error) {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	blockID := block.ID()
	if blockID.Index() <= m.latestCommittedIndex {
		return errors.Errorf("cannot add block %s: slot with %d is already committed", blockID, blockID.Index())
	}

	m.acceptedBlocks(blockID.Index()).Delete(blockID)

	m.AcceptedBlockRemoved.Trigger(blockID)

	return
}

// Evict evicts the given slot and returns the corresponding mutation sets.
func (m *SlotMutations) Evict(index iotago.SlotIndex) (acceptedBlocks *ads.Set[iotago.BlockID, *iotago.BlockID], err error) {
	m.evictionMutex.Lock()
	defer m.evictionMutex.Unlock()

	if index <= m.latestCommittedIndex {
		return nil, errors.Errorf("cannot commit slot %d: already committed", index)
	}

	defer m.evictUntil(index)

	return m.acceptedBlocks(index), nil
}

func (m *SlotMutations) Reset(index iotago.SlotIndex) {
	m.evictionMutex.Lock()
	defer m.evictionMutex.Unlock()

	for i := m.latestCommittedIndex; i > index; i-- {
		m.acceptedBlocksBySlot.Delete(i)
	}

	m.latestCommittedIndex = index
}

// acceptedBlocks returns the set of accepted blocks for the given slot.
func (m *SlotMutations) acceptedBlocks(index iotago.SlotIndex, createIfMissing ...bool) *ads.Set[iotago.BlockID, *iotago.BlockID] {
	if len(createIfMissing) > 0 && createIfMissing[0] {
		return lo.Return1(m.acceptedBlocksBySlot.GetOrCreate(index, newSet[iotago.BlockID, *iotago.BlockID]))
	}

	return lo.Return1(m.acceptedBlocksBySlot.Get(index))
}

// evictUntil removes all data for slots that are older than the given slot.
func (m *SlotMutations) evictUntil(index iotago.SlotIndex) {
	for i := m.latestCommittedIndex + 1; i <= index; i++ {
		m.acceptedBlocksBySlot.Delete(i)
	}

	m.latestCommittedIndex = index
}

// newSet is a generic constructor for a new ads.Set.
func newSet[K any, KPtr serializer.MarshalablePtr[K]]() *ads.Set[K, KPtr] {
	return ads.NewSet[K, KPtr](mapdb.NewMapDB())
}
