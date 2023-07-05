package model

import (
	"bytes"
	"encoding/json"

	"github.com/iotaledger/hive.go/serializer/v2/serix"
	"github.com/iotaledger/iota-core/pkg/core/api"
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

func BlockFromBytes(data []byte, apiProvider api.Provider, opts ...serix.Option) (*Block, error) {
	api := apiProvider.APIForVersion(data[0])

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

func (blk *Block) Transaction() (tx *iotago.Transaction, isTransaction bool) {
	basicBlock, isBasicBlock := blk.BasicBlock()
	if !isBasicBlock {
		return nil, false
	}

	tx, isTransaction = basicBlock.Payload.(*iotago.Transaction)
	return tx, isTransaction
}

func (blk *Block) BasicBlock() (basicBlock *iotago.BasicBlock, isBasicBlock bool) {
	basicBlock, isBasicBlock = blk.ProtocolBlock().Block.(*iotago.BasicBlock)
	return basicBlock, isBasicBlock
}

func (blk *Block) ValidatorBlock() (validatorBlock *iotago.ValidatorBlock, isValidatorBlock bool) {
	validatorBlock, isValidatorBlock = blk.ProtocolBlock().Block.(*iotago.ValidatorBlock)
	return validatorBlock, isValidatorBlock
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
