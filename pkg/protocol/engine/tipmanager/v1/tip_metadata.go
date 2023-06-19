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

	// isStronglyReferencedByTips is a derived property that is true if the block has at least one strongly connected
	// child.
	isStronglyReferencedByTips *lpromise.Value[bool]

	// isWeaklyReferencedByTips is a derived property that is true if the block has at least one strongly or weakly connected
	// child.
	isWeaklyReferencedByTips *lpromise.Value[bool]

	// isStronglyConnectedToTips is a derived property that is true if the block is either strongly referenced by tips or
	// part of the strong TipPool.
	isStronglyConnectedToTips *lpromise.Value[bool]

	// weaklyConnectedToTips is a derived property that is true if the block is either part of the weak TipPool or has
	// at least one weakly connected child.
	weaklyConnectedToTips *lpromise.Value[bool]

	// isEligibleStrongTip is a derived property that is true if the block is part of the strong TipPool and is not
	// orphaned.
	isEligibleStrongTip *lpromise.Value[bool]

	// isEligibleWeakTip is a derived property that is true if the block is part of the weak TipPool and is not
	// orphaned.
	isEligibleWeakTip *lpromise.Value[bool]

	// isStrongTip is a derived property that is true if the block is part of the strong TipPool, and is not
	// isStronglyReferencedByTips.
	isStrongTip *lpromise.Value[bool]

	// isWeakTip is a derived property that is true if the block is part of the weak TipPool and isWeaklyReferencedByTips is
	// false.
	isWeakTip *lpromise.Value[bool]

	// orphanedStrongParents holds the number of strong parents that are orphaned.
	orphanedStrongParents *lpromise.Value[int]

	// markedOrphaned is a property that is true if the block was marked as orphaned.
	markedOrphaned *lpromise.Value[bool]

	// isOrphaned is a derived property that is true if the block is either marked as orphaned or has at least one
	// orphaned strong parent.
	isOrphaned *lpromise.Value[bool]

	// evicted is triggered when the block is removed from the TipManager.
	evicted *promise.Event
}

