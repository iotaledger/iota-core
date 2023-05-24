package tipmanagerv1

import (
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

// BlockMetadata represents the metadata for a block in the TipManager.
type BlockMetadata struct {
	// Block holds the actual block.
	*blocks.Block

	// tipPool holds the TipPool the block is currently in.
	tipPool *promise.Value[TipPool]

	// stronglyConnectedChildren holds the number of strong children that can be reached from the tips using only strong
	// references.
	stronglyConnectedChildren *promise.Value[int]

	// weaklyConnectedChildren holds the number of weak children that can be reached from the tips.
	weaklyConnectedChildren *promise.Value[int]

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

	// isStrongTip is a derived property that is true if the block is part of the strong tip set.
	isStrongTip *promise.Value[bool]

	// isWeakTip is a derived property that is true if the block is part of the weak tip set.
	isWeakTip *promise.Value[bool]

	// evicted is triggered when the block is removed from the TipManager.
	evicted *promise.Event
}

// NewBlockMetadata creates a new BlockMetadata instance.
func NewBlockMetadata(block *blocks.Block) *BlockMetadata {
	return (&BlockMetadata{
		Block:                     block,
		tipPool:                   promise.NewValue[TipPool](),
		stronglyConnectedChildren: promise.NewValue[int](),
		weaklyConnectedChildren:   promise.NewValue[int](),
		stronglyReferencedByTips:  promise.NewValue[bool]().WithTriggerWithInitialEmptyValue(true),
		referencedByTips:          promise.NewValue[bool]().WithTriggerWithInitialEmptyValue(true),
		stronglyConnectedToTips:   promise.NewValue[bool](),
		weaklyConnectedToTips:     promise.NewValue[bool](),
		isStrongTip:               promise.NewValue[bool](),
		isWeakTip:                 promise.NewValue[bool](),
		evicted:                   promise.NewEvent(),
	}).setup()
}

// TipPool returns the TipPool the Block is currently in.
func (b *BlockMetadata) TipPool() TipPool {
	return b.tipPool.Get()
}

// IsEvicted returns true if the Block was removed from the TipManager.
func (b *BlockMetadata) IsEvicted() bool {
	return b.evicted.WasTriggered()
}

// OnEvicted registers a callback that is triggered when the Block is removed from the TipManager.
func (b *BlockMetadata) OnEvicted(handler func()) {
	b.evicted.OnTrigger(handler)
}

// setup sets up the behavior of the derived properties of the Block.
func (b *BlockMetadata) setup() (self *BlockMetadata) {
	var leaveTipPool func()

	b.tipPool.OnUpdate(func(_, tipPool TipPool) {
		b.stronglyConnectedToTips.Set(tipPool == StrongTipPool || b.stronglyConnectedChildren.Get() > 0)
		b.weaklyConnectedToTips.Set(tipPool == WeakTipPool || b.weaklyConnectedChildren.Get() > 0)

		if leaveTipPool != nil {
			leaveTipPool()
		}

		if tipPool == StrongTipPool {
			leaveTipPool = b.joinTipPool(b.stronglyReferencedByTips, b.isStrongTip)
		} else if tipPool == WeakTipPool {
			leaveTipPool = b.joinTipPool(b.referencedByTips, b.isWeakTip)
		} else {
			leaveTipPool = nil
		}
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

// joinTipPool joins the tip pool by setting up the tip pool derived properties.
func (b *BlockMetadata) joinTipPool(isReferencedByTips *promise.Value[bool], isTip *promise.Value[bool]) (leave func()) {
	unsubscribe := isReferencedByTips.OnUpdate(func(_, isReferenced bool) {
		isTip.Set(!isReferenced)
	})

	return func() {
		unsubscribe()

		isTip.Set(false)
	}
}

// setTipPool sets the TipPool of the Block.
func (b *BlockMetadata) setTipPool(newType TipPool) (updated bool) {
	b.tipPool.Compute(func(prevType TipPool) TipPool {
		updated = newType > prevType

		return lo.Cond(updated, newType, prevType)
	})

	return updated
}

// propagateConnectedChildren returns the rules for the propagation of the internal connected children counters.
func propagateConnectedChildren(isConnected bool, stronglyConnected bool) (propagationRules map[model.ParentsType]func(*BlockMetadata)) {
	diffToApply := lo.Cond(isConnected, +1, -1)

	updatedValue := func(value int) int {
		return value + diffToApply
	}

	propagationRules = map[model.ParentsType]func(*BlockMetadata){
		model.WeakParentType: func(parent *BlockMetadata) {
			parent.weaklyConnectedChildren.Compute(updatedValue)
		},
	}

	if stronglyConnected {
		propagationRules[model.StrongParentType] = func(parent *BlockMetadata) {
			parent.stronglyConnectedChildren.Compute(updatedValue)
		}
	}

	return propagationRules
}
