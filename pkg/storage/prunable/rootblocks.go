package prunable

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/kvstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

type RootBlocks struct {
	slot  iotago.SlotIndex
	store *kvstore.TypedStore[iotago.BlockID, iotago.CommitmentID]
}

// NewRootBlocks creates a new RootBlocks instance.
func NewRootBlocks(slot iotago.SlotIndex, store kvstore.KVStore) *RootBlocks {
	return &RootBlocks{
		slot: slot,
		store: kvstore.NewTypedStore[iotago.BlockID, iotago.CommitmentID](store,
			iotago.SlotIdentifier.Bytes,
			iotago.SlotIdentifierFromBytes,
			iotago.SlotIdentifier.Bytes,
			iotago.SlotIdentifierFromBytes,
		),
	}
}

// Store stores the given blockID as a root block.
func (r *RootBlocks) Store(id iotago.BlockID, commitmentID iotago.CommitmentID) (err error) {
	return r.store.Set(id, commitmentID)
}

// Load loads the root block ID and commitmentID for the given blockID.
func (r *RootBlocks) Load(blockID iotago.BlockID) (iotago.BlockID, iotago.CommitmentID, error) {
	commitmentID, err := r.store.Get(blockID)
	if err != nil {
		return iotago.EmptyBlockID(), iotago.CommitmentID{}, errors.Wrapf(err, "failed to get root block %s", blockID)
	}

	return blockID, commitmentID, nil
}

// Has returns true if the given blockID is a root block.
func (r *RootBlocks) Has(blockID iotago.BlockID) (has bool, err error) {
	return r.store.Has(blockID)
}

// Delete deletes the given blockID from the root blocks.
func (r *RootBlocks) Delete(blockID iotago.BlockID) (err error) {
	return r.store.Delete(blockID)
}

// Stream streams all root blocks for a slot index.
func (r *RootBlocks) Stream(consumer func(id iotago.BlockID, commitmentID iotago.CommitmentID) error) error {
	var innerErr error
	if storageErr := r.store.Iterate(kvstore.EmptyPrefix, func(blockID iotago.BlockID, commitmentID iotago.CommitmentID) (advance bool) {
		innerErr = consumer(blockID, commitmentID)
		return innerErr != nil
	}); storageErr != nil {
		return errors.Wrapf(storageErr, "failed to iterate over rootblocks for slot %s", r.slot)
	}

	return innerErr
}
