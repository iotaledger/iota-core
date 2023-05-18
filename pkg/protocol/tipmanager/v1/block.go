package v1

import (
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

type Block struct {
	*blocks.Block

	blockEvicted *promise.Event

	tipPool             *promise.Value[TipPoolType]
	strongApprovalCount *promise.Value[int]

	ApprovalCount int
}

func NewBlock(block *blocks.Block) *Block {
	return &Block{
		Block:               block,
		blockEvicted:        promise.NewEvent(),
		tipPool:             promise.NewValue[TipPoolType](),
		strongApprovalCount: promise.NewValue[int](),
	}
}

func (b *Block) OnStrongApprovalCountUpdated(handler func(prevValue, newValue int)) (unsubscribe func()) {
	return b.strongApprovalCount.OnUpdate(handler)
}

func (b *Block) OnTipPoolChanged(handler func(prevType, newType TipPoolType)) (unsubscribe func()) {
	return b.tipPool.OnUpdate(handler)
}

func (b *Block) setTipPool(newType TipPoolType) bool {
	var updated bool
	b.tipPool.Compute(func(prevType TipPoolType) TipPoolType {
		if updated = newType > prevType; !updated {
			return prevType
		}

		return newType
	})

	return updated
}

func (b *Block) IncreaseApprovalCount() *Block {
	b.ApprovalCount++

	return b
}
