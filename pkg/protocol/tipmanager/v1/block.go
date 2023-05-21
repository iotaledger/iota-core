package v1

import (
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

type Block struct {
	*blocks.Block

	tipPool                   *promise.Value[TipPoolType]
	stronglyConnectedChildren *promise.Int
	weaklyConnectedChildren   *promise.Int
	stronglyConnectedToTips   *promise.Value[bool]
	weaklyConnectedToTips     *promise.Value[bool]
	stronglyReferencedByTips  *promise.Value[bool]
	referencedByTips          *promise.Value[bool]

	blockEvicted *promise.Event
}

func NewBlock(block *blocks.Block) *Block {
	b := &Block{
		Block:                     block,
		blockEvicted:              promise.NewEvent(),
		tipPool:                   promise.NewValue[TipPoolType](),
		stronglyConnectedToTips:   promise.NewValue[bool](),
		weaklyConnectedToTips:     promise.NewValue[bool](),
		stronglyConnectedChildren: promise.NewInt(),
		weaklyConnectedChildren:   promise.NewInt(),
		stronglyReferencedByTips:  promise.NewValue[bool](),
		referencedByTips:          promise.NewValue[bool](),
	}

	b.tipPool.OnUpdate(func(_, tipPool TipPoolType) {
		b.stronglyConnectedToTips.Set(tipPool == StrongTipPool || b.stronglyConnectedChildren.Get() > 0)
		b.weaklyConnectedToTips.Set(tipPool == WeakTipPool || b.weaklyConnectedChildren.Get() > 0)
	})
	b.stronglyConnectedChildren.OnUpdate(func(_, strongChildren int) {
		b.stronglyConnectedToTips.Set(strongChildren > 0 || b.tipPool.Get() == StrongTipPool)
		b.referencedByTips.Set(strongChildren > 0 || b.weaklyConnectedChildren.Get() > 0)
		b.stronglyReferencedByTips.Set(strongChildren > 0)
	})
	b.weaklyConnectedChildren.OnUpdate(func(_, newCount int) {
		b.weaklyConnectedToTips.Set(newCount > 0 || b.tipPool.Get() == WeakTipPool)
		b.referencedByTips.Set(newCount > 0 || b.stronglyConnectedChildren.Get() > 0)
	})

	return b
}

func (b *Block) increaseStronglyConnectedChildren() {
	b.stronglyConnectedChildren.Increase()
}

func (b *Block) decreaseStronglyConnectedChildren() {
	b.stronglyConnectedChildren.Decrease()
}

func (b *Block) increaseWeaklyConnectedChildren() {
	b.weaklyConnectedChildren.Increase()
}

func (b *Block) decreaseWeaklyConnectedChildren() {
	b.weaklyConnectedChildren.Decrease()
}

func (b *Block) setTipPool(newType TipPoolType) (updated bool) {
	b.tipPool.Compute(func(prevType TipPoolType) TipPoolType {
		updated = newType > prevType

		return lo.Cond(updated, newType, prevType)
	})

	return updated
}
