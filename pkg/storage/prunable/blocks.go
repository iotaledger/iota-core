package prunable

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Blocks struct {
	Storage func(index iotago.SlotIndex) kvstore.KVStore

	api iotago.API
}

func NewBlocks(dbManager *Manager, storagePrefix byte) (newBlocks *Blocks) {
	return &Blocks{
		Storage: lo.Bind([]byte{storagePrefix}, dbManager.Get),
		api:     iotago.V3API(&iotago.ProtocolParameters{}), // TODO: do we need the protocol parameters for the storage?
	}
}

func (b *Blocks) Load(id iotago.BlockID) (*model.Block, error) {
	storage := b.Storage(id.Index())
	if storage == nil {
		return nil, errors.Errorf("storage does not exist for slot %s", id.Index())
	}

	blockBytes, err := storage.Get(id[:])
	if err != nil {
		if errors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, nil
		}

		return nil, errors.Wrapf(err, "failed to get block %s", id)
	}

	return model.BlockFromBlockIDAndBytes(id, blockBytes, b.api)
}

func (b *Blocks) Store(block *model.Block) error {
	blockID := block.ID()
	storage := b.Storage(blockID.Index())
	if storage == nil {
		return errors.Errorf("storage does not exist for slot %s", blockID.Index())
	}

	if err := storage.Set(blockID[:], block.Data()); err != nil {
		return errors.Wrapf(err, "failed to store block %s", block.ID)
	}

	return nil
}

func (b *Blocks) Delete(id iotago.BlockID) (err error) {
	storage := b.Storage(id.Index())
	if storage == nil {
		return errors.Errorf("storage does not exist for slot %s", id.Index())
	}

	if err = storage.Delete(id[:]); err != nil {
		return errors.Wrapf(err, "failed to delete block %s", id)
	}

	return nil
}

func (b *Blocks) ForEachBlockInSlot(index iotago.SlotIndex, consumer func(blockID iotago.BlockID) bool) error {
	storage := b.Storage(index)
	if storage == nil {
		return errors.Errorf("storage does not exist for slot %s", index)
	}

	var innerErr error
	if err := storage.IterateKeys(kvstore.EmptyPrefix, func(key kvstore.Key) bool {
		var blockID iotago.BlockID
		blockID, innerErr = iotago.SlotIdentifierFromBytes(key)
		if innerErr != nil {
			return false
		}
		return consumer(blockID)
	}); err != nil {
		return errors.Wrapf(err, "failed to stream blockIDs for slot %s", index)
	}

	if innerErr != nil {
		return errors.Wrapf(innerErr, "failed to deserialize blockIDs for slot %s", index)
	}

	return nil
}
