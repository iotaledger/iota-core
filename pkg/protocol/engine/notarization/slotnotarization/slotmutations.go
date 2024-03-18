package slotnotarization

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

// SlotMutations is an in-memory data structure that enables the collection of mutations for uncommitted slots.
type SlotMutations struct {
	// acceptedBlocksBySlot stores the accepted blocks per slot.
	acceptedBlocksBySlot *shrinkingmap.ShrinkingMap[iotago.SlotIndex, ads.Set[iotago.Identifier, iotago.BlockID]]

	// latestCommittedSlot stores the index of the latest committed slot.
	latestCommittedSlot iotago.SlotIndex

	evictionMutex syncutils.RWMutex
}

// NewSlotMutations creates a new SlotMutations instance.
func NewSlotMutations(lastCommittedSlot iotago.SlotIndex) *SlotMutations {
	return &SlotMutations{
		acceptedBlocksBySlot: shrinkingmap.New[iotago.SlotIndex, ads.Set[iotago.Identifier, iotago.BlockID]](),
		latestCommittedSlot:  lastCommittedSlot,
	}
}

// AddAcceptedBlock adds the given block to the set of accepted blocks.
func (m *SlotMutations) AddAcceptedBlock(block *blocks.Block) (err error) {
	m.evictionMutex.RLock()
	defer m.evictionMutex.RUnlock()

	blockID := block.ID()
	if blockID.Slot() <= m.latestCommittedSlot {
		return ierrors.Errorf("cannot add block %s: slot with %d is already committed", blockID, blockID.Slot())
	}

	if err := m.acceptedBlocks(blockID.Slot(), true).Add(blockID); err != nil {
		return ierrors.Wrapf(err, "failed to add block %s to accepted blocks", blockID)
	}

	return
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (m *SlotMutations) Reset() {
	m.acceptedBlocksBySlot.Clear()
}

func (m *SlotMutations) Commit(slot iotago.SlotIndex) (ads.Set[iotago.Identifier, iotago.BlockID], error) {
	m.evictionMutex.Lock()
	defer m.evictionMutex.Unlock()

	// If for whatever reason no blocks exist (empty slot), then we still want to create the ads.Set and get the root from it.
	acceptedBlocksSet := m.acceptedBlocks(slot, true)

	if err := acceptedBlocksSet.Commit(); err != nil {
		return nil, ierrors.Wrap(err, "failed to commit accepted blocks")
	}

	m.acceptedBlocksBySlot.Delete(slot)
	m.latestCommittedSlot = slot

	return acceptedBlocksSet, nil
}

// acceptedBlocks returns the set of accepted blocks for the given slot.
func (m *SlotMutations) acceptedBlocks(slot iotago.SlotIndex, createIfMissing ...bool) ads.Set[iotago.Identifier, iotago.BlockID] {
	if len(createIfMissing) > 0 && createIfMissing[0] {
		return lo.Return1(m.acceptedBlocksBySlot.GetOrCreate(slot, func() ads.Set[iotago.Identifier, iotago.BlockID] {
			return ads.NewSet[iotago.Identifier](
				mapdb.NewMapDB(),
				iotago.Identifier.Bytes,
				iotago.IdentifierFromBytes,
				iotago.BlockID.Bytes,
				iotago.BlockIDFromBytes,
			)
		}))
	}

	return lo.Return1(m.acceptedBlocksBySlot.Get(slot))
}

func (m *SlotMutations) AcceptedBlocksCount(index iotago.SlotIndex) int {
	acceptedBlocks, exists := m.acceptedBlocksBySlot.Get(index)
	if !exists {
		return 0
	}

	return acceptedBlocks.Size()
}
