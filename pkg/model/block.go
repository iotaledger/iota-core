package model

import (
	"bytes"
	"encoding/json"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Block struct {
	api iotago.API

	blockID iotago.BlockID

	data       []byte
	block      *iotago.Block
	commitment *Commitment
}

func newBlock(blockID iotago.BlockID, iotaBlock *iotago.Block, data []byte, api iotago.API) (*Block, error) {
	commitment, err := CommitmentFromCommitment(iotaBlock.SlotCommitment, api)
	if err != nil {
		return nil, err
	}

	block := &Block{
		api:        api,
		blockID:    blockID,
		data:       data,
		block:      iotaBlock,
		commitment: commitment,
	}

	return block, nil
}

func BlockFromBlock(iotaBlock *iotago.Block, api iotago.API, opts ...serix.Option) (*Block, error) {
	data, err := api.Encode(iotaBlock, opts...)
	if err != nil {
		return nil, err
	}

	blockID, err := iotaBlock.ID(api.SlotTimeProvider())
	if err != nil {
		return nil, err
	}

	return newBlock(blockID, iotaBlock, data, api)
}

func BlockFromIDAndBytes(blockID iotago.BlockID, data []byte, api iotago.API, opts ...serix.Option) (*Block, error) {
	iotaBlock := new(iotago.Block)
	if _, err := api.Decode(data, iotaBlock, opts...); err != nil {
		return nil, err
	}

	return newBlock(blockID, iotaBlock, data, api)
}

func BlockFromBytes(data []byte, api iotago.API, opts ...serix.Option) (*Block, error) {
	iotaBlock := new(iotago.Block)
	if _, err := api.Decode(data, iotaBlock, opts...); err != nil {
		return nil, err
	}

	blockID, err := iotaBlock.ID(api.SlotTimeProvider())
	if err != nil {
		return nil, err
	}

	return newBlock(blockID, iotaBlock, data, api)
}

func (blk *Block) ID() iotago.BlockID {
	return blk.blockID
}

func (blk *Block) Data() []byte {
	return blk.data
}

func (blk *Block) Block() *iotago.Block {
	return blk.block
}

func (blk *Block) SlotCommitment() *Commitment {
	return blk.commitment
}

// TODO: maybe move to iota.go and introduce parent type.
func (blk *Block) Parents() (parents []iotago.BlockID) {
	parents = make([]iotago.BlockID, 0)
	blk.ForEachParent(func(parent Parent) {
		parents = append(parents, parent.ID)
	})

	return parents
}

func (blk *Block) ParentsWithType() (parents []Parent) {
	parents = make([]Parent, 0)
	block := blk.Block()

	for _, parentBlockID := range block.StrongParents {
		parents = append(parents, Parent{parentBlockID, StrongParentType})
	}

	for _, parentBlockID := range block.WeakParents {
		parents = append(parents, Parent{parentBlockID, WeakParentType})
	}

	for _, parentBlockID := range block.ShallowLikeParents {
		parents = append(parents, Parent{parentBlockID, ShallowLikeParentType})
	}

	return parents
}

// ForEachParent executes a consumer func for each parent.
func (blk *Block) ForEachParent(consumer func(parent Parent)) {
	// TODO: is this even correct to ignore parents?
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

func (blk *Block) String() string {
	encode, err := blk.api.JSONEncode(blk.Block())
	if err != nil {
		panic(err)
	}
	var out bytes.Buffer
	if json.Indent(&out, encode, "", "  ") != nil {
		panic(err)
	}

	return out.String()
}
