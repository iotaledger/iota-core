package prunable

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/database"
	iotago "github.com/iotaledger/iota.go/v4"
)

type RootBlocks struct {
	Storage func(index iotago.SlotIndex) kvstore.KVStore
}

// NewRootBlocks creates a new RootBlocks instance.
func NewRootBlocks(databaseInstance *database.Manager, storagePrefix byte) (newRootBlocks *RootBlocks) {
	return &RootBlocks{
		Storage: lo.Bind([]byte{storagePrefix}, databaseInstance.Get),
	}
}

// Store stores the given blockID as a root block.
func (r *RootBlocks) Store(id iotago.BlockID, commitmentID iotago.CommitmentID) (err error) {
	if err = r.Storage(id.Index()).Set(id[:], commitmentID[:]); err != nil {
		return errors.Wrapf(err, "failed to store solid entry point block %s with commitment %s", id, commitmentID)
	}

	return nil
}

// Has returns true if the given blockID is a root block.
func (r *RootBlocks) Has(blockID iotago.BlockID) (has bool, err error) {
	has, err = r.Storage(blockID.Index()).Has(blockID[:])
	if err != nil {
		return false, errors.Wrapf(err, "failed to delete solid entry point block %s", blockID)
	}

	return has, nil
}

// Delete deletes the given blockID from the root blocks.
func (r *RootBlocks) Delete(blockID iotago.BlockID) (err error) {
	if err = r.Storage(blockID.Index()).Delete(blockID[:]); err != nil {
		return errors.Wrapf(err, "failed to delete solid entry point block %s", blockID)
	}

	return nil
}

// Stream streams all root blocks for a slot index
func (r *RootBlocks) Stream(index iotago.SlotIndex, processor func(id iotago.BlockID, commitmentID iotago.CommitmentID) error) (err error) {
	if storageErr := r.Storage(index).Iterate([]byte{}, func(blockIDBytes kvstore.Key, commitmentIDBytes kvstore.Value) bool {
		var id iotago.BlockID
		var commitmentID iotago.CommitmentID
		if id, err = iotago.SlotIdentifierFromBytes(blockIDBytes); err != nil {
			err = errors.Wrapf(err, "failed to parse blockID %s", blockIDBytes)
		} else if commitmentID, err = iotago.SlotIdentifierFromBytes(commitmentIDBytes); err != nil {
			err = errors.Wrapf(err, "failed to parse commitmentID %s", commitmentIDBytes)
		} else if err = processor(id, commitmentID); err != nil {
			err = errors.Wrapf(err, "failed to process root block %s with commitment %s", id, commitmentID)
		}

		return err == nil
	}); storageErr != nil {
		return errors.Wrapf(storageErr, "failed to iterate over rootblocks")
	}

	return err
}
