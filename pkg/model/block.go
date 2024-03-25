package model

import (
	"bytes"
	"encoding/json"

	"github.com/iotaledger/hive.go/serializer/v2/serix"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Block struct {
	blockID iotago.BlockID

	data  []byte
	block *iotago.Block
}

func newBlock(blockID iotago.BlockID, iotaBlock *iotago.Block, data []byte) (*Block, error) {
	block := &Block{
		blockID: blockID,
		data:    data,
		block:   iotaBlock,
	}

	return block, nil
}

func BlockFromBlock(block *iotago.Block, opts ...serix.Option) (*Block, error) {
	data, err := block.API.Encode(block, opts...)
	if err != nil {
		return nil, err
	}

	blockIdentifier, err := iotago.BlockIdentifierFromBlockBytes(data)
	if err != nil {
		return nil, err
	}

	return newBlock(block.IDWithBlockIdentifier(blockIdentifier), block, data)
}

// BlockFromIDAndBytes creates a new Block from the given blockID and the serialized block data.
// This is used when loading a block back from storage where we have both the blockID and the bytes available.
func BlockFromIDAndBytes(blockID iotago.BlockID, data []byte, api iotago.API) (*Block, error) {
	block, _, err := iotago.BlockFromBytes(iotago.SingleVersionProvider(api))(data)
	if err != nil {
		return nil, err
	}

	return newBlock(blockID, block, data)
}

// BlockFromBlockIdentifierAndBytes creates a new Block from the given blockIdentifier and the serialized block data.
// This is used when receiving blocks from the network where we pre-compute the blockIdentifier for filtering duplicates.
func BlockFromBlockIdentifierAndBytes(blockIdentifier iotago.Identifier, data []byte, apiProvider iotago.APIProvider) (*Block, error) {
	block, _, err := iotago.BlockFromBytes(apiProvider)(data)
	if err != nil {
		return nil, err
	}

	return newBlock(block.IDWithBlockIdentifier(blockIdentifier), block, data)
}

// BlockFromBytes creates a new Block from the serialized block data.
func BlockFromBytes(data []byte, apiProvider iotago.APIProvider) (*Block, error) {
	blockIdentifier, err := iotago.BlockIdentifierFromBlockBytes(data)
	if err != nil {
		return nil, err
	}

	return BlockFromBlockIdentifierAndBytes(blockIdentifier, data, apiProvider)
}

func BlockFromBytesFunc(apiProvider iotago.APIProvider) func(data []byte) (*Block, int, error) {
	return func(data []byte) (*Block, int, error) {
		block, err := BlockFromBytes(data, apiProvider)
		if err != nil {
			return nil, 0, err
		}

		return block, len(data), nil
	}
}

func (blk *Block) ID() iotago.BlockID {
	return blk.blockID
}

func (blk *Block) SlotCommitmentID() iotago.CommitmentID {
	return blk.block.Header.SlotCommitmentID
}

func (blk *Block) Data() []byte {
	return blk.data
}

func (blk *Block) Bytes() ([]byte, error) {
	return blk.data, nil
}

func (blk *Block) ProtocolBlock() *iotago.Block {
	return blk.block
}

func (blk *Block) Payload() iotago.Payload {
	basicBlock, isBasicBlock := blk.BasicBlock()
	if !isBasicBlock {
		return nil
	}

	return basicBlock.Payload
}

func (blk *Block) SignedTransaction() (tx *iotago.SignedTransaction, isTransaction bool) {
	payload := blk.Payload()
	if payload == nil {
		return nil, false
	}

	tx, isTransaction = payload.(*iotago.SignedTransaction)

	return tx, isTransaction
}

func (blk *Block) BasicBlock() (basicBlock *iotago.BasicBlockBody, isBasicBlock bool) {
	basicBlock, isBasicBlock = blk.ProtocolBlock().Body.(*iotago.BasicBlockBody)
	return basicBlock, isBasicBlock
}

func (blk *Block) ValidationBlock() (validationBlock *iotago.ValidationBlockBody, isValidationBlock bool) {
	validationBlock, isValidationBlock = blk.ProtocolBlock().Body.(*iotago.ValidationBlockBody)
	return validationBlock, isValidationBlock
}

func (blk *Block) String() string {
	encode, err := blk.block.API.JSONEncode(blk.ProtocolBlock())
	if err != nil {
		panic(err)
	}
	var out bytes.Buffer
	if err = json.Indent(&out, encode, "", "  "); err != nil {
		panic(err)
	}

	return out.String()
}

func (blk *Block) WorkScore() iotago.WorkScore {
	workScore, err := blk.ProtocolBlock().WorkScore()
	if err != nil {
		panic(err)
	}

	return workScore
}
