package tipmanagerv1

import (
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/tipmanager"
)

// TipMetadata represents the metadata for a block in the TipManager.
type TipMetadata struct {
	// Block holds the actual block.
	block *blocks.Block

	// tipPool holds the TipPool the block is currently in.
	tipPool *promise.Value[tipmanager.TipPool]

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

	// strongTip is a derived property that is true if the block is part of the strong tip set.
	strongTip *promise.Value[bool]

	// weakTip is a derived property that is true if the block is part of the weak tip set.
	weakTip *promise.Value[bool]

	// evicted is triggered when the block is removed from the TipManager.
	evicted *promise.Event
}

// NewBlockMetadata creates a new TipMetadata instance.
func NewBlockMetadata(block *blocks.Block) *TipMetadata {
	return (&TipMetadata{
		block:                     block,
		tipPool:                   promise.NewValue[tipmanager.TipPool](),
		stronglyConnectedChildren: promise.NewValue[int](),
		weaklyConnectedChildren:   promise.NewValue[int](),
		stronglyReferencedByTips:  promise.NewValue[bool]().WithTriggerWithInitialZeroValue(true),
		referencedByTips:          promise.NewValue[bool]().WithTriggerWithInitialZeroValue(true),
		stronglyConnectedToTips:   promise.NewValue[bool](),
		weaklyConnectedToTips:     promise.NewValue[bool](),
		strongTip:                 promise.NewValue[bool](),
		weakTip:                   promise.NewValue[bool](),
		evicted:                   promise.NewEvent(),
	}).setup()
}

// Block returns the Block the TipMetadata belongs to.
func (t *TipMetadata) Block() *blocks.Block {
	return t.block
}

// TipPool returns the TipPool the Block is currently in.
func (t *TipMetadata) TipPool() tipmanager.TipPool {
	return t.tipPool.Get()
}

// OnTipPoolUpdated registers a callback that is triggered when the TipPool the Block is currently in is updated.
func (t *TipMetadata) OnTipPoolUpdated(handler func(tipPool tipmanager.TipPool)) (unsubscribe func()) {
	return t.tipPool.OnUpdate(func(_, tipPool tipmanager.TipPool) { handler(tipPool) })
}

// IsStrongTip returns true if the Block is part of the strong tip set.
func (t *TipMetadata) IsStrongTip() bool {
	return t.strongTip.Get()
}

// OnIsStrongTipUpdated registers a callback that is triggered when the IsStrongTip property of the Block is updated.
func (t *TipMetadata) OnIsStrongTipUpdated(handler func(isStrongTip bool)) (unsubscribe func()) {
	return t.strongTip.OnUpdate(func(_, isStrongTip bool) { handler(isStrongTip) })
}

// IsWeakTip returns true if the Block is part of the weak tip set.
func (t *TipMetadata) IsWeakTip() bool {
	return t.weakTip.Get()
}

// OnIsWeakTipUpdated registers a callback that is triggered when the IsWeakTip property of the Block is updated.
func (t *TipMetadata) OnIsWeakTipUpdated(handler func(isWeakTip bool)) (unsubscribe func()) {
	return t.weakTip.OnUpdate(func(_, isWeakTip bool) { handler(isWeakTip) })
}

// IsEvicted returns true if the Block was removed from the TipManager.
func (t *TipMetadata) IsEvicted() bool {
	return t.evicted.WasTriggered()
}

// OnEvicted registers a callback that is triggered when the Block is removed from the TipManager.
func (t *TipMetadata) OnEvicted(handler func()) {
	t.evicted.OnTrigger(handler)
}

// setup sets up the behavior of the derived properties of the Block.
func (t *TipMetadata) setup() (self *TipMetadata) {
	var leaveTipPool func()

	t.OnTipPoolUpdated(func(tipPool tipmanager.TipPool) {
		if leaveTipPool != nil {
			leaveTipPool()
		}

		if tipPool == tipmanager.StrongTipPool {
			leaveTipPool = t.joinTipPool(t.stronglyReferencedByTips, t.strongTip)
		} else if tipPool == tipmanager.WeakTipPool {
			leaveTipPool = t.joinTipPool(t.referencedByTips, t.weakTip)
		} else {
			leaveTipPool = nil
		}

		t.stronglyConnectedToTips.Set(tipPool == tipmanager.StrongTipPool || t.stronglyConnectedChildren.Get() > 0)
		t.weaklyConnectedToTips.Set(tipPool == tipmanager.WeakTipPool || t.weaklyConnectedChildren.Get() > 0)
	})

	t.stronglyConnectedChildren.OnUpdate(func(_, strongChildren int) {
		t.stronglyConnectedToTips.Set(strongChildren > 0 || t.tipPool.Get() == tipmanager.StrongTipPool)
		t.referencedByTips.Set(strongChildren > 0 || t.weaklyConnectedChildren.Get() > 0)
		t.stronglyReferencedByTips.Set(strongChildren > 0)
	})

	t.weaklyConnectedChildren.OnUpdate(func(_, newCount int) {
		t.weaklyConnectedToTips.Set(newCount > 0 || t.tipPool.Get() == tipmanager.WeakTipPool)
		t.referencedByTips.Set(newCount > 0 || t.stronglyConnectedChildren.Get() > 0)
	})

	return t
}

// joinTipPool joins the tip pool by setting up the tip pool derived properties.
func (t *TipMetadata) joinTipPool(isReferencedByTips *promise.Value[bool], isTip *promise.Value[bool]) (leave func()) {
	unsubscribe := isReferencedByTips.OnUpdate(func(_, isReferenced bool) {
		isTip.Set(!isReferenced)
	})

	return func() {
		unsubscribe()

		isTip.Set(false)
	}
}

// setTipPool sets the TipPool of the Block.
func (t *TipMetadata) setTipPool(newType tipmanager.TipPool) {
	t.tipPool.Compute(func(prevType tipmanager.TipPool) tipmanager.TipPool {
		return lo.Cond(newType > prevType, newType, prevType)
	})
}

// propagateConnectedChildren returns the rules for the propagation of the internal connected children counters.
func propagateConnectedChildren(isConnected bool, stronglyConnected bool) (propagationRules map[model.ParentsType]func(*TipMetadata)) {
	diffToApply := lo.Cond(isConnected, +1, -1)

	updatedValue := func(value int) int {
		return value + diffToApply
	}

	propagationRules = map[model.ParentsType]func(*TipMetadata){
		model.WeakParentType: func(parent *TipMetadata) {
			parent.weaklyConnectedChildren.Compute(updatedValue)
		},
	}

	if stronglyConnected {
		propagationRules[model.StrongParentType] = func(parent *TipMetadata) {
			parent.stronglyConnectedChildren.Compute(updatedValue)
		}
	}

	return propagationRules
}

// code contract (make sure the type implements all required methods).
var _ tipmanager.TipMetadata = new(TipMetadata)
