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
	// block holds the block that the metadata belongs to.
	block *blocks.Block

	// tipPool holds the tip pool the block is currently assigned to.
	tipPool *lpromise.Value[tipmanager.TipPool]

	// isStrongTipPoolMember is true if the block is part of the strong tip pool and not orphaned.
	isStrongTipPoolMember *lpromise.Value[bool]

	// isWeakTipPoolMember is true if the block is part of the weak tip pool and not orphaned.
	isWeakTipPoolMember *lpromise.Value[bool]

	// isStronglyConnectedToTips is true if the block is either strongly referenced by tips or a strong tip pool member.
	isStronglyConnectedToTips *lpromise.Value[bool]

	// isWeaklyConnectedToTips is a derived property that is true if the block is either part of the weak TipPool or has
	// at least one weakly connected child.
	isWeaklyConnectedToTips *lpromise.Value[bool]

	// stronglyConnectedChildren holds the number of strong children that are strongly connected to tips.
	stronglyConnectedChildren *lpromise.Value[int]

	// weaklyConnectedChildren holds the number of weak children that are weakly connected to tips.
	weaklyConnectedChildren *lpromise.Value[int]

	// isStronglyReferencedByTips is true if the block has at least one strongly connected child.
	isStronglyReferencedByTips *lpromise.Value[bool]

	// isWeaklyReferencedByTips is a derived property that is true if the block has at least one strongly or weakly connected
	// child.
	isWeaklyReferencedByTips *lpromise.Value[bool]

	// isStrongTip is a derived property that is true if the block is part of the strong TipPool, and is not
	// isStronglyReferencedByTips.
	isStrongTip *lpromise.Value[bool]

	// isWeakTip is a derived property that is true if the block is part of the weak TipPool and isWeaklyReferencedByTips is
	// false.
	isWeakTip *lpromise.Value[bool]

	// isMarkedOrphaned is true if the block was marked as orphaned individually.
	isMarkedOrphaned *lpromise.Value[bool]

	// isOrphaned is true if the block is either marked as orphaned or has at least one orphaned strong parent.
	isOrphaned *lpromise.Value[bool]

	// orphanedStrongParents holds the number of strong parents that are orphaned.
	orphanedStrongParents *lpromise.Value[int]

	// isEvicted is triggered when the block is removed from the TipManager.
	isEvicted *promise.Event
}

// NewBlockMetadata creates a new TipMetadata instance.
func NewBlockMetadata(block *blocks.Block) *TipMetadata {
	t := &TipMetadata{
		block:                      block,
		tipPool:                    lpromise.NewValue[tipmanager.TipPool](),
		isStrongTipPoolMember:      lpromise.NewValue[bool](),
		isWeakTipPoolMember:        lpromise.NewValue[bool](),
		isStronglyConnectedToTips:  lpromise.NewValue[bool](),
		isWeaklyConnectedToTips:    lpromise.NewValue[bool](),
		stronglyConnectedChildren:  lpromise.NewValue[int](),
		weaklyConnectedChildren:    lpromise.NewValue[int](),
		isStronglyReferencedByTips: lpromise.NewValue[bool](),
		isWeaklyReferencedByTips:   lpromise.NewValue[bool](),
		isStrongTip:                lpromise.NewValue[bool](),
		isWeakTip:                  lpromise.NewValue[bool](),
		isMarkedOrphaned:           lpromise.NewValue[bool](),
		isOrphaned:                 lpromise.NewValue[bool](),
		orphanedStrongParents:      lpromise.NewValue[int](),
		isEvicted:                  promise.NewEvent(),
	}

	t.deriveIsStrongTipPoolMember()
	t.deriveIsWeakTipPoolMember()
	t.deriveIsStronglyConnectedToTips()
	t.deriveIsWeaklyConnectedToTips()
	t.deriveIsStronglyReferencedByTips()
	t.deriveIsWeaklyReferencedByTips()
	t.deriveIsStrongTip()
	t.deriveIsWeakTip()
	t.deriveIsOrphaned()

	t.OnEvicted(func() { t.SetTipPool(tipmanager.DroppedTipPool) })

	return t
}

