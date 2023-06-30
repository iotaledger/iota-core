package tipmanagerv1

import (
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/promise"
	"github.com/iotaledger/iota-core/pkg/core/value"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TipMetadata represents the metadata for a block in the TipManager.
type TipMetadata struct {
	// block holds the block that the metadata belongs to.
	block *blocks.Block

	// tipPool holds the tip pool the block is currently assigned to.
	tipPool *value.Value[tipmanager.TipPool]

	// isStrongTipPoolMember is true if the block is part of the strong tip pool and not orphaned.
	isStrongTipPoolMember *value.Value[bool]

	// isWeakTipPoolMember is true if the block is part of the weak tip pool and not orphaned.
	isWeakTipPoolMember *value.Value[bool]

	// isStronglyConnectedToTips is true if the block is either strongly referenced by others tips or is itself a strong
	// tip pool member.
	isStronglyConnectedToTips *value.Value[bool]

	// isConnectedToTips is true if the block is either referenced by others tips or is itself a weak or strong tip pool
	// member.
	isConnectedToTips *value.Value[bool]

	// stronglyConnectedStrongChildren holds the number of strong children that are strongly connected to the tips.
	stronglyConnectedStrongChildren *value.Value[int]

	// connectedWeakChildren holds the number of weak children that are connected to the tips.
	connectedWeakChildren *value.Value[int]

	// isStronglyReferencedByOtherTips is true if the block has at least one strong child that is strongly connected
	// to the tips.
	isStronglyReferencedByOtherTips *value.Value[bool]

	// isReferencedByOtherTips is true if the block is strongly referenced by other tips or has at least one weak child
	// that is connected to the tips.
	isReferencedByOtherTips *value.Value[bool]

	// isStrongTip is true if the block is a strong tip pool member and is not strongly referenced by other tips.
	isStrongTip *value.Value[bool]

	// isWeakTip is true if the block is a weak tip pool member and is not referenced by other tips.
	isWeakTip *value.Value[bool]

	// isLivenessThresholdReached is true if the block has reached the liveness threshold.
	isLivenessThresholdReached *value.Value[bool]

	// isMarkedOrphaned is true if the liveness threshold has been reached and the block was not accepted.
	isMarkedOrphaned *value.Value[bool]

	// isOrphaned is true if the block is either strongly or weakly orphaned.
	isOrphaned *value.Value[bool]

	// stronglyOrphanedStrongParents holds the number of strong parents that are strongly orphaned.
	stronglyOrphanedStrongParents *value.Value[int]

	// weaklyOrphanedWeakParents holds the number of weak parents that are weakly orphaned.
	weaklyOrphanedWeakParents *value.Value[int]

	// anyStrongParentStronglyOrphaned is true if the block has at least one orphaned parent.
	anyStrongParentStronglyOrphaned *value.Value[bool]

	// anyWeakParentWeaklyOrphaned is true if the block has at least one weak parent that is weakly orphaned.
	anyWeakParentWeaklyOrphaned *value.Value[bool]

	// isEvicted is triggered when the block is removed from the TipManager.
	isEvicted *promise.Event

	// isStronglyOrphaned is true if the block is either marked as orphaned, any of its strong parents is strongly
	// orphaned or any of its weak parents is weakly orphaned.
	isStronglyOrphaned *value.Value[bool]

	// isWeaklyOrphaned is true if the block is either marked as orphaned or has at least one weakly orphaned weak
	// parent.
	isWeaklyOrphaned *value.Value[bool]
}

// NewBlockMetadata creates a new TipMetadata instance.
func NewBlockMetadata(block *blocks.Block) *TipMetadata {
	t := &TipMetadata{
		// main properties
		block:                      block,
		tipPool:                    value.New[tipmanager.TipPool](),
		isLivenessThresholdReached: value.New[bool](),
		isMarkedOrphaned:           value.New[bool](),
		isEvicted:                  promise.NewEvent(),

		// derived properties (internal flags)
		isStrongTipPoolMember:           value.New[bool](),
		isWeakTipPoolMember:             value.New[bool](),
		isStronglyConnectedToTips:       value.New[bool](),
		isConnectedToTips:               value.New[bool](),
		isStronglyReferencedByOtherTips: value.New[bool](),
		isReferencedByOtherTips:         value.New[bool](),
		isStrongTip:                     value.New[bool](),
		isWeakTip:                       value.New[bool](),
		isOrphaned:                      value.New[bool](),
		anyStrongParentStronglyOrphaned: value.New[bool](),
		anyWeakParentWeaklyOrphaned:     value.New[bool](),
		isStronglyOrphaned:              value.New[bool](),
		isWeaklyOrphaned:                value.New[bool](),

		// derived properties (relational counters)
		stronglyConnectedStrongChildren: value.New[int](),
		connectedWeakChildren:           value.New[int](),
		stronglyOrphanedStrongParents:   value.New[int](),
		weaklyOrphanedWeakParents:       value.New[int](),
	}

	value.DeriveFrom2(t.isStrongTipPoolMember, isStrongTipPoolMember, t.tipPool, t.isOrphaned)
	value.DeriveFrom2(t.isWeakTipPoolMember, isWeakTipPoolMember, t.tipPool, t.isOrphaned)
	value.DeriveFrom2(t.isStronglyConnectedToTips, isStronglyConnectedToTips, t.isStrongTipPoolMember, t.isStronglyReferencedByOtherTips)
	value.DeriveFrom3(t.isConnectedToTips, isConnectedToTips, t.isReferencedByOtherTips, t.isStrongTipPoolMember, t.isWeakTipPoolMember)
	value.DeriveFrom1(t.isStronglyReferencedByOtherTips, isStronglyReferencedByOtherTips, t.stronglyConnectedStrongChildren)
	value.DeriveFrom2(t.isReferencedByOtherTips, isReferencedByOtherTips, t.connectedWeakChildren, t.isStronglyReferencedByOtherTips)
	value.DeriveFrom2(t.isStrongTip, isStrongTip, t.isStrongTipPoolMember, t.isStronglyReferencedByOtherTips)
	value.DeriveFrom2(t.isWeakTip, isWeakTip, t.isWeakTipPoolMember, t.isReferencedByOtherTips)
	value.DeriveFrom2(t.isOrphaned, isOrphaned, t.isStronglyOrphaned, t.isWeaklyOrphaned)
	value.DeriveFrom1(t.anyStrongParentStronglyOrphaned, anyStrongParentStronglyOrphaned, t.stronglyOrphanedStrongParents)
	value.DeriveFrom1(t.anyWeakParentWeaklyOrphaned, anyWeakParentWeaklyOrphaned, t.weaklyOrphanedWeakParents)
	value.DeriveFrom3(t.isStronglyOrphaned, isStronglyOrphaned, t.isMarkedOrphaned, t.anyStrongParentStronglyOrphaned, t.anyWeakParentWeaklyOrphaned)
	value.DeriveFrom2(t.isWeaklyOrphaned, isWeaklyOrphaned, t.isMarkedOrphaned, t.anyWeakParentWeaklyOrphaned)
	value.DeriveFrom1(t.isMarkedOrphaned, isMarkedOrphaned, t.isLivenessThresholdReached)

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

// SetLivenessThresholdReached marks the block as having reached the liveness threshold.
func (t *TipMetadata) SetLivenessThresholdReached() {
	t.isLivenessThresholdReached.Set(true)
}

// OnLivenessThresholdReached registers a callback that is triggered when the block reaches the liveness threshold.
func (t *TipMetadata) OnLivenessThresholdReached(handler func()) (unsubscribe func()) {
	return t.isLivenessThresholdReached.OnUpdate(func(_, _ bool) { handler() })
}

// IsLivenessThresholdReached returns true if the block reached the liveness threshold.
func (t *TipMetadata) IsLivenessThresholdReached() bool {
	return t.isLivenessThresholdReached.Get()
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

// setupStrongParent sets up the parent and children related properties for a strong parent.
func (t *TipMetadata) setupStrongParent(strongParent *TipMetadata) {
	strongParent.OnEvicted( // unsubscribe on eviction of the parent (prevent memory leaks).
		t.isStronglyConnectedToTips.OnUpdate(func(_, isStronglyConnectedToTips bool) {
			strongParent.stronglyConnectedStrongChildren.Compute(lo.Cond(isStronglyConnectedToTips, increase, decrease))
		}),
	)

	strongParent.isStronglyOrphaned.OnUpdate(func(_, isOrphaned bool) {
		t.stronglyOrphanedStrongParents.Compute(lo.Cond(isOrphaned, increase, decrease))
	})
}

// setupWeakParent sets up the parent and children related properties for a weak parent.
func (t *TipMetadata) setupWeakParent(weakParent *TipMetadata) {
	weakParent.OnEvicted( // unsubscribe on eviction of the parent (prevent memory leaks).
		t.isConnectedToTips.OnUpdate(func(_, isConnectedToTips bool) {
			weakParent.connectedWeakChildren.Compute(lo.Cond(isConnectedToTips, increase, decrease))
		}),
	)
}

// code contract (make sure the type implements all required methods).
var _ tipmanager.TipMetadata = new(TipMetadata)

// isStrongTipPoolMember returns true if the tipPool is the strong tip pool and the block is not orphaned.
func isStrongTipPoolMember(tipPool tipmanager.TipPool, isOrphaned bool) bool {
	return tipPool == tipmanager.StrongTipPool && !isOrphaned
}

// isWeakTipPoolMember returns true if the tipPool is the weak tip pool and the block is not orphaned.
func isWeakTipPoolMember(tipPool tipmanager.TipPool, isOrphaned bool) bool {
	return tipPool == tipmanager.WeakTipPool && !isOrphaned
}

// isStronglyConnectedToTips returns true if the block is a strong tip pool member or if it is strongly referenced by
// other tips.
func isStronglyConnectedToTips(isStrongTipPoolMember bool, isStronglyReferencedByOtherTips bool) bool {
	return isStrongTipPoolMember || isStronglyReferencedByOtherTips
}

// isConnectedToTips returns true if the block is a weak or strong tip pool member or if it is referenced by other tips.
func isConnectedToTips(isReferencedByOtherTips bool, isStrongTipPoolMember bool, isWeakTipPoolMember bool) bool {
	return isReferencedByOtherTips || isStrongTipPoolMember || isWeakTipPoolMember
}

// isStronglyReferencedByOtherTips returns true if the block has at least one strongly connected strong child.
func isStronglyReferencedByOtherTips(stronglyConnectedStrongChildren int) bool {
	return stronglyConnectedStrongChildren > 0
}

// isReferencedByOtherTips returns true if the block has at least one connected weak child or if it is strongly
// referenced by other tips.
func isReferencedByOtherTips(connectedWeakChildren int, isStronglyReferencedByOtherTips bool) bool {
	return connectedWeakChildren > 0 || isStronglyReferencedByOtherTips
}

// isStrongTip returns true if the block is a strong tip pool member and is not strongly referenced by other tips.
func isStrongTip(isStrongTipPoolMember bool, isStronglyReferencedByOtherTips bool) bool {
	return isStrongTipPoolMember && !isStronglyReferencedByOtherTips
}

// isWeakTip returns true if the block is a weak tip pool member and is not referenced by other tips.
func isWeakTip(isWeakTipPoolMember bool, isReferencedByOtherTips bool) bool {
	return isWeakTipPoolMember && !isReferencedByOtherTips
}

// isOrphaned returns true if the block is marked orphaned or if it has an orphaned strong parent.
func isOrphaned(isStronglyOrphaned bool, isWeaklyOrphaned bool) bool {
	return isStronglyOrphaned || isWeaklyOrphaned
}

// anyStrongParentStronglyOrphaned returns true if any of the strong parents is strongly orphaned.
func anyStrongParentStronglyOrphaned(stronglyOrphanedStrongParents int) bool {
	return stronglyOrphanedStrongParents > 0
}

// anyWeakParentWeaklyOrphaned returns true if any of the weak parents is weakly orphaned.
func anyWeakParentWeaklyOrphaned(weaklyOrphanedWeakParents int) bool {
	return weaklyOrphanedWeakParents > 0
}

// isStronglyOrphaned returns true if the block is marked orphaned or if any of the strong parents is strongly orphaned
// or if any of the weak parents is weakly orphaned.
func isStronglyOrphaned(isMarkedOrphaned, anyStrongParentStronglyOrphaned, anyWeakParentWeaklyOrphaned bool) bool {
	return isMarkedOrphaned || anyStrongParentStronglyOrphaned || anyWeakParentWeaklyOrphaned
}

// isWeaklyOrphaned returns true if the block is marked orphaned or if any of the weak parents is weakly orphaned.
func isWeaklyOrphaned(isMarkedOrphaned, anyWeakParentWeaklyOrphaned bool) bool {
	return isMarkedOrphaned || anyWeakParentWeaklyOrphaned
}

// isMarkedOrphaned returns true if the block is marked orphaned.
func isMarkedOrphaned(isLivenessThresholdReached bool) bool {
	accepted := false

	return isLivenessThresholdReached && !accepted
}

// increase increases the given value by 1.
func increase(currentValue int) int {
	return currentValue + 1
}

// decrease decreases the given value by 1.
func decrease(currentValue int) int {
	return currentValue - 1
}
