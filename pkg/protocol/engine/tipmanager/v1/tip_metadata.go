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
	tipPool agential.ValueReceptor[tipmanager.TipPool]

	// isLivenessThresholdReached is true if the block has reached the liveness threshold.
	isLivenessThresholdReached agential.ValueReceptor[bool]

	// stronglyConnectedStrongChildren holds the number of strong children that are strongly connected to the tips.
	stronglyConnectedStrongChildren *agential.ThresholdTransformer[bool]

	// connectedWeakChildren holds the number of weak children that are connected to the tips.
	connectedWeakChildren *agential.ThresholdTransformer[bool]

	// stronglyOrphanedStrongParents holds the number of strong parents that are strongly orphaned.
	stronglyOrphanedStrongParents *agential.ThresholdTransformer[bool]

	// weaklyOrphanedWeakParents holds the number of weak parents that are weakly orphaned.
	weaklyOrphanedWeakParents *agential.ThresholdTransformer[bool]

	// isStrongTipPoolMember is true if the block is part of the strong tip pool and not orphaned.
	isStrongTipPoolMember agential.ValueReceptor[bool]

	// isWeakTipPoolMember is true if the block is part of the weak tip pool and not orphaned.
	isWeakTipPoolMember agential.ValueReceptor[bool]

	// isStronglyConnectedToTips is true if the block is either strongly referenced by others tips or is itself a strong
	// tip pool member.
	isStronglyConnectedToTips agential.ValueReceptor[bool]

	// isConnectedToTips is true if the block is either referenced by others tips or is itself a weak or strong tip pool
	// member.
	isConnectedToTips agential.ValueReceptor[bool]

	// isStronglyReferencedByStronglyConnectedTips is true if the block has at least one strong child that is strongly connected
	// to the tips.
	isStronglyReferencedByStronglyConnectedTips agential.ValueReceptor[bool]

	// isReferencedByOtherTips is true if the block is strongly referenced by other tips or has at least one weak child
	// that is connected to the tips.
	isReferencedByOtherTips agential.ValueReceptor[bool]

	// isStrongTip is true if the block is a strong tip pool member and is not strongly referenced by other tips.
	isStrongTip agential.ValueReceptor[bool]

	// isWeakTip is true if the block is a weak tip pool member and is not referenced by other tips.
	isWeakTip agential.ValueReceptor[bool]

	// isMarkedOrphaned is true if the liveness threshold has been reached and the block was not accepted.
	isMarkedOrphaned agential.ValueReceptor[bool]

	// isOrphaned is true if the block is either strongly or weakly orphaned.
	isOrphaned agential.ValueReceptor[bool]

	// anyStrongParentStronglyOrphaned is true if the block has at least one orphaned parent.
	anyStrongParentStronglyOrphaned agential.ValueReceptor[bool]

	// anyWeakParentWeaklyOrphaned is true if the block has at least one weak parent that is weakly orphaned.
	anyWeakParentWeaklyOrphaned agential.ValueReceptor[bool]

	// isEvicted is triggered when the block is removed from the TipManager.
	isEvicted agential.ValueReceptor[bool]

	// isStronglyOrphaned is true if the block is either marked as orphaned, any of its strong parents is strongly
	// orphaned or any of its weak parents is weakly orphaned.
	isStronglyOrphaned agential.ValueReceptor[bool]

	// isWeaklyOrphaned is true if the block is either marked as orphaned or has at least one weakly orphaned weak
	// parent.
	isWeaklyOrphaned agential.ValueReceptor[bool]

	constructed agential.ValueReceptor[bool]
}