// ID returns the identifier of the block the TipMetadata belongs to.
func (t *TipMetadata) ID() iotago.BlockID {
	return t.block.ID()
}

// Block returns the block that the TipMetadata belongs to.
func (t *TipMetadata) Block() *blocks.Block {
	return t.block
}

// TipPool returns the current TipPool of the block.
func (t *TipMetadata) TipPool() tipmanager.TipPool {
	return t.tipPool.Get()
}

// SetTipPool sets the TipPool of the block (updated by the tip selection strategy).
func (t *TipMetadata) SetTipPool(tipPool tipmanager.TipPool) {
	t.tipPool.Compute(func(prevType tipmanager.TipPool) tipmanager.TipPool {
		return lo.Cond(tipPool > prevType, tipPool, prevType)
	})
}

// OnTipPoolUpdated registers a callback that is triggered when the TipPool of the block changes.
func (t *TipMetadata) OnTipPoolUpdated(handler func(tipPool tipmanager.TipPool)) (unsubscribe func()) {
	return t.tipPool.OnUpdate(func(_, tipPool tipmanager.TipPool) { handler(tipPool) })
}

// IsStrongTip returns true if the block is currently an unreferenced strong tip.
func (t *TipMetadata) IsStrongTip() bool {
	return t.isStrongTip.Get()
}

// OnIsStrongTipUpdated registers a callback that is triggered when the IsStrongTip property changes.
func (t *TipMetadata) OnIsStrongTipUpdated(handler func(isStrongTip bool)) (unsubscribe func()) {
	return t.isStrongTip.OnUpdate(func(_, isStrongTip bool) { handler(isStrongTip) })
}

// IsWeakTip returns true if the block is an unreferenced weak tip.
func (t *TipMetadata) IsWeakTip() bool {
	return t.isWeakTip.Get()
}

// OnIsWeakTipUpdated registers a callback that is triggered when the IsWeakTip property changes.
func (t *TipMetadata) OnIsWeakTipUpdated(handler func(isWeakTip bool)) (unsubscribe func()) {
	return t.isWeakTip.OnUpdate(func(_, isWeakTip bool) { handler(isWeakTip) })
}

// SetMarkedOrphaned marks the block as orphaned (updated by the tip selection strategy).
func (t *TipMetadata) SetMarkedOrphaned(orphaned bool) {
	t.isMarkedOrphaned.Set(orphaned)
}

// IsMarkedOrphaned returns true if the block is marked as orphaned.
func (t *TipMetadata) IsMarkedOrphaned() bool {
	return t.isMarkedOrphaned.Get()
}

// OnMarkedOrphanedUpdated registers a callback that is triggered when the IsMarkedOrphaned property changes.
func (t *TipMetadata) OnMarkedOrphanedUpdated(handler func(orphaned bool)) (unsubscribe func()) {
	return t.isMarkedOrphaned.OnUpdate(func(_, isMarkedOrphaned bool) {
		handler(isMarkedOrphaned)
	})
}

// IsOrphaned returns true if the block is marked orphaned or if it has an orphaned strong parent.
func (t *TipMetadata) IsOrphaned() bool {
	return t.isOrphaned.Get()
}

// OnIsOrphanedUpdated registers a callback that is triggered when the IsOrphaned property changes.
func (t *TipMetadata) OnIsOrphanedUpdated(handler func(isOrphaned bool)) (unsubscribe func()) {
	return t.isOrphaned.OnUpdate(func(_, isOrphaned bool) { handler(isOrphaned) })
}

// IsEvicted returns true if the block was evicted from the TipManager.
func (t *TipMetadata) IsEvicted() bool {
	return t.isEvicted.WasTriggered()
}

// OnEvicted registers a callback that is triggered when the block is evicted from the TipManager.
func (t *TipMetadata) OnEvicted(handler func()) {
	t.isEvicted.OnTrigger(handler)
}

