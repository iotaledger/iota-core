package v1

import (
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

// Block represents a block in the TipManager.
type Block struct {
	// Block holds the actual block.
	*blocks.Block

	// tipPool holds the TipPool the block is currently in.
	tipPool *promise.Value[TipPool]

	// stronglyConnectedChildren holds the number of strong children that can be reached from the tips using only strong
	// references.
	stronglyConnectedChildren *promise.Int

	// weaklyConnectedChildren holds the number of weak children that can be reached from the tips.
	weaklyConnectedChildren *promise.Int

	// stronglyReferencedByTips is a derived property that is true if the block has at least one strongly connected
	// child.
	stronglyReferencedByTips *promise.Value[bool]

	// referencedByTips is a derived property that is true if the block has at least one strongly or weakly connected
	// child.
	referencedByTips *promise.Value[bool]

	// stronglyConnectedToTips is a derived property that is true if the block is either strongly referenced by tips or
	// part of the strong TipPool.
	stronglyConnectedToTips *promise.Value[bool]

	// weaklyConnectedToTips is a derived property that is true if the block is either part of the weak TipPool or has
	// at least one weakly connected child.
	weaklyConnectedToTips *promise.Value[bool]

	// blockEvicted is triggered when the block is removed from the TipManager.
	blockEvicted *promise.Event
}

// NewBlock creates a new Block.
func NewBlock(block *blocks.Block) *Block {
	return (&Block{
		Block:                     block,
		tipPool:                   promise.NewValue[TipPool](),
		stronglyConnectedChildren: promise.NewInt(),
		weaklyConnectedChildren:   promise.NewInt(),
		stronglyReferencedByTips:  promise.NewValue[bool](),
		referencedByTips:          promise.NewValue[bool](),
		stronglyConnectedToTips:   promise.NewValue[bool](),
		weaklyConnectedToTips:     promise.NewValue[bool](),
		blockEvicted:              promise.NewEvent(),
	}).setup()
}

// setup sets up the behavior of the derived properties of the Block.
func (b *Block) setup() (self *Block) {
	b.tipPool.OnUpdate(func(_, tipPool TipPool) {
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

// setTipPool sets the TipPool of the Block.
func (b *Block) setTipPool(newType TipPool) (updated bool) {
	b.tipPool.Compute(func(prevType TipPool) TipPool {
		updated = newType > prevType

		return lo.Cond(updated, newType, prevType)
	})

	return updated
}

// increaseStronglyConnectedChildren increases the number of strongly connected children.
func (b *Block) increaseStronglyConnectedChildren() {
	b.stronglyConnectedChildren.Increase()
}

// decreaseStronglyConnectedChildren decreases the number of strongly connected children.
func (b *Block) decreaseStronglyConnectedChildren() {
	b.stronglyConnectedChildren.Decrease()
}

// increaseWeaklyConnectedChildren increases the number of weakly connected children.
func (b *Block) increaseWeaklyConnectedChildren() {
	b.weaklyConnectedChildren.Increase()
}

// decreaseWeaklyConnectedChildren decreases the number of weakly connected children.
func (b *Block) decreaseWeaklyConnectedChildren() {
	b.weaklyConnectedChildren.Decrease()
}

// propagateConnectedChildren returns the rules for the propagation of the internal connected children counters.
func propagateConnectedChildren(isConnected bool, stronglyConnected bool) (propagationRules map[model.ParentsType]func(*Block)) {
	if isConnected {
		if propagationRules = map[model.ParentsType]func(*Block){
			model.WeakParentType: (*Block).increaseWeaklyConnectedChildren,
		}; stronglyConnected {
			propagationRules[model.StrongParentType] = (*Block).increaseStronglyConnectedChildren
		}
	} else {
		if propagationRules = map[model.ParentsType]func(*Block){
			model.WeakParentType: (*Block).decreaseWeaklyConnectedChildren,
		}; stronglyConnected {
			propagationRules[model.StrongParentType] = (*Block).decreaseStronglyConnectedChildren
		}
	}

	return propagationRules
}
