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

	// isMarkedOrphaned is true if the block was marked as orphaned individually.
	isMarkedOrphaned *lpromise.Value[bool]

	// isOrphaned is true if the block is either marked as orphaned or has at least one orphaned strong parent.
	isOrphaned *lpromise.Value[bool]

	// orphanedStrongParents holds the number of strong parents that are orphaned.
	orphanedStrongParents *lpromise.Value[int]

	// tipPool holds the tip pool the block is currently assigned to.
	tipPool *lpromise.Value[tipmanager.TipPool]

	// isStrongTipPoolMember is true if the block is part of the strong tip pool and not orphaned.
	isStrongTipPoolMember *lpromise.Value[bool]

	// isWeakTipPoolMember is true if the block is part of the weak tip pool and not orphaned.
	isWeakTipPoolMember *lpromise.Value[bool]

	// isStronglyConnectedToTips is true if the block is either strongly referenced by tips or a strong tip pool member.
	isStronglyConnectedToTips *lpromise.Value[bool]

	// stronglyConnectedChildren holds the number of strong children that are strongly connected to tips.
	stronglyConnectedChildren *lpromise.Value[int]

	// isStronglyReferencedByTips is true if the block has at least one strongly connected child.
	isStronglyReferencedByTips *lpromise.Value[bool]

	// isWeaklyConnectedToTips is a derived property that is true if the block is either part of the weak TipPool or has
	// at least one weakly connected child.
	isWeaklyConnectedToTips *lpromise.Value[bool]

	// isWeaklyReferencedByTips is a derived property that is true if the block has at least one strongly or weakly connected
	// child.
	isWeaklyReferencedByTips *lpromise.Value[bool]

	// weaklyConnectedChildren holds the number of weak children that are weakly connected to tips.
	weaklyConnectedChildren *lpromise.Value[int]

	// isStrongTip is a derived property that is true if the block is part of the strong TipPool, and is not
	// isStronglyReferencedByTips.
	isStrongTip *lpromise.Value[bool]

	// isWeakTip is a derived property that is true if the block is part of the weak TipPool and isWeaklyReferencedByTips is
	// false.
	isWeakTip *lpromise.Value[bool]

	// isEvicted is triggered when the block is removed from the TipManager.
	isEvicted *promise.Event
}

// NewBlockMetadata creates a new TipMetadata instance.
func NewBlockMetadata(block *blocks.Block) *TipMetadata {
	t := &TipMetadata{
		block:                      block,
		tipPool:                    lpromise.NewValue[tipmanager.TipPool](),
		isMarkedOrphaned:           lpromise.NewValue[bool](),
		orphanedStrongParents:      lpromise.NewValue[int](),
		stronglyConnectedChildren:  lpromise.NewValue[int](),
		weaklyConnectedChildren:    lpromise.NewValue[int](),
		isOrphaned:                 lpromise.NewValue[bool](),
		isStrongTipPoolMember:      lpromise.NewValue[bool](),
		isWeakTipPoolMember:        lpromise.NewValue[bool](),
		isStronglyReferencedByTips: lpromise.NewValue[bool](),
		isWeaklyReferencedByTips:   lpromise.NewValue[bool](),
		isStronglyConnectedToTips:  lpromise.NewValue[bool](),
		isWeaklyConnectedToTips:    lpromise.NewValue[bool](),
		isStrongTip:                lpromise.NewValue[bool](),
		isWeakTip:                  lpromise.NewValue[bool](),
		isEvicted:                  promise.NewEvent(),
	}

	t.setupIsOrphaned()
	t.setupIsStrongTipPoolMember()
	t.setupIsWeakTipPoolMember()
	t.setupIsStronglyReferencedByTips()
	t.setupIsWeaklyReferencedByTips()
	t.setupIsStronglyConnectedToTips()
	t.setupIsWeaklyConnectedToTips()
	t.setupIsStrongTip()
	t.setupIsWeakTip()

	t.OnEvicted(func() {
		t.SetTipPool(tipmanager.DroppedTipPool)
	})

	return t
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
	return t.isStrongTip.Get()
}

// OnIsStrongTipUpdated registers a callback that is triggered when the IsStrongTip property of the Block is updated.
func (t *TipMetadata) OnIsStrongTipUpdated(handler func(isStrongTip bool)) (unsubscribe func()) {
	return t.isStrongTip.OnUpdate(func(_, isStrongTip bool) { handler(isStrongTip) })
}

// IsWeakTip returns true if the Block is part of the weak tip set.
func (t *TipMetadata) IsWeakTip() bool {
	return t.isWeakTip.Get()
}

// OnIsWeakTipUpdated registers a callback that is triggered when the IsWeakTip property of the Block is updated.
func (t *TipMetadata) OnIsWeakTipUpdated(handler func(isWeakTip bool)) (unsubscribe func()) {
	return t.isWeakTip.OnUpdate(func(_, isWeakTip bool) { handler(isWeakTip) })
}

// SetMarkedOrphaned marks the Block as orphaned (updated by the tip selection strategy).
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

// IsOrphaned returns true if the Block is orphaned.
func (t *TipMetadata) IsOrphaned() bool {
	return t.isOrphaned.Get()
}

// OnIsOrphanedUpdated registers a callback that is triggered when the IsOrphaned property of the Block is updated.
func (t *TipMetadata) OnIsOrphanedUpdated(handler func(isOrphaned bool)) (unsubscribe func()) {
	return t.isOrphaned.OnUpdate(func(_, isOrphaned bool) { handler(isOrphaned) })
}

// IsEvicted returns true if the Block was removed from the TipManager.
func (t *TipMetadata) IsEvicted() bool {
	return t.isEvicted.WasTriggered()
}

// OnEvicted registers a callback that is triggered when the Block is removed from the TipManager.
func (t *TipMetadata) OnEvicted(handler func()) {
	t.isEvicted.OnTrigger(handler)
}

func (t *TipMetadata) setupIsOrphaned() {
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

func (t *TipMetadata) setupIsStrongTipPoolMember() {
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

func (t *TipMetadata) setupIsWeakTipPoolMember() {
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

func (t *TipMetadata) setupIsStronglyReferencedByTips() {
	t.stronglyConnectedChildren.OnUpdate(func(_, stronglyConnectedChildren int) {
		t.isStronglyReferencedByTips.Compute(func(_ bool) bool {
			return stronglyConnectedChildren > 0
		})
	})
}

func (t *TipMetadata) setupIsWeaklyReferencedByTips() {
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

func (t *TipMetadata) setupIsStronglyConnectedToTips() {
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

func (t *TipMetadata) setupIsWeaklyConnectedToTips() {
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

func (t *TipMetadata) setupIsStrongTip() {
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

func (t *TipMetadata) setupIsWeakTip() {
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

func (t *TipMetadata) setupStrongParent(strongParent *TipMetadata) {
	// unsubscribe on eviction of the parent (prevent memory leaks).
	strongParent.OnEvicted(
		t.isStronglyConnectedToTips.OnUpdate(func(_, isStronglyConnectedToTips bool) {
			strongParent.stronglyConnectedChildren.Compute(lo.Cond(isStronglyConnectedToTips, increase, decrease))
		}),
	)

	strongParent.OnIsOrphanedUpdated(func(isOrphaned bool) {
		t.orphanedStrongParents.Compute(lo.Cond(isOrphaned, increase, decrease))
	})
}

func (t *TipMetadata) setupWeakParent(weakParent *TipMetadata) {
	// unsubscribe on eviction of the parent (prevent memory leaks).
	weakParent.OnEvicted(
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
