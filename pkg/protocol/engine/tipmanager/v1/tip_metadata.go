package tipmanagerv1

import (
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
	tipPool *agential.ValueReceptor[tipmanager.TipPool]

	// isLivenessThresholdReached is true if the block has reached the liveness threshold.
	isLivenessThresholdReached *agential.ValueReceptor[bool]

	// stronglyConnectedStrongChildren holds the number of strong children that are strongly connected to the tips.
	stronglyConnectedStrongChildren *agential.Counter[bool]

	// connectedWeakChildren holds the number of weak children that are connected to the tips.
	connectedWeakChildren *agential.Counter[bool]

	// stronglyOrphanedStrongParents holds the number of strong parents that are strongly orphaned.
	stronglyOrphanedStrongParents *agential.Counter[bool]

	// weaklyOrphanedWeakParents holds the number of weak parents that are weakly orphaned.
	weaklyOrphanedWeakParents *agential.Counter[bool]

	// isStrongTipPoolMember is true if the block is part of the strong tip pool and not orphaned.
	isStrongTipPoolMember *agential.DerivedValueReceptor[bool]

	// isWeakTipPoolMember is true if the block is part of the weak tip pool and not orphaned.
	isWeakTipPoolMember *agential.DerivedValueReceptor[bool]

	// isStronglyConnectedToTips is true if the block is either strongly referenced by others tips or is itself a strong
	// tip pool member.
	isStronglyConnectedToTips *agential.DerivedValueReceptor[bool]

	// isConnectedToTips is true if the block is either referenced by others tips or is itself a weak or strong tip pool
	// member.
	isConnectedToTips *agential.DerivedValueReceptor[bool]

	// isStronglyReferencedByTips is true if the block has at least one strong child that is strongly connected
	// to the tips.
	isStronglyReferencedByTips *agential.DerivedValueReceptor[bool]

	// isWeaklyReferencedByTips is true if the block has at least one weak child that is connected to the tips.
	isWeaklyReferencedByTips *agential.DerivedValueReceptor[bool]

	// isReferencedByTips is true if the block is strongly referenced by other tips or has at least one weak child
	// that is connected to the tips.
	isReferencedByTips *agential.DerivedValueReceptor[bool]

	// isStrongTip is true if the block is a strong tip pool member and is not strongly referenced by other tips.
	isStrongTip *agential.DerivedValueReceptor[bool]

	// isWeakTip is true if the block is a weak tip pool member and is not referenced by other tips.
	isWeakTip *agential.DerivedValueReceptor[bool]

	// isMarkedOrphaned is true if the liveness threshold has been reached and the block was not accepted.
	isMarkedOrphaned *agential.DerivedValueReceptor[bool]

	// isOrphaned is true if the block is either strongly or weakly orphaned.
	isOrphaned *agential.DerivedValueReceptor[bool]

	// anyStrongParentStronglyOrphaned is true if the block has at least one orphaned parent.
	anyStrongParentStronglyOrphaned *agential.DerivedValueReceptor[bool]

	// anyWeakParentWeaklyOrphaned is true if the block has at least one weak parent that is weakly orphaned.
	anyWeakParentWeaklyOrphaned *agential.DerivedValueReceptor[bool]

	// isEvicted is triggered when the block is removed from the TipManager.
	isEvicted *agential.ValueReceptor[bool]

	// isStronglyOrphaned is true if the block is either marked as orphaned, any of its strong parents is strongly
	// orphaned or any of its weak parents is weakly orphaned.
	isStronglyOrphaned *agential.DerivedValueReceptor[bool]

	// isWeaklyOrphaned is true if the block is either marked as orphaned or has at least one weakly orphaned weak
	// parent.
	isWeaklyOrphaned *agential.DerivedValueReceptor[bool]

	constructed *agential.ValueReceptor[bool]
}

