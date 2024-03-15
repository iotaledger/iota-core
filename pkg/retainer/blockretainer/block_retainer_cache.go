package blockretainer

import (
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/options"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type cache struct {
	uncommittedBlockMetadataChanges *shrinkingmap.ShrinkingMap[iotago.SlotIndex, map[iotago.BlockID]api.BlockState]
}

func newCache(opts ...options.Option[cache]) *cache {
	return options.Apply(&cache{
		uncommittedBlockMetadataChanges: shrinkingmap.New[iotago.SlotIndex, map[iotago.BlockID]api.BlockState](),
	}, opts)
}

// blockMetadataByID returns the block metadata of a block by its ID.
func (c *cache) blockMetadataByID(blockID iotago.BlockID) (api.BlockState, bool) {
	slotMap, exists := c.uncommittedBlockMetadataChanges.Get(blockID.Slot())
	if exists {
		blockMetadata, found := slotMap[blockID]
		if found {
			return blockMetadata, true
		}
	}

	return api.BlockStateUnknown, false
}

func (c *cache) setBlockMetadata(blockID iotago.BlockID, state api.BlockState) {
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
