package tipmanagerv1

import (
	"github.com/iotaledger/iota-core/pkg/core/reactive"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TipMetadata represents the metadata for a block in the TipManager.
type TipMetadata struct {
	// block holds the block that the metadata belongs to.
	block *blocks.Block

	// tipPool holds the tip pool the block is currently assigned to.
	tipPool reactive.Variable[tipmanager.TipPool]

	// isLivenessThresholdReached is true if the block has reached the liveness threshold.
	isLivenessThresholdReached reactive.Variable[bool]

	// isStrongTipPoolMember is true if the block is part of the strong tip pool and not orphaned.
	isStrongTipPoolMember reactive.Variable[bool]

	// isWeakTipPoolMember is true if the block is part of the weak tip pool and not orphaned.
	isWeakTipPoolMember reactive.Variable[bool]

	// isStronglyConnectedToTips is true if the block is either strongly referenced by others tips or is itself a strong
	// tip pool member.
	isStronglyConnectedToTips reactive.Variable[bool]

	// isConnectedToTips is true if the block is either referenced by others tips or is itself a weak or strong tip pool
	// member.
	isConnectedToTips reactive.Variable[bool]

	// isStronglyReferencedByTips is true if the block has at least one strong child that is strongly connected
	// to the tips.
	isStronglyReferencedByTips reactive.Variable[bool]

	// isWeaklyReferencedByTips is true if the block has at least one weak child that is connected to the tips.
	isWeaklyReferencedByTips reactive.Variable[bool]

	// isReferencedByTips is true if the block is strongly referenced by other tips or has at least one weak child
	// that is connected to the tips.
	isReferencedByTips reactive.Variable[bool]

	// isStrongTip is true if the block is a strong tip pool member and is not strongly referenced by other tips.
	isStrongTip reactive.Variable[bool]

	// isWeakTip is true if the block is a weak tip pool member and is not referenced by other tips.
	isWeakTip reactive.Variable[bool]

	// isMarkedOrphaned is true if the liveness threshold has been reached and the block was not accepted.
	isMarkedOrphaned reactive.Variable[bool]

	// isOrphaned is true if the block is either strongly or weakly orphaned.
	isOrphaned reactive.Variable[bool]

	// anyStrongParentStronglyOrphaned is true if the block has at least one orphaned parent.
	anyStrongParentStronglyOrphaned reactive.Variable[bool]

	// anyWeakParentWeaklyOrphaned is true if the block has at least one weak parent that is weakly orphaned.
	anyWeakParentWeaklyOrphaned reactive.Variable[bool]

	// isEvicted is triggered when the block is removed from the TipManager.
	isEvicted reactive.Variable[bool]

	// isStronglyOrphaned is true if the block is either marked as orphaned, any of its strong parents is strongly
	// orphaned or any of its weak parents is weakly orphaned.
	isStronglyOrphaned reactive.Variable[bool]

	// isWeaklyOrphaned is true if the block is either marked as orphaned or has at least one weakly orphaned weak
	// parent.
	isWeaklyOrphaned reactive.Variable[bool]

	// stronglyConnectedStrongChildren holds the number of strong children that are strongly connected to the tips.
	stronglyConnectedStrongChildren *reactive.Counter[bool]

	// connectedWeakChildren holds the number of weak children that are connected to the tips.
	connectedWeakChildren *reactive.Counter[bool]

	// stronglyOrphanedStrongParents holds the number of strong parents that are strongly orphaned.
	stronglyOrphanedStrongParents *reactive.Counter[bool]

	// weaklyOrphanedWeakParents holds the number of weak parents that are weakly orphaned.
	weaklyOrphanedWeakParents *reactive.Counter[bool]

	constructed reactive.Variable[bool]
}

// NewBlockMetadata creates a new TipMetadata instance.
func NewBlockMetadata(block *blocks.Block) *TipMetadata {
	t := &TipMetadata{
		block:                           block,
		tipPool:                         reactive.NewVariable[tipmanager.TipPool](tipmanager.TipPool.Max),
		constructed:                     reactive.NewVariable[bool](trapDoor[bool]),
		isLivenessThresholdReached:      reactive.NewVariable[bool](trapDoor[bool]),
		isEvicted:                       reactive.NewVariable[bool](trapDoor[bool]),
		stronglyConnectedStrongChildren: reactive.NewCounter[bool](),
		connectedWeakChildren:           reactive.NewCounter[bool](),
		stronglyOrphanedStrongParents:   reactive.NewCounter[bool](),
		weaklyOrphanedWeakParents:       reactive.NewCounter[bool](),
	}

	t.isStrongTipPoolMember = reactive.DeriveVariableFrom3Values(t, func(tipPool tipmanager.TipPool, isOrphaned bool, isEvicted bool) bool {
		return tipPool == tipmanager.StrongTipPool && !isOrphaned && !isEvicted
	}, &t.tipPool, &t.isOrphaned, &t.isEvicted)

	t.isWeakTipPoolMember = reactive.DeriveVariableFrom3Values(t, func(tipPool tipmanager.TipPool, isOrphaned bool, isEvicted bool) bool {
		return tipPool == tipmanager.WeakTipPool && !isOrphaned && !isEvicted
	}, &t.tipPool, &t.isOrphaned, &t.isEvicted)

	t.isStronglyConnectedToTips = reactive.DeriveVariableFrom2Values(t, func(isStrongTipPoolMember bool, isStronglyReferencedByTips bool) bool {
		return isStrongTipPoolMember || isStronglyReferencedByTips
	}, &t.isStrongTipPoolMember, &t.isStronglyReferencedByTips)

	t.isConnectedToTips = reactive.DeriveVariableFrom3Values(t, func(isReferencedByTips bool, isStrongTipPoolMember bool, isWeakTipPoolMember bool) bool {
		return isReferencedByTips || isStrongTipPoolMember || isWeakTipPoolMember
	}, &t.isReferencedByTips, &t.isStrongTipPoolMember, &t.isWeakTipPoolMember)

	t.isStronglyReferencedByTips = reactive.DeriveVariableFromValue[bool, int](t, func(stronglyConnectedStrongChildren int) bool {
		return stronglyConnectedStrongChildren > 0
	}, &t.stronglyConnectedStrongChildren)

	t.isWeaklyReferencedByTips = reactive.DeriveVariableFromValue[bool, int](t, func(connectedWeakChildren int) bool {
		return connectedWeakChildren > 0
	}, &t.connectedWeakChildren)

	t.isReferencedByTips = reactive.DeriveVariableFrom2Values[bool, bool, bool](t, func(isWeaklyReferencedByTips bool, isStronglyReferencedByTips bool) bool {
		return isWeaklyReferencedByTips || isStronglyReferencedByTips
	}, &t.isWeaklyReferencedByTips, &t.isStronglyReferencedByTips)

	t.isStrongTip = reactive.DeriveVariableFrom2Values[bool, bool, bool](t, func(isStrongTipPoolMember bool, isStronglyReferencedByTips bool) bool {
		return isStrongTipPoolMember && !isStronglyReferencedByTips
	}, &t.isStrongTipPoolMember, &t.isStronglyReferencedByTips)

	t.isWeakTip = reactive.DeriveVariableFrom2Values[bool, bool, bool](t, func(isWeakTipPoolMember bool, isReferencedByTips bool) bool {
		return isWeakTipPoolMember && !isReferencedByTips
	}, &t.isWeakTipPoolMember, &t.isReferencedByTips)

	t.isOrphaned = reactive.DeriveVariableFrom2Values[bool, bool, bool](t, func(isStronglyOrphaned bool, isWeaklyOrphaned bool) bool {
		return isStronglyOrphaned || isWeaklyOrphaned
	}, &t.isStronglyOrphaned, &t.isWeaklyOrphaned)

	t.anyStrongParentStronglyOrphaned = reactive.DeriveVariableFromValue[bool, int](t, func(stronglyOrphanedStrongParents int) bool {
		return stronglyOrphanedStrongParents > 0
	}, &t.stronglyOrphanedStrongParents)

	t.anyWeakParentWeaklyOrphaned = reactive.DeriveVariableFromValue[bool, int](t, func(weaklyOrphanedWeakParents int) bool {
		return weaklyOrphanedWeakParents > 0
	}, &t.weaklyOrphanedWeakParents)

	t.isStronglyOrphaned = reactive.DeriveVariableFrom3Values[bool, bool, bool, bool](t, func(isMarkedOrphaned, anyStrongParentStronglyOrphaned, anyWeakParentWeaklyOrphaned bool) bool {
		return isMarkedOrphaned || anyStrongParentStronglyOrphaned || anyWeakParentWeaklyOrphaned
	}, &t.isMarkedOrphaned, &t.anyStrongParentStronglyOrphaned, &t.anyWeakParentWeaklyOrphaned)

	t.isWeaklyOrphaned = reactive.DeriveVariableFrom2Values[bool, bool, bool](t, func(isMarkedOrphaned, anyWeakParentWeaklyOrphaned bool) bool {
		return isMarkedOrphaned || anyWeakParentWeaklyOrphaned
	}, &t.isMarkedOrphaned, &t.anyWeakParentWeaklyOrphaned)

	t.isMarkedOrphaned = reactive.DeriveVariableFromValue[bool, bool](t, func(isLivenessThresholdReached bool) bool {
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

// TipPool exposes a variable that stores the current TipPool of the block.
func (t *TipMetadata) TipPool() reactive.Variable[tipmanager.TipPool] {
	return t.tipPool
}

// IsLivenessThresholdReached exposes a variable that stores if the liveness threshold was reached.
func (t *TipMetadata) IsLivenessThresholdReached() reactive.Variable[bool] {
	return t.isLivenessThresholdReached
}

// IsStrongTip returns a Value that indicates if the block is a strong tip.
func (t *TipMetadata) IsStrongTip() reactive.Value[bool] {
	return t.isStrongTip
}

// IsWeakTip returns a Value that indicates if the block is a weak tip.
func (t *TipMetadata) IsWeakTip() reactive.Value[bool] {
	return t.isWeakTip
}

// IsOrphaned returns a Value that indicates if the block was orphaned.
func (t *TipMetadata) IsOrphaned() reactive.Value[bool] {
	return t.isOrphaned
}

// IsEvicted returns true if the block was evicted from the TipManager.
func (t *TipMetadata) IsEvicted() reactive.Value[bool] {
	return t.isEvicted
}

// Constructed returns a value that indicates if the TipMetadata has initialized all its properties.
func (t *TipMetadata) Constructed() reactive.Value[bool] {
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