// NewBlockMetadata creates a new TipMetadata instance.
func NewBlockMetadata(block *blocks.Block) *TipMetadata {
	t := &TipMetadata{
		block:                           block,
		tipPool:                         agential.NewValueReceptor[tipmanager.TipPool](tipmanager.TipPool.Max),
		constructed:                     agential.NewValueReceptor[bool](trapDoor[bool]),
		isLivenessThresholdReached:      agential.NewValueReceptor[bool](trapDoor[bool]),
		isEvicted:                       agential.NewValueReceptor[bool](trapDoor[bool]),
		stronglyConnectedStrongChildren: agential.NewCounter[bool](),
		connectedWeakChildren:           agential.NewCounter[bool](),
		stronglyOrphanedStrongParents:   agential.NewCounter[bool](),
		weaklyOrphanedWeakParents:       agential.NewCounter[bool](),
	}

	t.isStrongTipPoolMember = agential.DeriveValueFrom3Inputs(t, func(tipPool tipmanager.TipPool, isOrphaned bool, isEvicted bool) bool {
		return tipPool == tipmanager.StrongTipPool && !isOrphaned && !isEvicted
	}, &t.tipPool, &t.isOrphaned, &t.isEvicted)

	t.isWeakTipPoolMember = agential.DeriveValueFrom3Inputs(t, func(tipPool tipmanager.TipPool, isOrphaned bool, isEvicted bool) bool {
		return tipPool == tipmanager.WeakTipPool && !isOrphaned && !isEvicted
	}, &t.tipPool, &t.isOrphaned, &t.isEvicted)

	t.isStronglyConnectedToTips = agential.DeriveValueReceptorFrom2Inputs(t, func(isStrongTipPoolMember bool, isStronglyReferencedByTips bool) bool {
		return isStrongTipPoolMember || isStronglyReferencedByTips
	}, &t.isStrongTipPoolMember, &t.isStronglyReferencedByTips)

	t.isConnectedToTips = agential.DeriveValueFrom3Inputs(t, func(isReferencedByTips bool, isStrongTipPoolMember bool, isWeakTipPoolMember bool) bool {
		return isReferencedByTips || isStrongTipPoolMember || isWeakTipPoolMember
	}, &t.isReferencedByTips, &t.isStrongTipPoolMember, &t.isWeakTipPoolMember)

	t.isStronglyReferencedByTips = agential.DeriveValueReceptorFrom1Input[bool, int](t, func(stronglyConnectedStrongChildren int) bool {
		return stronglyConnectedStrongChildren > 0
	}, &t.stronglyConnectedStrongChildren)

	t.isWeaklyReferencedByTips = agential.DeriveValueReceptorFrom1Input[bool, int](t, func(connectedWeakChildren int) bool {
		return connectedWeakChildren > 0
	}, &t.connectedWeakChildren)

	t.isReferencedByTips = agential.DeriveValueReceptorFrom2Inputs[bool, bool, bool](t, func(isWeaklyReferencedByTips bool, isStronglyReferencedByTips bool) bool {
		return isWeaklyReferencedByTips || isStronglyReferencedByTips
	}, &t.isWeaklyReferencedByTips, &t.isStronglyReferencedByTips)

	t.isStrongTip = agential.DeriveValueReceptorFrom2Inputs[bool, bool, bool](t, func(isStrongTipPoolMember bool, isStronglyReferencedByOtherTips bool) bool {
		return isStrongTipPoolMember && !isStronglyReferencedByOtherTips
	}, &t.isStrongTipPoolMember, &t.isStronglyReferencedByTips)

	t.isWeakTip = agential.DeriveValueReceptorFrom2Inputs[bool, bool, bool](t, func(isWeakTipPoolMember bool, isReferencedByTips bool) bool {
		return isWeakTipPoolMember && !isReferencedByTips
	}, &t.isWeakTipPoolMember, &t.isReferencedByTips)

	t.isOrphaned = agential.DeriveValueReceptorFrom2Inputs[bool, bool, bool](t, func(isStronglyOrphaned bool, isWeaklyOrphaned bool) bool {
		return isStronglyOrphaned || isWeaklyOrphaned
	}, &t.isStronglyOrphaned, &t.isWeaklyOrphaned)

	t.anyStrongParentStronglyOrphaned = agential.DeriveValueReceptorFrom1Input[bool, int](t, func(stronglyOrphanedStrongParents int) bool {
		return stronglyOrphanedStrongParents > 0
	}, &t.stronglyOrphanedStrongParents)

	t.anyWeakParentWeaklyOrphaned = agential.DeriveValueReceptorFrom1Input[bool, int](t, func(weaklyOrphanedWeakParents int) bool {
		return weaklyOrphanedWeakParents > 0
	}, &t.weaklyOrphanedWeakParents)

	t.isStronglyOrphaned = agential.DeriveValueFrom3Inputs[bool, bool, bool, bool](t, func(isMarkedOrphaned, anyStrongParentStronglyOrphaned, anyWeakParentWeaklyOrphaned bool) bool {
		return isMarkedOrphaned || anyStrongParentStronglyOrphaned || anyWeakParentWeaklyOrphaned
	}, &t.isMarkedOrphaned, &t.anyStrongParentStronglyOrphaned, &t.anyWeakParentWeaklyOrphaned)

	t.isWeaklyOrphaned = agential.DeriveValueReceptorFrom2Inputs[bool, bool, bool](t, func(isMarkedOrphaned, anyWeakParentWeaklyOrphaned bool) bool {
		return isMarkedOrphaned || anyWeakParentWeaklyOrphaned
	}, &t.isMarkedOrphaned, &t.anyWeakParentWeaklyOrphaned)

	t.isMarkedOrphaned = agential.DeriveValueReceptorFrom1Input[bool, bool](t, func(isLivenessThresholdReached bool) bool {
		return isLivenessThresholdReached /* TODO: && !accepted */
	}, &t.isLivenessThresholdReached)

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

// TipPool exposes a ValueReceptor that stores the current TipPool of the block.
func (t *TipMetadata) TipPool() *agential.ValueReceptor[tipmanager.TipPool] {
	return t.tipPool
}

// IsLivenessThresholdReached exposes a ValueReceptor that stores if the liveness threshold was reached.
func (t *TipMetadata) IsLivenessThresholdReached() *agential.ValueReceptor[bool] {
	return t.isLivenessThresholdReached
}

// IsStrongTip returns a Value that indicates if the block is a strong tip.
func (t *TipMetadata) IsStrongTip() agential.Value[bool] {
	return t.isStrongTip
}

// IsWeakTip returns a Value that indicates if the block is a weak tip.
func (t *TipMetadata) IsWeakTip() agential.Value[bool] {
	return t.isWeakTip
}

// IsOrphaned returns a Value that indicates if the block was orphaned.
func (t *TipMetadata) IsOrphaned() agential.Value[bool] {
	return t.isOrphaned
}

// IsEvicted returns true if the block was evicted from the TipManager.
func (t *TipMetadata) IsEvicted() agential.Value[bool] {
	return t.isEvicted
}

// Constructed returns a value that indicates if the TipMetadata has initialized all its properties.
func (t *TipMetadata) Constructed() agential.Value[bool] {
	return t.constructed
}

// connectStrongParent sets up the parent and children related properties for a strong parent.
func (t *TipMetadata) connectStrongParent(strongParent *TipMetadata) {
	t.stronglyOrphanedStrongParents.Count(strongParent.isStronglyOrphaned)

	// unsubscribe when the parent is evicted, since we otherwise continue to hold a reference to it.
	unsubscribe := strongParent.stronglyConnectedStrongChildren.Count(t.isStronglyConnectedToTips)
	strongParent.isEvicted.OnUpdate(func(_, _ bool) { unsubscribe() })
}

// connectWeakParent sets up the parent and children related properties for a weak parent.
func (t *TipMetadata) connectWeakParent(weakParent *TipMetadata) {
	t.weaklyOrphanedWeakParents.Count(weakParent.isWeaklyOrphaned)

	// unsubscribe when the parent is evicted, since we otherwise continue to hold a reference to it.
	unsubscribe := weakParent.connectedWeakChildren.Count(t.isConnectedToTips)
	weakParent.isEvicted.OnUpdate(func(_, _ bool) { unsubscribe() })
}

// code contract (make sure the type implements all required methods).
var _ tipmanager.TipMetadata = new(TipMetadata)

func trapDoor[T comparable](prevValue, newValue T) T {
	var emptyValue T
	if newValue == emptyValue {
		return prevValue
	}

	return newValue
}