// NewBlockMetadata creates a new TipMetadata instance.
func NewBlockMetadata(block *blocks.Block) *TipMetadata {
	t := &TipMetadata{
		block:                      block,
		tipPool:                    agential.NewValueReceptor[tipmanager.TipPool](tipmanager.TipPool.Max),
		constructed:                agential.NewValueReceptor[bool](trapDoor[bool]),
		isLivenessThresholdReached: agential.NewValueReceptor[bool](trapDoor[bool]),
		isEvicted:                  agential.NewValueReceptor[bool](trapDoor[bool]),
	}

	t.isStrongTipPoolMember = agential.NewDerivedValueReceptorFrom3Inputs(t, func(tipPool tipmanager.TipPool, isOrphaned bool, isEvicted bool) bool {
		return tipPool == tipmanager.StrongTipPool && !isOrphaned && !isEvicted
	}, &t.tipPool, &t.isOrphaned, &t.isEvicted)

	t.isWeakTipPoolMember = agential.NewDerivedValueReceptorFrom3Inputs(t, func(tipPool tipmanager.TipPool, isOrphaned bool, isEvicted bool) bool {
		return tipPool == tipmanager.WeakTipPool && !isOrphaned && !isEvicted
	}, &t.tipPool, &t.isOrphaned, &t.isEvicted)

	t.isStronglyConnectedToTips = agential.NewDerivedValueReceptorFrom2Inputs(t, func(isStrongTipPoolMember bool, isStronglyReferencedByStronglyConnectedTips bool) bool {
		return isStrongTipPoolMember || isStronglyReferencedByStronglyConnectedTips
	}, &t.isStrongTipPoolMember, &t.isStronglyReferencedByStronglyConnectedTips)

	t.isConnectedToTips = agential.NewDerivedValueReceptorFrom3Inputs(t, func(isReferencedByOtherTips bool, isStrongTipPoolMember bool, isWeakTipPoolMember bool) bool {
		return isReferencedByOtherTips || isStrongTipPoolMember || isWeakTipPoolMember
	}, &t.isReferencedByOtherTips, &t.isStrongTipPoolMember, &t.isWeakTipPoolMember)

	t.isStronglyReferencedByStronglyConnectedTips = agential.NewValueReceptorFromInput[bool, int](t, func(stronglyConnectedStrongChildren int) bool {
		return stronglyConnectedStrongChildren > 0
	}, &t.stronglyConnectedStrongChildren)

	t.isReferencedByOtherTips = agential.NewDerivedValueReceptorFrom2Inputs[bool, int, bool](t, func(connectedWeakChildren int, isStronglyReferencedByStronglyConnectedTips bool) bool {
		return connectedWeakChildren > 0 || isStronglyReferencedByStronglyConnectedTips
	}, &t.connectedWeakChildren, &t.isStronglyReferencedByStronglyConnectedTips)

	t.isStrongTip = agential.NewDerivedValueReceptorFrom2Inputs[bool, bool, bool](t, func(isStrongTipPoolMember bool, isStronglyReferencedByOtherTips bool) bool {
		return isStrongTipPoolMember && !isStronglyReferencedByOtherTips
	}, &t.isStrongTipPoolMember, &t.isStronglyReferencedByStronglyConnectedTips)

	t.isWeakTip = agential.NewDerivedValueReceptorFrom2Inputs[bool, bool, bool](t, func(isWeakTipPoolMember bool, isReferencedByOtherTips bool) bool {
		return isWeakTipPoolMember && !isReferencedByOtherTips
	}, &t.isWeakTipPoolMember, &t.isReferencedByOtherTips)

	t.isOrphaned = agential.NewDerivedValueReceptorFrom2Inputs[bool, bool, bool](t, func(isStronglyOrphaned bool, isWeaklyOrphaned bool) bool {
		return isStronglyOrphaned || isWeaklyOrphaned
	}, &t.isStronglyOrphaned, &t.isWeaklyOrphaned)

	t.anyStrongParentStronglyOrphaned = agential.NewValueReceptorFromInput[bool, int](t, func(stronglyOrphanedStrongParents int) bool {
		return stronglyOrphanedStrongParents > 0
	}, &t.stronglyOrphanedStrongParents)

	t.anyWeakParentWeaklyOrphaned = agential.NewValueReceptorFromInput[bool, int](t, func(weaklyOrphanedWeakParents int) bool {
		return weaklyOrphanedWeakParents > 0
	}, &t.weaklyOrphanedWeakParents)

	t.isStronglyOrphaned = agential.NewDerivedValueReceptorFrom3Inputs[bool, bool, bool, bool](t, func(isMarkedOrphaned, anyStrongParentStronglyOrphaned, anyWeakParentWeaklyOrphaned bool) bool {
		return isMarkedOrphaned || anyStrongParentStronglyOrphaned || anyWeakParentWeaklyOrphaned
	}, &t.isMarkedOrphaned, &t.anyStrongParentStronglyOrphaned, &t.anyWeakParentWeaklyOrphaned)

	t.isWeaklyOrphaned = agential.NewDerivedValueReceptorFrom2Inputs[bool, bool, bool](t, func(isMarkedOrphaned, anyWeakParentWeaklyOrphaned bool) bool {
		return isMarkedOrphaned || anyWeakParentWeaklyOrphaned
	}, &t.isMarkedOrphaned, &t.anyWeakParentWeaklyOrphaned)

	t.isMarkedOrphaned = agential.NewValueReceptorFromInput[bool, bool](t, func(isLivenessThresholdReached bool) bool {
		return isLivenessThresholdReached /* TODO: && !accepted */
	}, &t.isLivenessThresholdReached)

	t.stronglyConnectedStrongChildren = agential.NewThresholdTransformer[bool]()

	t.connectedWeakChildren = agential.NewThresholdTransformer[bool]()

	t.stronglyOrphanedStrongParents = agential.NewThresholdTransformer[bool]()

	t.weaklyOrphanedWeakParents = agential.NewThresholdTransformer[bool]()

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
func (t *TipMetadata) TipPool() agential.ValueReceptor[tipmanager.TipPool] {
	return t.tipPool
}

// LivenessThresholdReached is a receptor that holds a boolean value indicating if the liveness threshold is reached.
func (t *TipMetadata) LivenessThresholdReached() agential.ValueReceptor[bool] {
	return t.isLivenessThresholdReached
}

// IsStrongTip returns true if the block is currently an unreferenced strong tip.
func (t *TipMetadata) IsStrongTip() agential.ReadOnlyValueReceptor[bool] {
	return t.isStrongTip
}

// IsWeakTip returns true if the block is an unreferenced weak tip.
func (t *TipMetadata) IsWeakTip() agential.ReadOnlyValueReceptor[bool] {
	return t.isWeakTip
}

// IsOrphaned returns true if the block is marked orphaned or if it has an orphaned strong parent.
func (t *TipMetadata) IsOrphaned() agential.ReadOnlyValueReceptor[bool] {
	return t.isOrphaned
}

func (t *TipMetadata) Evicted() agential.ReadOnlyValueReceptor[bool] {
	return t.isEvicted
}

func (t *TipMetadata) Constructed() agential.ReadOnlyValueReceptor[bool] {
	return t.constructed
}

// connectStrongParent sets up the parent and children related properties for a strong parent.
func (t *TipMetadata) connectStrongParent(strongParent *TipMetadata) {
	t.stronglyOrphanedStrongParents.Track(strongParent.isStronglyOrphaned)

	// unsubscribe when the parent is evicted, since we otherwise continue to hold a reference to it.
	unsubscribe := strongParent.stronglyConnectedStrongChildren.Track(t.isStronglyConnectedToTips)
	strongParent.isEvicted.OnUpdate(func(_, _ bool) { unsubscribe() })
}

// connectWeakParent sets up the parent and children related properties for a weak parent.
func (t *TipMetadata) connectWeakParent(weakParent *TipMetadata) {
	unsubscribe := lo.Batch(
		weakParent.connectedWeakChildren.Track(t.isConnectedToTips),

		t.weaklyOrphanedWeakParents.Track(weakParent.isWeaklyOrphaned),
	)

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
