package tipmanagerv1

import (
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/promise"
	lpromise "github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TipMetadata represents the metadata for a block in the TipManager.
type TipMetadata struct {
	// Block holds the actual block.
	block *blocks.Block

	// tipPool holds the TipPool the block is currently in.
	tipPool *lpromise.Value[tipmanager.TipPool]

	// stronglyConnectedChildren holds the number of strong children that can be reached from the tips using only strong
	// references.
	stronglyConnectedChildren *lpromise.Value[int]

	// weaklyConnectedChildren holds the number of weak children that can be reached from the tips.
	weaklyConnectedChildren *lpromise.Value[int]

	// stronglyReferencedByTips is a derived property that is true if the block has at least one strongly connected
	// child.
	stronglyReferencedByTips *lpromise.Value[bool]

	// referencedByTips is a derived property that is true if the block has at least one strongly or weakly connected
	// child.
	referencedByTips *lpromise.Value[bool]

	// stronglyConnectedToTips is a derived property that is true if the block is either strongly referenced by tips or
	// part of the strong TipPool.
	stronglyConnectedToTips *lpromise.Value[bool]

	// weaklyConnectedToTips is a derived property that is true if the block is either part of the weak TipPool or has
	// at least one weakly connected child.
	weaklyConnectedToTips *lpromise.Value[bool]

	// strongTip is a derived property that is true if the block is part of the strong TipPool, and is not
	// stronglyReferencedByTips.
	strongTip *lpromise.Value[bool]

	// weakTip is a derived property that is true if the block is part of the weak TipPool and is not referencedByTips.
	weakTip *lpromise.Value[bool]

	// orphanedStrongParents holds the number of parents that are orphaned.
	orphanedStrongParents *lpromise.Value[int]

	// markedOrphaned is a property that is true if the block was marked as orphaned.
	markedOrphaned *lpromise.Value[bool]

	// orphaned is a derived property that is true if the block is either marked as orphaned or has at least one
	// orphaned strong parent.
	orphaned *lpromise.Value[bool]

	// evicted is triggered when the block is removed from the TipManager.
	evicted *promise.Event
}

// NewBlockMetadata creates a new TipMetadata instance.
func NewBlockMetadata(block *blocks.Block) *TipMetadata {
	return (&TipMetadata{
		block:                     block,
		tipPool:                   lpromise.NewValue[tipmanager.TipPool](),
		stronglyConnectedChildren: lpromise.NewValue[int](),
		weaklyConnectedChildren:   lpromise.NewValue[int](),
		stronglyReferencedByTips:  lpromise.NewValue[bool]().WithTriggerWithInitialZeroValue(true),
		referencedByTips:          lpromise.NewValue[bool]().WithTriggerWithInitialZeroValue(true),
		stronglyConnectedToTips:   lpromise.NewValue[bool](),
		weaklyConnectedToTips:     lpromise.NewValue[bool](),
		strongTip:                 lpromise.NewValue[bool](),
		weakTip:                   lpromise.NewValue[bool](),
		orphanedStrongParents:     lpromise.NewValue[int](),
		markedOrphaned:            lpromise.NewValue[bool](),
		orphaned:                  lpromise.NewValue[bool](),
		evicted:                   promise.NewEvent(),
	}).setup()
}

// ID returns the ID of the Block the TipMetadata belongs to.
func (t *TipMetadata) ID() iotago.BlockID {
	return t.block.ID()
}

// Block returns the Block the TipMetadata belongs to.
func (t *TipMetadata) Block() *blocks.Block {
	return t.block
}

// TipPool returns the TipPool the Block is currently in.
func (t *TipMetadata) TipPool() tipmanager.TipPool {
	return t.tipPool.Get()
}

// SetTipPool sets the TipPool of the Block.
func (t *TipMetadata) SetTipPool(tipPool tipmanager.TipPool) {
	t.tipPool.Compute(func(prevType tipmanager.TipPool) tipmanager.TipPool {
		return lo.Cond(tipPool > prevType, tipPool, prevType)
	})
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

// IsOrphaned returns true if the Block is orphaned.
func (t *TipMetadata) IsOrphaned() bool {
	return t.orphaned.Get()
}

// OnIsOrphanedUpdated registers a callback that is triggered when the IsOrphaned property of the Block is updated.
func (t *TipMetadata) OnIsOrphanedUpdated(handler func(isOrphaned bool)) (unsubscribe func()) {
	return t.orphaned.OnUpdate(func(_, isOrphaned bool) { handler(isOrphaned) })
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
	leaveCurrentTipPool := void

	joinTipPool := func(isReferencedByTips *lpromise.Value[bool], isTip *lpromise.Value[bool]) {
		unsubscribe := lo.Batch(
			isReferencedByTips.OnUpdate(func(_, isReferenced bool) {
				isTip.Compute(func(_ bool) bool {
					return !isReferenced && !t.orphaned.Get()
				})
			}),

			t.OnIsOrphanedUpdated(func(isOrphaned bool) {
				isTip.Compute(func(_ bool) bool {
					return !isOrphaned && !isReferencedByTips.Get()
				})
			}),
		)

		leaveCurrentTipPool = func() {
			unsubscribe()

			isTip.Set(false)

			leaveCurrentTipPool = void
		}
	}

	t.OnTipPoolUpdated(func(tipPool tipmanager.TipPool) {
		leaveCurrentTipPool()

		if tipPool == tipmanager.StrongTipPool {
			joinTipPool(t.stronglyReferencedByTips, t.strongTip)
		} else if tipPool == tipmanager.WeakTipPool {
			joinTipPool(t.referencedByTips, t.weakTip)
		}

		t.stronglyConnectedToTips.Compute(func(_ bool) bool {
			return tipPool == tipmanager.StrongTipPool || t.stronglyConnectedChildren.Get() > 0
		})

		t.weaklyConnectedToTips.Compute(func(_ bool) bool {
			return tipPool == tipmanager.WeakTipPool || t.weaklyConnectedChildren.Get() > 0
		})
	})

	t.stronglyConnectedChildren.OnUpdate(func(_, stronglyConnectedChildren int) {
		t.stronglyConnectedToTips.Compute(func(_ bool) bool {
			return stronglyConnectedChildren > 0 || t.tipPool.Get() == tipmanager.StrongTipPool
		})

		t.referencedByTips.Compute(func(_ bool) bool {
			return stronglyConnectedChildren > 0 || t.weaklyConnectedChildren.Get() > 0
		})

		t.stronglyReferencedByTips.Compute(func(_ bool) bool {
			return stronglyConnectedChildren > 0
		})
	})

	t.weaklyConnectedChildren.OnUpdate(func(_, newCount int) {
		t.weaklyConnectedToTips.Compute(func(_ bool) bool {
			return newCount > 0 || t.tipPool.Get() == tipmanager.WeakTipPool
		})

		t.referencedByTips.Compute(func(_ bool) bool {
			return newCount > 0 || t.stronglyConnectedChildren.Get() > 0
		})
	})

	t.markedOrphaned.OnUpdate(func(_, markedOrphaned bool) {
		t.orphaned.Compute(func(_ bool) bool {
			return markedOrphaned || t.orphanedStrongParents.Get() > 0
		})
	})

	t.orphanedStrongParents.OnUpdate(func(_, orphanedStrongParents int) {
		t.orphaned.Compute(func(_ bool) bool {
			return orphanedStrongParents > 0 || t.markedOrphaned.Get()
		})
	})

	return t
}

// code contract (make sure the type implements all required methods).
var _ tipmanager.TipMetadata = new(TipMetadata)

// void is a function that does nothing.
func void() {}
