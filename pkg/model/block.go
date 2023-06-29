package model

import (
	"bytes"
	"encoding/json"

	"github.com/iotaledger/hive.go/serializer/v2/serix"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Block struct {
	api iotago.API

	blockID iotago.BlockID

	data          []byte
	protocolBlock *iotago.ProtocolBlock
}

func newBlock(blockID iotago.BlockID, iotaBlock *iotago.ProtocolBlock, data []byte, api iotago.API) (*Block, error) {
	block := &Block{
		api:           api,
		blockID:       blockID,
		data:          data,
		protocolBlock: iotaBlock,
	}

	return block, nil
}

func BlockFromBlock(protocolBlock *iotago.ProtocolBlock, api iotago.API, opts ...serix.Option) (*Block, error) {
	data, err := api.Encode(protocolBlock, opts...)
	if err != nil {
		return nil, err
	}

	blockID, err := protocolBlock.ID(api)
	if err != nil {
		return nil, err
	}

	return newBlock(blockID, protocolBlock, data, api)
}

func BlockFromIDAndBytes(blockID iotago.BlockID, data []byte, api iotago.API, opts ...serix.Option) (*Block, error) {
	protocolBlock := new(iotago.ProtocolBlock)
	if _, err := api.Decode(data, protocolBlock, opts...); err != nil {
		return nil, err
	}

	return newBlock(blockID, protocolBlock, data, api)
}

func BlockFromBytes(data []byte, api iotago.API, opts ...serix.Option) (*Block, error) {
	iotaBlock := new(iotago.ProtocolBlock)
	if _, err := api.Decode(data, iotaBlock, opts...); err != nil {
		return nil, err
	}

	blockID, err := iotaBlock.ID(api)
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

func (blk *Block) ProtocolBlock() *iotago.ProtocolBlock {
	return blk.protocolBlock
}

func (blk *Block) String() string {
	encode, err := blk.api.JSONEncode(blk.ProtocolBlock())
	if err != nil {
		panic(err)
	}
	var out bytes.Buffer
	if json.Indent(&out, encode, "", "  ") != nil {
		panic(err)
	}

	return out.String()
}
