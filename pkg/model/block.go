package model

import (
	"bytes"
	"encoding/json"

	"github.com/iotaledger/hive.go/ierrors"
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

func BlockFromBytes(data []byte, apiProvider iotago.APIProvider, opts ...serix.Option) (*Block, error) {
	version, _, err := iotago.VersionFromBytes(data)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to determine version")
	}

	apiForVersion, err := apiProvider.APIForVersion(version)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get API for version %d", version)
	}

	iotaBlock := new(iotago.ProtocolBlock)
	if _, err := apiForVersion.Decode(data, iotaBlock, opts...); err != nil {
		return nil, err
	}

	blockID, err := iotaBlock.ID(apiForVersion)
	if err != nil {
		return nil, err
	}

	return newBlock(blockID, iotaBlock, data, apiForVersion)
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

func (blk *Block) Payload() iotago.Payload {
	basicBlock, isBasicBlock := blk.BasicBlock()
	if !isBasicBlock {
		return nil
	}

	return basicBlock.Payload
}

func (blk *Block) Transaction() (tx *iotago.Transaction, isTransaction bool) {
	payload := blk.Payload()
	if payload == nil {
		return nil, false
	}

	tx, isTransaction = payload.(*iotago.Transaction)

	return tx, isTransaction
}

func (blk *Block) BasicBlock() (basicBlock *iotago.BasicBlock, isBasicBlock bool) {
	basicBlock, isBasicBlock = blk.ProtocolBlock().Block.(*iotago.BasicBlock)
	return basicBlock, isBasicBlock
}

func (blk *Block) ValidationBlock() (validationBlock *iotago.ValidationBlock, isValidationBlock bool) {
	validationBlock, isValidationBlock = blk.ProtocolBlock().Block.(*iotago.ValidationBlock)
	return validationBlock, isValidationBlock
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

func (blk *Block) WorkScore() iotago.WorkScore {
	if _, isBasic := blk.BasicBlock(); isBasic {
		workScore, err := blk.ProtocolBlock().WorkScore(blk.api.ProtocolParameters().WorkScoreStructure())
		if err != nil {
			panic(err)
		}

		return workScore
	}

	// else this is a validator block and should have workScore Zero
	// TODO: deal with validator blocks with issue #236
	return iotago.WorkScore(0)

}
