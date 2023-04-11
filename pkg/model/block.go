package model

import (
	"fmt"
	"sync"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Block struct {
	api iotago.API

	// Key
	blockID iotago.BlockID

	// Value
	data      []byte
	blockOnce sync.Once
	block     *iotago.Block
}

func BlockFromBlock(iotaBlock *iotago.Block, api iotago.API, timeProvider *iotago.SlotTimeProvider, opts ...serix.Option) (*Block, error) {
	data, err := api.Encode(iotaBlock, opts...)
	if err != nil {
		return nil, err
	}

	blockID, err := iotaBlock.ID(timeProvider)
	if err != nil {
		return nil, err
	}

	block := &Block{
		api:     api,
		blockID: blockID,
		data:    data,
	}

	block.blockOnce.Do(func() {
		block.block = iotaBlock
	})

	return block, nil
}

func BlockFromBlockIDAndBytes(blockID iotago.BlockID, data []byte, api iotago.API, opts ...serix.Option) (*Block, error) {
	iotaBlock := new(iotago.Block)
	if _, err := api.Decode(data, iotaBlock, opts...); err != nil {
		return nil, err
	}

	block := &Block{
		api:     api,
		blockID: blockID,
		data:    data,
	}

	block.blockOnce.Do(func() {
		block.block = iotaBlock
	})

	return block, nil
}

func BlockFromBytes(data []byte, api iotago.API, timeProvider *iotago.SlotTimeProvider, opts ...serix.Option) (*Block, error) {
	iotaBlock := new(iotago.Block)
	if _, err := api.Decode(data, iotaBlock, opts...); err != nil {
		return nil, err
	}

	blockID, err := iotaBlock.ID(timeProvider)
	if err != nil {
		return nil, err
	}

	return BlockFromBlockIDAndBytes(blockID, data, api, opts...)
}

func (blk *Block) ID() iotago.BlockID {
	return blk.blockID
}

func (blk *Block) Data() []byte {
	return blk.data
}

func (blk *Block) Block() *iotago.Block {
	blk.blockOnce.Do(func() {
		iotaBlock := new(iotago.Block)
		// No need to verify the block again here
		if _, err := blk.api.Decode(blk.data, iotaBlock); err != nil {
			panic(fmt.Sprintf("failed to deserialize block: %v, error: %s", blk.blockID.ToHex(), err))
		}

		blk.block = iotaBlock
	})

	return blk.block
}

// TODO: maybe move to iota.go and introduce parent type
func (blk *Block) Parents() (parents []iotago.BlockID) {
	blk.ForEachParent(func(parent Parent) {
		parents = append(parents, parent.ID)
	})

	return parents
}

// ForEachParent executes a consumer func for each parent.
func (blk *Block) ForEachParent(consumer func(parent Parent)) {
	seenBlockIDs := make(map[iotago.BlockID]types.Empty)
	block := blk.Block()

	for _, parentBlockID := range block.StrongParents {
		if _, exists := seenBlockIDs[parentBlockID]; !exists {
			seenBlockIDs[parentBlockID] = types.Void
			consumer(Parent{parentBlockID, StrongParentType})
		}
	}

	for _, parentBlockID := range block.WeakParents {
		if _, exists := seenBlockIDs[parentBlockID]; !exists {
			seenBlockIDs[parentBlockID] = types.Void
			consumer(Parent{parentBlockID, WeakParentType})
		}
	}

	for _, parentBlockID := range block.ShallowLikeParents {
		if _, exists := seenBlockIDs[parentBlockID]; !exists {
			seenBlockIDs[parentBlockID] = types.Void
			consumer(Parent{parentBlockID, ShallowLikeParentType})
		}
	}
}
