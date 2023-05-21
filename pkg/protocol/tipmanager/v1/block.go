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
	stronglyReachableFromTips *promise.Value[bool]
	weaklyReachableFromTips   *promise.Value[bool]

	blockEvicted *promise.Event
}

func NewBlock(block *blocks.Block) *Block {
	b := &Block{
		Block:                     block,
		blockEvicted:              promise.NewEvent(),
		tipPool:                   promise.NewValue[TipPoolType](),
		stronglyConnectedChildren: promise.NewInt(),
		weaklyConnectedChildren:   promise.NewInt(),
		stronglyReachableFromTips: promise.NewValue[bool](),
		weaklyReachableFromTips:   promise.NewValue[bool](),
	}

	b.tipPool.OnUpdate(func(_, tipPool TipPoolType) {
		b.stronglyReachableFromTips.Set(tipPool == StrongTipPool || b.stronglyConnectedChildren.Get() > 0)
		b.weaklyReachableFromTips.Set(tipPool == WeakTipPool || b.weaklyConnectedChildren.Get() > 0)
	})
	b.OnStronglyConnectedChildrenUpdated(func(_, newCount int) {
		b.stronglyReachableFromTips.Set(newCount > 0 || b.tipPool.Get() == StrongTipPool)
	})
	b.OnWeaklyConnectedChildrenUpdated(func(_, newCount int) {
		b.weaklyReachableFromTips.Set(newCount > 0 || b.tipPool.Get() == WeakTipPool)
	})

	return b
}

func (b *Block) OnWeaklyConnectedChildrenUpdated(handler func(prevValue, newValue int)) (unsubscribe func()) {
	return b.weaklyConnectedChildren.OnUpdate(handler)
}

func (b *Block) OnConnectionToStrongTips(callback func()) (unsubscribe func()) {
	return b.stronglyReachableFromTips.OnUpdate(func(_, newValue bool) {
		if newValue {
			callback()
		}
	})
}

func (b *Block) OnConnectionToStrongTipsLost(callback func()) (unsubscribe func()) {
	return b.stronglyReachableFromTips.OnUpdate(func(_, newValue bool) {
		if !newValue {
			callback()
		}
	})
}

func (b *Block) OnStronglyConnectedChildrenUpdated(handler func(prevValue, newValue int)) (unsubscribe func()) {
	return b.stronglyConnectedChildren.OnUpdate(handler)
}

func (b *Block) setTipPool(newType TipPoolType) (updated bool) {
	b.tipPool.Compute(func(prevType TipPoolType) TipPoolType {
		updated = newType > prevType

		return lo.Cond(updated, newType, prevType)
	})

	return updated
}
