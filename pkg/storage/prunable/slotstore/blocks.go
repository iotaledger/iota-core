package slotstore

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Blocks struct {
	slot  iotago.SlotIndex
	store kvstore.KVStore

	apiForSlot iotago.API
}

func NewBlocks(slot iotago.SlotIndex, store kvstore.KVStore, apiForSlot iotago.API) (newBlocks *Blocks) {
	return &Blocks{
		slot:       slot,
		store:      store,
		apiForSlot: apiForSlot,
	}
}

func (b *Blocks) Load(id iotago.BlockID) (*model.Block, error) {
	blockBytes, err := b.store.Get(id[:])
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			//nolint:nilnil // expected behavior
			return nil, nil
		}

		return nil, ierrors.Wrapf(err, "failed to get block %s", id)
	}

	return model.BlockFromIDAndBytes(id, blockBytes, b.apiForSlot)
}

func (b *Blocks) Store(block *model.Block) error {
	blockID := block.ID()
	return b.store.Set(blockID[:], block.Data())
}

func (b *Blocks) Delete(id iotago.BlockID) (err error) {
	return b.store.Delete(id[:])
}

func (b *Blocks) StreamKeys(consumer func(blockID iotago.BlockID) error) error {
	var innerErr error
	if err := b.store.IterateKeys(kvstore.EmptyPrefix, func(key kvstore.Key) bool {
		var blockID iotago.BlockID
		blockID, _, innerErr = iotago.BlockIDFromBytes(key)
		if innerErr != nil {
			return false
		}

		return consumer(blockID) == nil
	}); err != nil {
		return ierrors.Wrapf(err, "failed to stream blockIDs for slot %s", b.slot)
	}

	if innerErr != nil {
		return ierrors.Wrapf(innerErr, "failed to deserialize blockIDs for slot %s", b.slot)
	}

	return nil
}

func (b *Blocks) ForEachBlockInSlot(consumer func(block *model.Block) error) error {
	var innerErr error
	if err := b.store.Iterate(kvstore.EmptyPrefix, func(key kvstore.Key, value kvstore.Value) bool {
		var id iotago.BlockID
		id, _, innerErr = iotago.BlockIDFromBytes(key)
		var block *model.Block
		block, innerErr = model.BlockFromIDAndBytes(id, value, b.apiForSlot)

		if innerErr != nil {
			return false
		}

		return consumer(block) == nil
	}); err != nil {
		return ierrors.Wrapf(err, "failed to stream blocks for slot %s", b.slot)
	}

	if innerErr != nil {
		return ierrors.Wrapf(innerErr, "failed to deserialize blocks for slot %s", b.slot)
	}

	return nil
}

// Clear clears the storage.
func (b *Blocks) Clear() error {
	return b.store.Clear()
}
