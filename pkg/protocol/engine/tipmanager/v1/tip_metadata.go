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
	stronglyConnectedStrongChildren *agential.ThresholdTransformer[bool]

	// connectedWeakChildren holds the number of weak children that are connected to the tips.
	connectedWeakChildren *agential.ThresholdTransformer[bool]

	// stronglyOrphanedStrongParents holds the number of strong parents that are strongly orphaned.
	stronglyOrphanedStrongParents *agential.ThresholdTransformer[bool]

	// weaklyOrphanedWeakParents holds the number of weak parents that are weakly orphaned.
	weaklyOrphanedWeakParents *agential.ThresholdTransformer[bool]

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

	constructed agential.Receptor[bool]
}

// NewBlockMetadata creates a new TipMetadata instance.
func NewBlockMetadata(block *blocks.Block) *TipMetadata {
	t := &TipMetadata{
		block:                           block,
		constructed:                     agential.NewReceptor[bool](preventResets[bool]),
		tipPool:                         agential.NewReceptor[tipmanager.TipPool](tipmanager.TipPool.Max),
		isLivenessThresholdReached:      agential.NewReceptor[bool](preventResets[bool]),
		isEvicted:                       agential.NewReceptor[bool](preventResets[bool]),
		stronglyConnectedStrongChildren: agential.NewThresholdTransformer[bool](),
		connectedWeakChildren:           agential.NewThresholdTransformer[bool](),
		stronglyOrphanedStrongParents:   agential.NewThresholdTransformer[bool](),
		weaklyOrphanedWeakParents:       agential.NewThresholdTransformer[bool](),
	}

	t.isStrongTipPoolMember = agential.NewTransformerWith2Inputs(t, func(tipPool tipmanager.TipPool, isOrphaned bool) bool {
		return tipPool == tipmanager.StrongTipPool && !isOrphaned
	}, &t.tipPool, &t.isOrphaned)

	t.isWeakTipPoolMember = agential.NewTransformerWith2Inputs(t, func(tipPool tipmanager.TipPool, isOrphaned bool) bool {
		return tipPool == tipmanager.WeakTipPool && !isOrphaned
	}, &t.tipPool, &t.isOrphaned)

	t.isStronglyConnectedToTips = agential.NewTransformerWith2Inputs(t, func(isStrongTipPoolMember bool, IsStronglyReferencedByStronglyConnectedTips bool) bool {
		return isStrongTipPoolMember || IsStronglyReferencedByStronglyConnectedTips
	}, &t.isStrongTipPoolMember, &t.isStronglyReferencedByStronglyConnectedTips)

	t.isConnectedToTips = agential.NewTransformerWith3Inputs(t, func(isReferencedByOtherTips bool, isStrongTipPoolMember bool, isWeakTipPoolMember bool) bool {
		return isReferencedByOtherTips || isStrongTipPoolMember || isWeakTipPoolMember
	}, &t.isReferencedByOtherTips, &t.isStrongTipPoolMember, &t.isWeakTipPoolMember)

	t.isStronglyReferencedByStronglyConnectedTips = agential.NewTransformerWith1Input[bool, int](t, func(stronglyConnectedStrongChildren int) bool {
		return stronglyConnectedStrongChildren > 0
	}, &t.stronglyConnectedStrongChildren)

	t.isReferencedByOtherTips = agential.NewTransformerWith2Inputs[bool, int, bool](t, func(connectedWeakChildren int, isStronglyReferencedByStronglyConnectedTips bool) bool {
		return connectedWeakChildren > 0 || isStronglyReferencedByStronglyConnectedTips
	}, &t.connectedWeakChildren, &t.isStronglyReferencedByStronglyConnectedTips)

	t.isStrongTip = agential.NewTransformerWith2Inputs[bool, bool, bool](t, func(isStrongTipPoolMember bool, isStronglyReferencedByOtherTips bool) bool {
		return isStrongTipPoolMember && !isStronglyReferencedByOtherTips
	}, &t.isStrongTipPoolMember, &t.isStronglyReferencedByStronglyConnectedTips)

	t.isWeakTip = agential.NewTransformerWith2Inputs[bool, bool, bool](t, func(isWeakTipPoolMember bool, isReferencedByOtherTips bool) bool {
		return isWeakTipPoolMember && !isReferencedByOtherTips
	}, &t.isWeakTipPoolMember, &t.isReferencedByOtherTips)

	t.isOrphaned = agential.NewTransformerWith2Inputs[bool, bool, bool](t, func(isStronglyOrphaned bool, isWeaklyOrphaned bool) bool {
		return isStronglyOrphaned || isWeaklyOrphaned
	}, &t.isStronglyOrphaned, &t.isWeaklyOrphaned)

	t.anyStrongParentStronglyOrphaned = agential.NewTransformerWith1Input[bool, int](t, func(stronglyOrphanedStrongParents int) bool {
		return stronglyOrphanedStrongParents > 0
	}, &t.stronglyOrphanedStrongParents)

	t.anyWeakParentWeaklyOrphaned = agential.NewTransformerWith1Input[bool, int](t, func(weaklyOrphanedWeakParents int) bool {
		return weaklyOrphanedWeakParents > 0
	}, &t.weaklyOrphanedWeakParents)

	t.isStronglyOrphaned = agential.NewTransformerWith3Inputs[bool, bool, bool, bool](t, func(isMarkedOrphaned, anyStrongParentStronglyOrphaned, anyWeakParentWeaklyOrphaned bool) bool {
		return isMarkedOrphaned || anyStrongParentStronglyOrphaned || anyWeakParentWeaklyOrphaned
	}, &t.isMarkedOrphaned, &t.anyStrongParentStronglyOrphaned, &t.anyWeakParentWeaklyOrphaned)

	t.isWeaklyOrphaned = agential.NewTransformerWith2Inputs[bool, bool, bool](t, func(isMarkedOrphaned, anyWeakParentWeaklyOrphaned bool) bool {
		return isMarkedOrphaned || anyWeakParentWeaklyOrphaned
	}, &t.isMarkedOrphaned, &t.anyWeakParentWeaklyOrphaned)

	t.isMarkedOrphaned = agential.NewTransformerWith1Input[bool, bool](t, func(isLivenessThresholdReached bool) bool {
		return isLivenessThresholdReached /* TODO: && !accepted */
	}, &t.isLivenessThresholdReached)

	t.isEvicted.OnUpdate(func(_, _ bool) { t.tipPool.Set(tipmanager.DroppedTipPool) })

	t.constructed.Set(true)

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

// TipPool is a receptor that holds the tip pool the block is currently in.
func (t *TipMetadata) TipPool() agential.Receptor[tipmanager.TipPool] {
	return t.tipPool
}

// LivenessThresholdReached is a receptor that holds a boolean value indicating if the liveness threshold is reached.
func (t *TipMetadata) LivenessThresholdReached() agential.Receptor[bool] {
	return t.isLivenessThresholdReached
}

// IsStrongTip returns true if the block is currently an unreferenced strong tip.
func (t *TipMetadata) IsStrongTip() agential.ReadOnlyReceptor[bool] {
	return t.isStrongTip
}

// IsWeakTip returns true if the block is an unreferenced weak tip.
func (t *TipMetadata) IsWeakTip() agential.ReadOnlyReceptor[bool] {
	return t.isWeakTip
}

// IsOrphaned returns true if the block is marked orphaned or if it has an orphaned strong parent.
func (t *TipMetadata) IsOrphaned() agential.ReadOnlyReceptor[bool] {
	return t.isOrphaned
}

func (t *TipMetadata) Evicted() agential.ReadOnlyReceptor[bool] {
	return t.isEvicted
}

func (t *TipMetadata) OnConstructed(handler func()) (unsubscribe func()) {
	return t.constructed.OnUpdate(func(_, _ bool) {
		handler()
	})
}

// connectStrongParent sets up the parent and children related properties for a strong parent.
func (t *TipMetadata) connectStrongParent(strongParent *TipMetadata) {
	unsubscribe := lo.Batch(
		strongParent.stronglyConnectedStrongChildren.ProvideInput(t.isStronglyConnectedToTips),

		t.stronglyOrphanedStrongParents.ProvideInput(strongParent.isStronglyOrphaned),
	)

	strongParent.isEvicted.OnUpdate(func(_, _ bool) { unsubscribe() })
}

// connectWeakParent sets up the parent and children related properties for a weak parent.
func (t *TipMetadata) connectWeakParent(weakParent *TipMetadata) {
	unsubscribe := lo.Batch(
		weakParent.connectedWeakChildren.ProvideInput(t.isConnectedToTips),

		t.weaklyOrphanedWeakParents.ProvideInput(weakParent.isWeaklyOrphaned),
	)

	weakParent.isEvicted.OnUpdate(func(_, _ bool) { unsubscribe() })
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