// deriveIsStrongTipPoolMember derives the isStrongTipPoolMember property.
func (t *TipMetadata) deriveIsStrongTipPoolMember() {
	t.tipPool.OnUpdate(func(_, tipPool tipmanager.TipPool) {
		t.isStrongTipPoolMember.Compute(func(_ bool) bool {
			return tipPool == tipmanager.StrongTipPool && !t.isOrphaned.Get()
		})
	})

	t.isOrphaned.OnUpdate(func(_, isOrphaned bool) {
		t.isStrongTipPoolMember.Compute(func(_ bool) bool {
			return !isOrphaned && t.tipPool.Get() == tipmanager.StrongTipPool
		})
	})
}

// deriveIsWeakTipPoolMember derives the isWeakTipPoolMember property.
func (t *TipMetadata) deriveIsWeakTipPoolMember() {
	t.isOrphaned.OnUpdate(func(_, isOrphaned bool) {
		t.isWeakTipPoolMember.Compute(func(_ bool) bool {
			return !isOrphaned && t.tipPool.Get() == tipmanager.WeakTipPool
		})
	})

	t.tipPool.OnUpdate(func(_, tipPool tipmanager.TipPool) {
		t.isWeakTipPoolMember.Compute(func(_ bool) bool {
			return tipPool == tipmanager.WeakTipPool && !t.isOrphaned.Get()
		})
	})
}

// deriveIsStronglyConnectedToTips derives the isStronglyConnectedToTips property.
func (t *TipMetadata) deriveIsStronglyConnectedToTips() {
	t.isStronglyReferencedByTips.OnUpdate(func(_, isStronglyReferencedByTips bool) {
		t.isStronglyConnectedToTips.Compute(func(_ bool) bool {
			return isStronglyReferencedByTips || t.isStrongTipPoolMember.Get()
		})
	})

	t.isStrongTipPoolMember.OnUpdate(func(_, isStrongTipPoolMember bool) {
		t.isStronglyConnectedToTips.Compute(func(_ bool) bool {
			return isStrongTipPoolMember || t.isStronglyReferencedByTips.Get()
		})
	})
}

// deriveIsWeaklyConnectedToTips derives the isWeaklyConnectedToTips property.
func (t *TipMetadata) deriveIsWeaklyConnectedToTips() {
	t.isWeaklyReferencedByTips.OnUpdate(func(_, isWeaklyReferencedByTips bool) {
		t.isWeaklyConnectedToTips.Compute(func(_ bool) bool {
			return isWeaklyReferencedByTips || t.isWeakTipPoolMember.Get() || t.isStrongTipPoolMember.Get()
		})
	})

	t.isStrongTipPoolMember.OnUpdate(func(_, isStrongTipPoolMember bool) {
		t.isWeaklyConnectedToTips.Compute(func(_ bool) bool {
			return isStrongTipPoolMember || t.isWeakTipPoolMember.Get() || t.isWeaklyReferencedByTips.Get()
		})
	})

	t.isWeakTipPoolMember.OnUpdate(func(_, isWeakTipPoolMember bool) {
		t.isWeaklyConnectedToTips.Compute(func(_ bool) bool {
			return isWeakTipPoolMember || t.isWeaklyReferencedByTips.Get() || t.isStronglyConnectedToTips.Get()
		})
	})
}

// deriveIsStronglyReferencedByTips derives the isStronglyReferencedByTips property.
func (t *TipMetadata) deriveIsStronglyReferencedByTips() {
	t.stronglyConnectedChildren.OnUpdate(func(_, stronglyConnectedChildren int) {
		t.isStronglyReferencedByTips.Compute(func(_ bool) bool {
			return stronglyConnectedChildren > 0
		})
	})
}

