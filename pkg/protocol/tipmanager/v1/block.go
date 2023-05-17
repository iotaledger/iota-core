package v1

import (
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

type Block struct {
	*blocks.Block

	blockEvicted *promise.Event

	ApprovalCount int
}

func NewBlock(block *blocks.Block) *Block {
	return &Block{
		Block:        block,
		blockEvicted: promise.NewEvent(),
	}
}

func (b *Block) IncreaseApprovalCount() *Block {
	b.ApprovalCount++

	return b
}
