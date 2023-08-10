package engine

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
)

// CommittedSlotAPI is a wrapper for the Engine that provides access to the data of a committed slot.
type CommittedSlotAPI struct {
	// engine is the Engine that is used to access the data.
	engine *Engine

	// slotIndex is the index of the slot that is accessed.
	slotIndex iotago.SlotIndex
}

// NewCommittedSlotAPI creates a new CommittedSlotAPI.
func NewCommittedSlotAPI(engine *Engine, slotIndex iotago.SlotIndex) *CommittedSlotAPI {
	return &CommittedSlotAPI{
		engine:    engine,
		slotIndex: slotIndex,
	}
}

// Commitment returns the commitment of the slot.
func (c *CommittedSlotAPI) Commitment() (commitment *model.Commitment, err error) {
	if commitment, err = c.engine.Storage.Commitments().Load(c.slotIndex); err != nil {
		return nil, ierrors.Wrapf(err, "failed to load commitment for slot %d", c.slotIndex)
	}

	return commitment, nil
}

// Roots returns the roots of the slot.
func (c *CommittedSlotAPI) Roots() (roots iotago.Roots, err error) {
	if c.engine.Storage.Settings().LatestCommitment().Index() < c.slotIndex {
		return roots, ierrors.Errorf("slot %d is not committed yet", c.slotIndex)
	}

	rootsStorage := c.engine.Storage.Roots(c.slotIndex)
	if rootsStorage == nil {
		return roots, ierrors.Errorf("no roots storage for slot %d", c.slotIndex)
	}

	rootsBytes, err := rootsStorage.Get(kvstore.Key{prunable.RootsKey})
	if err != nil {
		return roots, ierrors.Wrapf(err, "failed to load roots for slot %d", c.slotIndex)
	}

	_, err = c.engine.APIForSlot(c.slotIndex).Decode(rootsBytes, &roots)
	if err != nil {
		return roots, ierrors.Wrapf(err, "failed to decode roots for slot %d", c.slotIndex)
	}

	return roots, nil
}

// BlockIDs returns the accepted block IDs of the slot.
func (c *CommittedSlotAPI) BlockIDs() (blockIDs iotago.BlockIDs, err error) {
	if c.engine.Storage.Settings().LatestCommitment().Index() < c.slotIndex {
		return blockIDs, ierrors.Errorf("slot %d is not committed yet", c.slotIndex)
	}

	if err = c.engine.Storage.Blocks(c.slotIndex).ForEachBlockIDInSlot(func(blockID iotago.BlockID) error {
		blockIDs = append(blockIDs, blockID)
		return nil
	}); err != nil {
		return nil, ierrors.Wrapf(err, "failed to iterate over blocks in slot %d", c.slotIndex)
	}

	return blockIDs, nil
}