// deriveIsWeaklyReferencedByTips derives the isWeaklyReferencedByTips property.
func (t *TipMetadata) deriveIsWeaklyReferencedByTips() {
	t.weaklyConnectedChildren.OnUpdate(func(_, weaklyConnectedChildren int) {
		t.isWeaklyReferencedByTips.Compute(func(_ bool) bool {
			return weaklyConnectedChildren > 0 || t.isStronglyReferencedByTips.Get()
		})
	})

	t.isStronglyReferencedByTips.OnUpdate(func(_, isStronglyReferencedByTips bool) {
		t.isWeaklyReferencedByTips.Compute(func(_ bool) bool {
			return isStronglyReferencedByTips || t.weaklyConnectedChildren.Get() > 0
		})
	})
}

// deriveIsStrongTip derives the isStrongTip property.
func (t *TipMetadata) deriveIsStrongTip() {
	t.isStronglyReferencedByTips.OnUpdate(func(_, isStronglyReferencedByTips bool) {
		t.isStrongTip.Compute(func(_ bool) bool {
			return !isStronglyReferencedByTips && t.isStrongTipPoolMember.Get()
		})
	})

	t.isStrongTipPoolMember.OnUpdate(func(_, isStrongTipPoolMember bool) {
		t.isStrongTip.Compute(func(_ bool) bool {
			return isStrongTipPoolMember && !t.isStronglyReferencedByTips.Get()
		})
	})
}

// deriveIsWeakTip derives the isWeakTip property.
func (t *TipMetadata) deriveIsWeakTip() {
	t.isWeaklyReferencedByTips.OnUpdate(func(_, isWeaklyReferencedByTips bool) {
		t.isWeakTip.Compute(func(_ bool) bool {
			return !isWeaklyReferencedByTips && t.isWeakTipPoolMember.Get()
		})
	})

	t.isWeakTipPoolMember.OnUpdate(func(_, isWeakTipPoolMember bool) {
		t.isWeakTip.Compute(func(_ bool) bool {
			return isWeakTipPoolMember && !t.isWeaklyReferencedByTips.Get()
		})
	})
}

// deriveIsOrphaned derives the isOrphaned property.
func (t *TipMetadata) deriveIsOrphaned() {
	t.isMarkedOrphaned.OnUpdate(func(_, isMarkedOrphaned bool) {
		t.isOrphaned.Compute(func(_ bool) bool {
			return isMarkedOrphaned || t.orphanedStrongParents.Get() > 0
		})
	})

	t.orphanedStrongParents.OnUpdate(func(_, orphanedStrongParents int) {
		t.isOrphaned.Compute(func(_ bool) bool {
			return orphanedStrongParents > 0 || t.isMarkedOrphaned.Get()
		})
	})
}

// setupStrongParent sets up the parent and children related properties for a strong parent.
func (t *TipMetadata) setupStrongParent(strongParent *TipMetadata) {
	strongParent.OnEvicted( // unsubscribe on eviction of the parent (prevent memory leaks).
		t.isStronglyConnectedToTips.OnUpdate(func(_, isStronglyConnectedToTips bool) {
			strongParent.stronglyConnectedChildren.Compute(lo.Cond(isStronglyConnectedToTips, increase, decrease))
		}),
	)

	strongParent.OnIsOrphanedUpdated(func(isOrphaned bool) {
		t.orphanedStrongParents.Compute(lo.Cond(isOrphaned, increase, decrease))
	})
}

// setupWeakParent sets up the parent and children related properties for a weak parent.
func (t *TipMetadata) setupWeakParent(weakParent *TipMetadata) {
	weakParent.OnEvicted( // unsubscribe on eviction of the parent (prevent memory leaks).
		t.isWeaklyConnectedToTips.OnUpdate(func(_, isWeaklyConnectedToTips bool) {
			weakParent.weaklyConnectedChildren.Compute(lo.Cond(isWeaklyConnectedToTips, increase, decrease))
		}),
	)
}

// code contract (make sure the type implements all required methods).
var _ tipmanager.TipMetadata = new(TipMetadata)

// increase increases the given value by 1.
func increase(currentValue int) int {
	return currentValue + 1
}

// decrease decreases the given value by 1.
func decrease(currentValue int) int {
	return currentValue - 1
}
