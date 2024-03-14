package blockretainer

import (
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/options"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type BlockRetainerCache struct {
	uncommittedBlockMetadataChanges *shrinkingmap.ShrinkingMap[iotago.SlotIndex, map[iotago.BlockID]api.BlockState]
}

func NewBlockRetainerCache(opts ...options.Option[BlockRetainerCache]) *BlockRetainerCache {
	return options.Apply(&BlockRetainerCache{
		uncommittedBlockMetadataChanges: shrinkingmap.New[iotago.SlotIndex, map[iotago.BlockID]api.BlockState](),
	}, opts)
}

// blockMetadataByID returns the block metadata of a block by its ID.
func (c *BlockRetainerCache) blockMetadataByID(blockID iotago.BlockID) (api.BlockState, bool) {
	slotMap, exists := c.uncommittedBlockMetadataChanges.Get(blockID.Slot())
	if exists {
		blockMetadata, found := slotMap[blockID]
		if found {
			return blockMetadata, true
		}
	}

	return api.BlockStateUnknown, false
}

func (c *BlockRetainerCache) setBlockMetadata(blockID iotago.BlockID, state api.BlockState) {
	blocks, exists := c.uncommittedBlockMetadataChanges.Get(blockID.Slot())
	if !exists {
		blocks = make(map[iotago.BlockID]api.BlockState)
	}

	prevState, ok := blocks[blockID]
	if ok && state == api.BlockStateDropped {
		if prevState == api.BlockStateAccepted || state == api.BlockStateConfirmed {
			// do not overwrite acceptance with the local congestion dropped event
			return
		}
	}

	blocks[blockID] = state
	c.uncommittedBlockMetadataChanges.Set(blockID.Slot(), blocks)
}
