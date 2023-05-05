package slotnotarization

import (
	"sync"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

// SlotMutations is an in-memory data structure that enables the collection of mutations for uncommitted slots.
type SlotMutations struct {
	weights *account.Accounts[iotago.AccountID, *iotago.AccountID]

	// ratifiedAcceptedBlocksBySlot stores the accepted blocks per slot.
	ratifiedAcceptedBlocksBySlot *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *ads.Set[iotago.BlockID, *iotago.BlockID]]

	// acceptedTransactionsBySlot stores the accepted transactions per slot.
	acceptedTransactionsBySlot *shrinkingmap.ShrinkingMap[slot.Index, *ads.Set[utxo.TransactionID, *utxo.TransactionID]]

	// latestCommittedIndex stores the index of the latest committed slot.
	latestCommittedIndex iotago.SlotIndex

	evictionMutex sync.RWMutex
}

// TODO: weights should be replaced with accounts
// NewSlotMutations creates a new SlotMutations instance.
func NewSlotMutations(weights *account.Accounts[iotago.AccountID, *iotago.AccountID], lastCommittedSlot iotago.SlotIndex) (newMutationFactory *SlotMutations) {
	return &SlotMutations{
		weights:                      weights,
		ratifiedAcceptedBlocksBySlot: shrinkingmap.New[iotago.SlotIndex, *ads.Set[iotago.BlockID, *iotago.BlockID]](),
		latestCommittedIndex:         lastCommittedSlot,
	}
}

// AddRatifiedAcceptedBlock adds the given block to the set of accepted blocks.
func (m *SlotMutations) AddRatifiedAcceptedBlock(block *blocks.Block) (err error) {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	blockID := block.ID()
	if blockID.Index() <= m.latestCommittedIndex {
		return errors.Errorf("cannot add block %s: slot with %d is already committed", blockID, blockID.Index())
	}

	m.RatifiedAcceptedBlocks(blockID.Index(), true).Add(blockID)

	return
}

// Evict evicts the given slot and returns the corresponding mutation sets.
func (m *SlotMutations) Evict(index iotago.SlotIndex) error {
	m.evictionMutex.Lock()
	defer m.evictionMutex.Unlock()

	if index <= m.latestCommittedIndex {
		return errors.Errorf("cannot commit slot %d: already committed", index)
	}

	m.evictUntil(index)

	return nil
}

func (m *SlotMutations) Reset(index iotago.SlotIndex) {
	m.evictionMutex.Lock()
	defer m.evictionMutex.Unlock()

	for i := m.latestCommittedIndex; i > index; i-- {
		m.ratifiedAcceptedBlocksBySlot.Delete(i)
	}

	m.latestCommittedIndex = index
}

// RatifiedAcceptedBlocks returns the set of ratified accepted blocks for the given slot.
func (m *SlotMutations) RatifiedAcceptedBlocks(index iotago.SlotIndex, createIfMissing ...bool) *ads.Set[iotago.BlockID, *iotago.BlockID] {
	if len(createIfMissing) > 0 && createIfMissing[0] {
		return lo.Return1(m.ratifiedAcceptedBlocksBySlot.GetOrCreate(index, newSet[iotago.BlockID, *iotago.BlockID]))
	}

	return lo.Return1(m.ratifiedAcceptedBlocksBySlot.Get(index))
}

// evictUntil removes all data for slots that are older than the given slot.
func (m *SlotMutations) evictUntil(index iotago.SlotIndex) {
	for i := m.latestCommittedIndex + 1; i <= index; i++ {
		m.ratifiedAcceptedBlocksBySlot.Delete(i)
	}

	m.latestCommittedIndex = index
}

// newSet is a generic constructor for a new ads.Set.
func newSet[K any, KPtr serializer.MarshalablePtr[K]]() *ads.Set[K, KPtr] {
	return ads.NewSet[K, KPtr](mapdb.NewMapDB())
}
