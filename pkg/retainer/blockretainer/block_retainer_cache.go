package blockretainer

import (
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type cache struct {
	uncommittedBlockMetadata *shrinkingmap.ShrinkingMap[iotago.SlotIndex, map[iotago.BlockID]api.BlockState]
}

func newCache() *cache {
	return &cache{
		uncommittedBlockMetadata: shrinkingmap.New[iotago.SlotIndex, map[iotago.BlockID]api.BlockState](),
	}
}

// blockMetadataByID returns the block metadata of a block by its ID.
func (c *cache) blockMetadataByID(blockID iotago.BlockID) (api.BlockState, bool) {
	slotMap, exists := c.uncommittedBlockMetadata.Get(blockID.Slot())
	if exists {
		blockMetadata, found := slotMap[blockID]
		if found {
			return blockMetadata, true
		}
	}

	return api.BlockStateUnknown, false
}

func (c *cache) setBlockMetadata(blockID iotago.BlockID, state api.BlockState) {
	blocks, _ := c.uncommittedBlockMetadata.GetOrCreate(blockID.Slot(), func() map[iotago.BlockID]api.BlockState {
		return make(map[iotago.BlockID]api.BlockState)
	})

	prevState, ok := blocks[blockID]
	if ok && state == api.BlockStateDropped {
		if prevState == api.BlockStateAccepted || state == api.BlockStateConfirmed {
			// do not overwrite acceptance with the local congestion dropped event
			return
		}
	}

	blocks[blockID] = state
}
