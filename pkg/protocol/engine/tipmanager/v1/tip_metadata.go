package tipmanagerv1

import (
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/agential"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TipMetadata represents the metadata for a block in the TipManager.
type TipMetadata struct {
	// block holds the block that the metadata belongs to.
	block *blocks.Block

	// tipPool holds the tip pool the block is currently assigned to.
	tipPool agential.Receptor[tipmanager.TipPool]

	// isLivenessThresholdReached is true if the block has reached the liveness threshold.
	isLivenessThresholdReached agential.Receptor[bool]

	// stronglyConnectedStrongChildren holds the number of strong children that are strongly connected to the tips.
	stronglyConnectedStrongChildren *agential.ThresholdReceptor[int, bool]

	// connectedWeakChildren holds the number of weak children that are connected to the tips.
	connectedWeakChildren *agential.ThresholdReceptor[int, bool]

	// stronglyOrphanedStrongParents holds the number of strong parents that are strongly orphaned.
	stronglyOrphanedStrongParents *agential.ThresholdReceptor[int, bool]

	// weaklyOrphanedWeakParents holds the number of weak parents that are weakly orphaned.
	weaklyOrphanedWeakParents *agential.ThresholdReceptor[int, bool]

	// isStrongTipPoolMember is true if the block is part of the strong tip pool and not orphaned.
	isStrongTipPoolMember *agential.TransformerWith2Inputs[bool, tipmanager.TipPool, bool]

	// isWeakTipPoolMember is true if the block is part of the weak tip pool and not orphaned.
	isWeakTipPoolMember *agential.TransformerWith2Inputs[bool, tipmanager.TipPool, bool]

	// isStronglyConnectedToTips is true if the block is either strongly referenced by others tips or is itself a strong
	// tip pool member.
	isStronglyConnectedToTips *agential.TransformerWith2Inputs[bool, bool, bool]

	// isConnectedToTips is true if the block is either referenced by others tips or is itself a weak or strong tip pool
	// member.
	isConnectedToTips *agential.TransformerWith3Inputs[bool, bool, bool, bool]

	// isStronglyReferencedByStronglyConnectedTips is true if the block has at least one strong child that is strongly connected
	// to the tips.
	isStronglyReferencedByStronglyConnectedTips *agential.TransformerWith1Input[bool, int]

	// isReferencedByOtherTips is true if the block is strongly referenced by other tips or has at least one weak child
	// that is connected to the tips.
	isReferencedByOtherTips *agential.TransformerWith2Inputs[bool, int, bool]

	// isStrongTip is true if the block is a strong tip pool member and is not strongly referenced by other tips.
	isStrongTip *agential.TransformerWith2Inputs[bool, bool, bool]

	// isWeakTip is true if the block is a weak tip pool member and is not referenced by other tips.
	isWeakTip *agential.TransformerWith2Inputs[bool, bool, bool]

	// isMarkedOrphaned is true if the liveness threshold has been reached and the block was not accepted.
	isMarkedOrphaned *agential.TransformerWith1Input[bool, bool]

	// isOrphaned is true if the block is either strongly or weakly orphaned.
	isOrphaned *agential.TransformerWith2Inputs[bool, bool, bool]

	// anyStrongParentStronglyOrphaned is true if the block has at least one orphaned parent.
	anyStrongParentStronglyOrphaned *agential.TransformerWith1Input[bool, int]

	// anyWeakParentWeaklyOrphaned is true if the block has at least one weak parent that is weakly orphaned.
	anyWeakParentWeaklyOrphaned *agential.TransformerWith1Input[bool, int]

	// isEvicted is triggered when the block is removed from the TipManager.
	isEvicted agential.Receptor[bool]

	// isStronglyOrphaned is true if the block is either marked as orphaned, any of its strong parents is strongly
	// orphaned or any of its weak parents is weakly orphaned.
	isStronglyOrphaned *agential.TransformerWith3Inputs[bool, bool, bool, bool]

	// isWeaklyOrphaned is true if the block is either marked as orphaned or has at least one weakly orphaned weak
	// parent.
	isWeaklyOrphaned *agential.TransformerWith2Inputs[bool, bool, bool]
}

func (t *TipMetadata) Evicted() agential.ReadOnlyReceptor[bool] {
	return t.isEvicted.ReadOnly()
}

// NewBlockMetadata creates a new TipMetadata instance.
func NewBlockMetadata(block *blocks.Block) *TipMetadata {
	t := &TipMetadata{
		block:                           block,
		tipPool:                         agential.NewReceptor[tipmanager.TipPool](tipmanager.TipPool.Max),
		isLivenessThresholdReached:      agential.NewReceptor[bool](),
		stronglyConnectedStrongChildren: agential.NewThresholdReceptor[int, bool](),
		connectedWeakChildren:           agential.NewThresholdReceptor[int, bool](),
		stronglyOrphanedStrongParents:   agential.NewThresholdReceptor[int, bool](),
		weaklyOrphanedWeakParents:       agential.NewThresholdReceptor[int, bool](),
		isEvicted:                       agential.NewReceptor[bool](preventResets[bool]),

		isStrongTipPoolMember: agential.NewTransformerWith2Inputs(func(tipPool tipmanager.TipPool, isOrphaned bool) bool {
			return tipPool == tipmanager.StrongTipPool && !isOrphaned
		}),
		isWeakTipPoolMember: agential.NewTransformerWith2Inputs(func(tipPool tipmanager.TipPool, isOrphaned bool) bool {
			return tipPool == tipmanager.WeakTipPool && !isOrphaned
		}),
		isStronglyConnectedToTips: agential.NewTransformerWith2Inputs(func(isStrongTipPoolMember bool, isStronglyReferencedByOtherTips bool) bool {
			return isStrongTipPoolMember || isStronglyReferencedByOtherTips
		}),
		isConnectedToTips: agential.NewTransformerWith3Inputs(func(isReferencedByOtherTips bool, isStrongTipPoolMember bool, isWeakTipPoolMember bool) bool {
			return isReferencedByOtherTips || isStrongTipPoolMember || isWeakTipPoolMember
		}),
		isStronglyReferencedByStronglyConnectedTips: agential.NewTransformerWith1Input[bool, int](func(stronglyConnectedStrongChildren int) bool {
			return stronglyConnectedStrongChildren > 0
		}),
		isReferencedByOtherTips: agential.NewTransformerWith2Inputs[bool, int, bool](func(connectedWeakChildren int, isStronglyReferencedByOtherTips bool) bool {
			return connectedWeakChildren > 0 || isStronglyReferencedByOtherTips
		}),
		isStrongTip: agential.NewTransformerWith2Inputs[bool, bool, bool](func(isStrongTipPoolMember bool, isStronglyReferencedByOtherTips bool) bool {
			return isStrongTipPoolMember && !isStronglyReferencedByOtherTips
		}),
		isWeakTip: agential.NewTransformerWith2Inputs[bool, bool, bool](func(isWeakTipPoolMember bool, isReferencedByOtherTips bool) bool {
			return isWeakTipPoolMember && !isReferencedByOtherTips
		}),
		isOrphaned: agential.NewTransformerWith2Inputs[bool, bool, bool](func(isStronglyOrphaned bool, isWeaklyOrphaned bool) bool {
			return isStronglyOrphaned || isWeaklyOrphaned
		}),
		anyStrongParentStronglyOrphaned: agential.NewTransformerWith1Input[bool, int](func(stronglyOrphanedStrongParents int) bool {
			return stronglyOrphanedStrongParents > 0
		}),
		anyWeakParentWeaklyOrphaned: agential.NewTransformerWith1Input[bool, int](func(weaklyOrphanedWeakParents int) bool {
			return weaklyOrphanedWeakParents > 0
		}),
		isStronglyOrphaned: agential.NewTransformerWith3Inputs[bool, bool, bool, bool](func(isMarkedOrphaned, anyStrongParentStronglyOrphaned, anyWeakParentWeaklyOrphaned bool) bool {
			return isMarkedOrphaned || anyStrongParentStronglyOrphaned || anyWeakParentWeaklyOrphaned
		}),
		isWeaklyOrphaned: agential.NewTransformerWith2Inputs[bool, bool, bool](func(isMarkedOrphaned, anyWeakParentWeaklyOrphaned bool) bool {
			return isMarkedOrphaned || anyWeakParentWeaklyOrphaned
		}),
		isMarkedOrphaned: agential.NewTransformerWith1Input[bool, bool](func(isLivenessThresholdReached bool) bool {
			return isLivenessThresholdReached /* TODO: && !accepted */
		}),
	}

	t.connectGates()

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
func (t *TipMetadata) TipPool() agential.Receptor[tipmanager.TipPool] {
	return t.tipPool
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
	return t.isEvicted.Get()
}

// OnEvicted registers a callback that is triggered when the block is evicted from the TipManager.
func (t *TipMetadata) OnEvicted(handler func()) {
	t.isEvicted.OnUpdate(func(old, new bool) {
		handler()
	})
}

func (t *TipMetadata) connectGates() {
	t.isStrongTipPoolMember.ProvideInputs(t.tipPool, t.isOrphaned)
	t.isWeakTipPoolMember.ProvideInputs(t.tipPool, t.isOrphaned)
	t.isStronglyConnectedToTips.ProvideInputs(t.isStrongTipPoolMember, t.isStronglyReferencedByStronglyConnectedTips)
	t.isConnectedToTips.ProvideInputs(t.isReferencedByOtherTips, t.isStrongTipPoolMember, t.isWeakTipPoolMember)
	t.isStronglyReferencedByStronglyConnectedTips.ProvideInputs(t.stronglyConnectedStrongChildren)
	t.isReferencedByOtherTips.ProvideInputs(t.connectedWeakChildren, t.isStronglyReferencedByStronglyConnectedTips)
	t.isStrongTip.ProvideInputs(t.isStrongTipPoolMember, t.isStronglyReferencedByStronglyConnectedTips)
	t.isWeakTip.ProvideInputs(t.isWeakTipPoolMember, t.isReferencedByOtherTips)
	t.isOrphaned.ProvideInputs(t.isStronglyOrphaned, t.isWeaklyOrphaned)
	t.anyStrongParentStronglyOrphaned.ProvideInputs(t.stronglyOrphanedStrongParents)
	t.anyWeakParentWeaklyOrphaned.ProvideInputs(t.weaklyOrphanedWeakParents)
	t.isStronglyOrphaned.ProvideInputs(t.isMarkedOrphaned, t.anyStrongParentStronglyOrphaned, t.anyWeakParentWeaklyOrphaned)
	t.isWeaklyOrphaned.ProvideInputs(t.isMarkedOrphaned, t.anyWeakParentWeaklyOrphaned)
	t.isMarkedOrphaned.ProvideInputs(t.isLivenessThresholdReached)
}

// connectStrongParent sets up the parent and children related properties for a strong parent.
func (t *TipMetadata) connectStrongParent(strongParent *TipMetadata) {
	strongParent.OnEvicted(lo.Batch(
		strongParent.stronglyConnectedStrongChildren.ProvideInput(t.isStronglyConnectedToTips),

		t.stronglyOrphanedStrongParents.ProvideInput(strongParent.isStronglyOrphaned),
	))
}

// connectWeakParent sets up the parent and children related properties for a weak parent.
func (t *TipMetadata) connectWeakParent(weakParent *TipMetadata) {
	weakParent.OnEvicted(lo.Batch(
		weakParent.connectedWeakChildren.ProvideInput(t.isConnectedToTips),

		t.weaklyOrphanedWeakParents.ProvideInput(weakParent.isWeaklyOrphaned),
	))
}

// code contract (make sure the type implements all required methods).
var _ tipmanager.TipMetadata = new(TipMetadata)

func preventResets[T comparable](prevValue, newValue T) T {
	var emptyValue T
	if newValue == emptyValue {
		return prevValue
	}

	return newValue
}
