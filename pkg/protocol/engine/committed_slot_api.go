package engine

import (
	"github.com/iotaledger/hive.go/ads"
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
	if commitment, err = c.engine.Storage.Commitments().Load(c.CommitmentID.Slot()); err != nil {
		return nil, ierrors.Wrapf(err, "failed to load commitment for slot %d", c.CommitmentID)
	}

	return commitment, nil
}

// Roots returns the roots of the slot.
func (c *CommittedSlotAPI) Roots() (committedRoots *iotago.Roots, err error) {
	if c.engine.Storage.Settings().LatestCommitment().Slot() < c.CommitmentID.Slot() {
		return nil, ierrors.Errorf("slot %d is not committed yet", c.CommitmentID)
	}

	rootsStorage, err := c.engine.Storage.Roots(c.CommitmentID.Slot())
	if err != nil {
		return nil, ierrors.Errorf("no roots storage for slot %d", c.CommitmentID)
	}

	roots, _, err := rootsStorage.Load(c.CommitmentID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to load roots for slot %d", c.CommitmentID)
	}

	return roots, nil
}

// BlockIDs returns the accepted block IDs of the slot.
func (c *CommittedSlotAPI) BlockIDs() (blockIDs iotago.BlockIDs, err error) {
	if c.engine.Storage.Settings().LatestCommitment().Slot() < c.CommitmentID.Slot() {
		return blockIDs, ierrors.Errorf("slot %d is not committed yet", c.CommitmentID)
	}

	store, err := c.engine.Storage.Blocks(c.CommitmentID.Slot())
	if err != nil {
		return nil, ierrors.Errorf("failed to get block store of slot index %d", c.CommitmentID.Slot())
	}

	if err := store.ForEachBlockInSlot(func(block *model.Block) error {
		blockIDs = append(blockIDs, block.ID())
		return nil
	}); err != nil {
		return nil, ierrors.Wrapf(err, "failed to iterate over blocks of slot %d", c.CommitmentID.Slot())
	}

	return blockIDs, nil
}

func (c *CommittedSlotAPI) TransactionIDs() (iotago.TransactionIDs, error) {
	if c.engine.Storage.Settings().LatestCommitment().Slot() < c.CommitmentID.Slot() {
		return nil, ierrors.Errorf("slot %d is not committed yet", c.CommitmentID)
	}

	store, err := c.engine.Storage.Mutations(c.CommitmentID.Slot())
	if err != nil {
		return nil, ierrors.Errorf("failed to get mutations of slot index %d", c.CommitmentID.Slot())
	}

	set := ads.NewSet[iotago.Identifier](store, iotago.TransactionID.Bytes, iotago.TransactionIDFromBytes)
	transactionIDs := make(iotago.TransactionIDs, 0, set.Size())

	if err = set.Stream(func(txID iotago.TransactionID) error {
		transactionIDs = append(transactionIDs, txID)
		return nil
	}); err != nil {
		return nil, ierrors.Wrapf(err, "failed to iterate over mutations of slot %d", c.CommitmentID.Slot())
	}

	return transactionIDs, nil
}
