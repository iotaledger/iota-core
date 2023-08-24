package engine

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

// CommittedSlotAPI is a wrapper for the Engine that provides access to the data of a committed slot.
type CommittedSlotAPI struct {
	// engine is the Engine that is used to access the data.
	engine *Engine

	// CommitmentID is the index of the slot that is accessed.
	CommitmentID iotago.CommitmentID
}

// NewCommittedSlotAPI creates a new CommittedSlotAPI.
func NewCommittedSlotAPI(engine *Engine, commitmentID iotago.CommitmentID) *CommittedSlotAPI {
	return &CommittedSlotAPI{
		engine:       engine,
		CommitmentID: commitmentID,
	}
}

// Commitment returns the commitment of the slot.
func (c *CommittedSlotAPI) Commitment() (commitment *model.Commitment, err error) {
	if commitment, err = c.engine.Storage.Commitments().Load(c.CommitmentID.Index()); err != nil {
		return nil, ierrors.Wrapf(err, "failed to load commitment for slot %d", c.CommitmentID)
	}

	return commitment, nil
}

// Roots returns the roots of the slot.
func (c *CommittedSlotAPI) Roots() (committedRoots *iotago.Roots, err error) {
	if c.engine.Storage.Settings().LatestCommitment().Index() < c.CommitmentID.Index() {
		return nil, ierrors.Errorf("slot %d is not committed yet", c.CommitmentID)
	}

	rootsStorage, err := c.engine.Storage.Roots(c.CommitmentID.Index())
	if err != nil {
		return nil, ierrors.Errorf("no roots storage for slot %d", c.CommitmentID)
	}

	roots, err := rootsStorage.Load(c.CommitmentID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to load roots for slot %d", c.CommitmentID)
	}

	return roots, nil
}

// BlockIDs returns the accepted block IDs of the slot.
func (c *CommittedSlotAPI) BlockIDs() (blockIDs iotago.BlockIDs, err error) {
	if c.engine.Storage.Settings().LatestCommitment().Index() < c.CommitmentID.Index() {
		return blockIDs, ierrors.Errorf("slot %d is not committed yet", c.CommitmentID)
	}

	store, err := c.engine.Storage.Blocks(c.CommitmentID.Index())
	if err != nil {
		return nil, ierrors.Errorf("failed to get block store of slot index %d", c.CommitmentID.Index())
	}

	store.ForEachBlockInSlot(func(block *model.Block) error {
		blockIDs = append(blockIDs, block.ID())
		return nil
	})

	return blockIDs, nil
}