// NewBlockMetadata creates a new TipMetadata instance.
func NewBlockMetadata(block *blocks.Block) *TipMetadata {
	return (&TipMetadata{
		block:                      block,
		tipPool:                    lpromise.NewValue[tipmanager.TipPool](),
		stronglyConnectedChildren:  lpromise.NewValue[int](),
		weaklyConnectedChildren:    lpromise.NewValue[int](),
		isStronglyReferencedByTips: lpromise.NewValue[bool](),
		isWeaklyReferencedByTips:   lpromise.NewValue[bool](),
		isStronglyConnectedToTips:  lpromise.NewValue[bool](),
		weaklyConnectedToTips:      lpromise.NewValue[bool](),
		isEligibleStrongTip:        lpromise.NewValue[bool](),
		isEligibleWeakTip:          lpromise.NewValue[bool](),
		isStrongTip:                lpromise.NewValue[bool](),
		isWeakTip:                  lpromise.NewValue[bool](),
		orphanedStrongParents:      lpromise.NewValue[int](),
		markedOrphaned:             lpromise.NewValue[bool](),
		isOrphaned:                 lpromise.NewValue[bool](),
		evicted:                    promise.NewEvent(),
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
	return t.evicted.WasTriggered()
}

// OnEvicted registers a callback that is triggered when the Block is removed from the TipManager.
func (t *TipMetadata) OnEvicted(handler func()) {
	t.evicted.OnTrigger(handler)
}

// setup sets up the behavior of the derived properties of the Block.
func (t *TipMetadata) setup() (self *TipMetadata) {
	t.setupIsEligibleStrongTip()
	t.setupIsEligibleWeakTip()
	t.setupStronglyConnectedToTips()
	t.setupIsOrphaned()
	t.setupIsStrongTip()
	t.setupIsWeakTip()
	t.setupIsWeaklyReferencedByTips()
	t.setupWeaklyConnectedToTips()
	t.setupIsStronglyReferencedByTips()

	return t
}

func (t *TipMetadata) setupIsEligibleStrongTip() {
	t.tipPool.OnUpdate(func(_, tipPool tipmanager.TipPool) {
		t.isEligibleStrongTip.Compute(func(_ bool) bool {
			return tipPool == tipmanager.StrongTipPool && !t.isOrphaned.Get()
		})
	})

	t.isOrphaned.OnUpdate(func(_, isOrphaned bool) {
		t.isEligibleStrongTip.Compute(func(_ bool) bool {
			return !isOrphaned && t.tipPool.Get() == tipmanager.StrongTipPool
		})
	})
}

func (t *TipMetadata) setupIsEligibleWeakTip() {
	t.isOrphaned.OnUpdate(func(_, isOrphaned bool) {
		t.isEligibleWeakTip.Compute(func(_ bool) bool {
			return !isOrphaned && t.tipPool.Get() == tipmanager.WeakTipPool
		})
	})

	t.tipPool.OnUpdate(func(_, tipPool tipmanager.TipPool) {
		t.isEligibleWeakTip.Compute(func(_ bool) bool {
			return tipPool == tipmanager.WeakTipPool && !t.isOrphaned.Get()
		})
	})
}

func (t *TipMetadata) setupStronglyConnectedToTips() {
	t.isStronglyReferencedByTips.OnUpdate(func(_, stronglyReferencedByTips bool) {
		t.isStronglyConnectedToTips.Compute(func(_ bool) bool {
			return stronglyReferencedByTips || t.isEligibleStrongTip.Get()
		})
	})

	t.isEligibleStrongTip.OnUpdate(func(_, isEligibleStrongTip bool) {
		t.isStronglyConnectedToTips.Compute(func(_ bool) bool {
			return isEligibleStrongTip || t.isStronglyReferencedByTips.Get()
		})
	})
}

func (t *TipMetadata) setupIsOrphaned() {
	t.markedOrphaned.OnUpdate(func(_, markedOrphaned bool) {
		t.isOrphaned.Compute(func(_ bool) bool {
			return markedOrphaned || t.orphanedStrongParents.Get() > 0
		})
	})

	t.orphanedStrongParents.OnUpdate(func(_, orphanedStrongParents int) {
		t.isOrphaned.Compute(func(_ bool) bool {
			return orphanedStrongParents > 0 || t.markedOrphaned.Get()
		})
	})
}

func (t *TipMetadata) setupIsStrongTip() {
	t.isStronglyReferencedByTips.OnUpdate(func(_, isStronglyReferencedByTips bool) {
		t.isStrongTip.Compute(func(_ bool) bool {
			return !isStronglyReferencedByTips && t.isEligibleStrongTip.Get()
		})
	})

	t.isEligibleStrongTip.OnUpdate(func(_, isEligibleStrongTip bool) {
		t.isStrongTip.Compute(func(_ bool) bool {
			return isEligibleStrongTip && !t.isStronglyReferencedByTips.Get()
		})
	})
}

func (t *TipMetadata) setupIsWeakTip() {
	t.isWeaklyReferencedByTips.OnUpdate(func(_, isReferencedByTips bool) {
		t.isWeakTip.Compute(func(_ bool) bool {
			return !isReferencedByTips && t.isEligibleWeakTip.Get()
		})
	})

	t.isEligibleWeakTip.OnUpdate(func(_, isEligibleWeakTip bool) {
		t.isWeakTip.Compute(func(_ bool) bool {
			return isEligibleWeakTip && !t.isWeaklyReferencedByTips.Get()
		})
	})
}

func (t *TipMetadata) setupWeaklyConnectedToTips() {
	t.isWeaklyReferencedByTips.OnUpdate(func(_, isWeaklyReferencedByTips bool) {
		t.weaklyConnectedToTips.Compute(func(_ bool) bool {
			return isWeaklyReferencedByTips || t.isEligibleWeakTip.Get() || t.isStronglyConnectedToTips.Get()
		})
	})

	t.isEligibleWeakTip.OnUpdate(func(_, isEligibleWeakTip bool) {
		t.weaklyConnectedToTips.Compute(func(_ bool) bool {
			return isEligibleWeakTip || t.isWeaklyReferencedByTips.Get() || t.isStronglyConnectedToTips.Get()
		})
	})

	t.isStronglyConnectedToTips.OnUpdate(func(_, stronglyConnectedToTips bool) {
		t.weaklyConnectedToTips.Compute(func(_ bool) bool {
			return stronglyConnectedToTips || t.isEligibleWeakTip.Get() || t.isWeaklyReferencedByTips.Get()
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
			return weaklyConnectedChildren > 0
		})
	})
}

func (t *TipMetadata) registerStrongParent(strongParent *TipMetadata) {
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

func (t *TipMetadata) registerWeakParent(weakParent *TipMetadata) {
	// unsubscribe on eviction of the parent (prevent memory leaks).
	weakParent.OnEvicted(
		t.weaklyConnectedToTips.OnUpdate(func(_, isConnected bool) {
			weakParent.weaklyConnectedChildren.Compute(lo.Cond(isConnected, increase, decrease))
		}),
	)
}

// code contract (make sure the type implements all required methods).
var _ tipmanager.TipMetadata = new(TipMetadata)
