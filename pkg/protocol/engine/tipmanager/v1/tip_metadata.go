package tipmanagerv1

import (
	"fmt"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/log"
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

	// livenessThresholdReached is an event that is triggered when the liveness threshold is reached.
	livenessThresholdReached reactive.Event

	// evicted is an event that is triggered when the block is evicted.
	evicted reactive.Event

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

	// isLatestValidationBlock is true if the block is the latest block of a validator.
	isLatestValidationBlock reactive.Variable[bool]

	// referencesLatestValidationBlock is true if the block is the latest validator block or has parents that reference
	// the latest validator block.
	referencesLatestValidationBlock reactive.Variable[bool]

	// isStrongTip is true if the block is a strong tip pool member and is not strongly referenced by other tips.
	isStrongTip reactive.Variable[bool]

	// isWeakTip is true if the block is a weak tip pool member and is not referenced by other tips.
	isWeakTip reactive.Variable[bool]

	// isValidationTip is true if the block is a strong tip and references the latest validator block.
	isValidationTip reactive.Variable[bool]

	// isMarkedOrphaned is true if the liveness threshold has been reached and the block was not accepted.
	isMarkedOrphaned reactive.Variable[bool]

	// isOrphaned is true if the block is either strongly or weakly orphaned.
	isOrphaned reactive.Variable[bool]

	// anyStrongParentStronglyOrphaned is true if the block has at least one strong parent that is strongly orphaned.
	anyStrongParentStronglyOrphaned reactive.Variable[bool]

	// anyWeakParentWeaklyOrphaned is true if the block has at least one weak parent that is weakly orphaned.
	anyWeakParentWeaklyOrphaned reactive.Variable[bool]

	// isStronglyOrphaned is true if the block is either marked as orphaned, any of its strong parents is strongly
	// orphaned or any of its weak parents is weakly orphaned.
	isStronglyOrphaned reactive.Variable[bool]

	// isWeaklyOrphaned is true if the block is either marked as orphaned or has at least one weakly orphaned weak
	// parent.
	isWeaklyOrphaned reactive.Variable[bool]

	// stronglyConnectedStrongChildren holds the number of strong children that are strongly connected to the tips.
	stronglyConnectedStrongChildren reactive.Counter[bool]

	// connectedWeakChildren holds the number of weak children that are connected to the tips.
	connectedWeakChildren reactive.Counter[bool]

	// stronglyOrphanedStrongParents holds the number of strong parents that are strongly orphaned.
	stronglyOrphanedStrongParents reactive.Counter[bool]

	// weaklyOrphanedWeakParents holds the number of weak parents that are weakly orphaned.
	weaklyOrphanedWeakParents reactive.Counter[bool]

	// parentsReferencingLatestValidationBlock holds the number of parents that reference the latest validator block.
	parentsReferencingLatestValidationBlock reactive.Counter[bool]

	log.Logger
}

// NewTipMetadata creates a new TipMetadata instance.
func NewTipMetadata(logger log.Logger, block *blocks.Block) *TipMetadata {
	t := &TipMetadata{
		Logger: logger,

		block:                                   block,
		tipPool:                                 reactive.NewVariable[tipmanager.TipPool](tipmanager.TipPool.Max),
		livenessThresholdReached:                reactive.NewEvent(),
		evicted:                                 reactive.NewEvent(),
		isStrongTipPoolMember:                   reactive.NewVariable[bool](),
		isWeakTipPoolMember:                     reactive.NewVariable[bool](),
		isStronglyConnectedToTips:               reactive.NewVariable[bool](),
		isConnectedToTips:                       reactive.NewVariable[bool](),
		isStronglyReferencedByTips:              reactive.NewVariable[bool](),
		isWeaklyReferencedByTips:                reactive.NewVariable[bool](),
		isReferencedByTips:                      reactive.NewVariable[bool](),
		isLatestValidationBlock:                 reactive.NewVariable[bool](),
		referencesLatestValidationBlock:         reactive.NewVariable[bool](),
		isStrongTip:                             reactive.NewVariable[bool](),
		isWeakTip:                               reactive.NewVariable[bool](),
		isValidationTip:                         reactive.NewVariable[bool](),
		isMarkedOrphaned:                        reactive.NewVariable[bool](),
		isOrphaned:                              reactive.NewVariable[bool](),
		anyStrongParentStronglyOrphaned:         reactive.NewVariable[bool](),
		anyWeakParentWeaklyOrphaned:             reactive.NewVariable[bool](),
		isStronglyOrphaned:                      reactive.NewVariable[bool](),
		isWeaklyOrphaned:                        reactive.NewVariable[bool](),
		stronglyConnectedStrongChildren:         reactive.NewCounter[bool](),
		connectedWeakChildren:                   reactive.NewCounter[bool](),
		stronglyOrphanedStrongParents:           reactive.NewCounter[bool](),
		weaklyOrphanedWeakParents:               reactive.NewCounter[bool](),
		parentsReferencingLatestValidationBlock: reactive.NewCounter[bool](),
	}

	t.initLogging()
	t.initBehavior()

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

// LivenessThresholdReached exposes an event that is triggered when the liveness threshold is reached.
func (t *TipMetadata) LivenessThresholdReached() reactive.Event {
	return t.livenessThresholdReached
}

// IsStrongTip returns a ReadableVariable that indicates if the block is a strong tip.
func (t *TipMetadata) IsStrongTip() reactive.ReadableVariable[bool] {
	return t.isStrongTip
}

// IsWeakTip returns a ReadableVariable that indicates if the block is a weak tip.
func (t *TipMetadata) IsWeakTip() reactive.ReadableVariable[bool] {
	return t.isWeakTip
}

// IsOrphaned returns a ReadableVariable that indicates if the block was orphaned.
func (t *TipMetadata) IsOrphaned() reactive.ReadableVariable[bool] {
	return t.isOrphaned
}

// Evicted exposes an event that is triggered when the block is evicted.
func (t *TipMetadata) Evicted() reactive.Event {
	return t.evicted
}

// String returns a human-readable representation of the TipMetadata.
func (t *TipMetadata) String() string {
	return fmt.Sprintf(
		"TipMetadata: [\n"+
			"Block: %s\n"+
			"TipPool: %d\n"+
			"IsStrongTipPoolMember: %v\n"+
			"IsWeakTipPoolMember: %v\n"+
			"IsStronglyConnectedToTips: %v\n"+
			"IsConnectedToTips: %v\n"+
			"IsStronglyReferencedByTips: %v\n"+
			"IsWeaklyReferencedByTips: %v\n"+
			"IsReferencedByTips: %v\n"+
			"IsStrongTip: %v\n"+
			"IsWeakTip: %v\n"+
			"IsMarkedOrphaned: %v\n"+
			"IsOrphaned: %v\n"+
			"AnyStrongParentStronglyOrphaned: %v\n"+
			"AnyWeakParentWeaklyOrphaned: %v\n"+
			"IsStronglyOrphaned: %v\n"+
			"IsWeaklyOrphaned: %v\n"+
			"StronglyConnectedStrongChildren: %d\n"+
			"ConnectedWeakChildren: %d\n"+
			"StronglyOrphanedStrongParents: %d\n"+
			"WeaklyOrphanedWeakParents: %d\n"+
			"]",
		t.block,
		t.tipPool.Get(),
		t.isStrongTipPoolMember.Get(),
		t.isWeakTipPoolMember.Get(),
		t.isStronglyConnectedToTips.Get(),
		t.isConnectedToTips.Get(),
		t.isStronglyReferencedByTips.Get(),
		t.isWeaklyReferencedByTips.Get(),
		t.isReferencedByTips.Get(),
		t.isStrongTip.Get(),
		t.isWeakTip.Get(),
		t.isMarkedOrphaned.Get(),
		t.isOrphaned.Get(),
		t.anyStrongParentStronglyOrphaned.Get(),
		t.anyWeakParentWeaklyOrphaned.Get(),
		t.isStronglyOrphaned.Get(),
		t.isWeaklyOrphaned.Get(),
		t.stronglyConnectedStrongChildren.Get(),
		t.connectedWeakChildren.Get(),
		t.stronglyOrphanedStrongParents.Get(),
		t.weaklyOrphanedWeakParents.Get(),
	)
}

// initLogging initializes the logging for the TipMetadata.
func (t *TipMetadata) initLogging() {
	logLevel := log.LevelTrace

	t.tipPool.LogUpdates(t, logLevel, "TipPool")
	t.livenessThresholdReached.LogUpdates(t, logLevel, "LivenessThresholdReached")
	t.evicted.LogUpdates(t, logLevel, "Evicted")
	t.isStrongTipPoolMember.LogUpdates(t, logLevel, "IsStrongTipPoolMember")
	t.isWeakTipPoolMember.LogUpdates(t, logLevel, "IsWeakTipPoolMember")
	t.isStronglyConnectedToTips.LogUpdates(t, logLevel, "IsStronglyConnectedToTips")
	t.isConnectedToTips.LogUpdates(t, logLevel, "IsConnectedToTips")
	t.isStronglyReferencedByTips.LogUpdates(t, logLevel, "IsStronglyReferencedByTips")
	t.isWeaklyReferencedByTips.LogUpdates(t, logLevel, "IsWeaklyReferencedByTips")
	t.isReferencedByTips.LogUpdates(t, logLevel, "IsReferencedByTips")
	t.isLatestValidationBlock.LogUpdates(t, logLevel, "IsLatestValidationBlock")
	t.referencesLatestValidationBlock.LogUpdates(t, logLevel, "ReferencesLatestValidationBlock")
	t.isStrongTip.LogUpdates(t, logLevel, "IsStrongTip")
	t.isWeakTip.LogUpdates(t, logLevel, "IsWeakTip")
	t.isValidationTip.LogUpdates(t, logLevel, "IsValidationTip")
	t.isMarkedOrphaned.LogUpdates(t, logLevel, "IsMarkedOrphaned")
	t.isOrphaned.LogUpdates(t, logLevel, "IsOrphaned")
	t.anyStrongParentStronglyOrphaned.LogUpdates(t, logLevel, "AnyStrongParentStronglyOrphaned")
	t.anyWeakParentWeaklyOrphaned.LogUpdates(t, logLevel, "AnyWeakParentWeaklyOrphaned")
	t.isStronglyOrphaned.LogUpdates(t, logLevel, "IsStronglyOrphaned")
	t.isWeaklyOrphaned.LogUpdates(t, logLevel, "IsWeaklyOrphaned")
	t.stronglyConnectedStrongChildren.LogUpdates(t, logLevel, "StronglyConnectedStrongChildren")
	t.connectedWeakChildren.LogUpdates(t, logLevel, "ConnectedWeakChildren")
	t.stronglyOrphanedStrongParents.LogUpdates(t, logLevel, "StronglyOrphanedStrongParents")
	t.weaklyOrphanedWeakParents.LogUpdates(t, logLevel, "WeaklyOrphanedWeakParents")
	t.parentsReferencingLatestValidationBlock.LogUpdates(t, logLevel, "ParentsReferencingLatestValidationBlock")

	t.evicted.OnTrigger(t.Logger.Shutdown)
}

// initBehavior initializes the behavior of the TipMetadata.
func (t *TipMetadata) initBehavior() {
	t.referencesLatestValidationBlock.DeriveValueFrom(reactive.NewDerivedVariable2(func(_ bool, isLatestValidationBlock bool, parentsReferencingLatestValidationBlock int) bool {
		return isLatestValidationBlock || parentsReferencingLatestValidationBlock > 0
	}, t.isLatestValidationBlock, t.parentsReferencingLatestValidationBlock))

	t.anyStrongParentStronglyOrphaned.DeriveValueFrom(reactive.NewDerivedVariable[bool, int](func(_ bool, stronglyOrphanedStrongParents int) bool {
		return stronglyOrphanedStrongParents > 0
	}, t.stronglyOrphanedStrongParents))

	t.anyWeakParentWeaklyOrphaned.DeriveValueFrom(reactive.NewDerivedVariable[bool, int](func(_ bool, weaklyOrphanedWeakParents int) bool {
		return weaklyOrphanedWeakParents > 0
	}, t.weaklyOrphanedWeakParents))

	t.isStronglyOrphaned.DeriveValueFrom(reactive.NewDerivedVariable3[bool, bool, bool, bool](func(_ bool, isMarkedOrphaned bool, anyStrongParentStronglyOrphaned bool, anyWeakParentWeaklyOrphaned bool) bool {
		return isMarkedOrphaned || anyStrongParentStronglyOrphaned || anyWeakParentWeaklyOrphaned
	}, t.isMarkedOrphaned, t.anyStrongParentStronglyOrphaned, t.anyWeakParentWeaklyOrphaned))

	t.isWeaklyOrphaned.DeriveValueFrom(reactive.NewDerivedVariable2[bool, bool, bool](func(_ bool, isMarkedOrphaned bool, anyWeakParentWeaklyOrphaned bool) bool {
		return isMarkedOrphaned || anyWeakParentWeaklyOrphaned
	}, t.isMarkedOrphaned, t.anyWeakParentWeaklyOrphaned))

	t.isOrphaned.DeriveValueFrom(reactive.NewDerivedVariable2[bool, bool, bool](func(_ bool, isStronglyOrphaned bool, isWeaklyOrphaned bool) bool {
		return isStronglyOrphaned || isWeaklyOrphaned
	}, t.isStronglyOrphaned, t.isWeaklyOrphaned))

	t.isStrongTipPoolMember.DeriveValueFrom(reactive.NewDerivedVariable3(func(_ bool, tipPool tipmanager.TipPool, isOrphaned bool, isEvicted bool) bool {
		return tipPool == tipmanager.StrongTipPool && !isOrphaned && !isEvicted
	}, t.tipPool, t.isOrphaned, t.evicted))

	t.isWeakTipPoolMember.DeriveValueFrom(reactive.NewDerivedVariable3(func(_ bool, tipPool tipmanager.TipPool, isOrphaned bool, isEvicted bool) bool {
		return tipPool == tipmanager.WeakTipPool && !isOrphaned && !isEvicted
	}, t.tipPool, t.isOrphaned, t.evicted))

	t.isStronglyReferencedByTips.DeriveValueFrom(reactive.NewDerivedVariable[bool, int](func(_ bool, stronglyConnectedStrongChildren int) bool {
		return stronglyConnectedStrongChildren > 0
	}, t.stronglyConnectedStrongChildren))

	t.isWeaklyReferencedByTips.DeriveValueFrom(reactive.NewDerivedVariable[bool, int](func(_ bool, connectedWeakChildren int) bool {
		return connectedWeakChildren > 0
	}, t.connectedWeakChildren))

	t.isReferencedByTips.DeriveValueFrom(reactive.NewDerivedVariable2[bool, bool, bool](func(_ bool, isWeaklyReferencedByTips bool, isStronglyReferencedByTips bool) bool {
		return isWeaklyReferencedByTips || isStronglyReferencedByTips
	}, t.isWeaklyReferencedByTips, t.isStronglyReferencedByTips))

	t.isStronglyConnectedToTips.DeriveValueFrom(reactive.NewDerivedVariable2(func(_ bool, isStrongTipPoolMember bool, isStronglyReferencedByTips bool) bool {
		return isStrongTipPoolMember || isStronglyReferencedByTips
	}, t.isStrongTipPoolMember, t.isStronglyReferencedByTips))

	t.isConnectedToTips.DeriveValueFrom(reactive.NewDerivedVariable3(func(_ bool, isReferencedByTips bool, isStrongTipPoolMember bool, isWeakTipPoolMember bool) bool {
		return isReferencedByTips || isStrongTipPoolMember || isWeakTipPoolMember
	}, t.isReferencedByTips, t.isStrongTipPoolMember, t.isWeakTipPoolMember))

	t.isStrongTip.DeriveValueFrom(reactive.NewDerivedVariable2[bool, bool, bool](func(_ bool, isStrongTipPoolMember bool, isStronglyReferencedByTips bool) bool {
		return isStrongTipPoolMember && !isStronglyReferencedByTips
	}, t.isStrongTipPoolMember, t.isStronglyReferencedByTips))

	t.isWeakTip.DeriveValueFrom(reactive.NewDerivedVariable2[bool, bool, bool](func(_ bool, isWeakTipPoolMember bool, isReferencedByTips bool) bool {
		return isWeakTipPoolMember && !isReferencedByTips
	}, t.isWeakTipPoolMember, t.isReferencedByTips))

	t.isValidationTip.DeriveValueFrom(reactive.NewDerivedVariable2(func(_ bool, isStrongTip bool, referencesLatestValidationBlock bool) bool {
		return isStrongTip && referencesLatestValidationBlock
	}, t.isStrongTip, t.referencesLatestValidationBlock))

	// unsubscribe from external properties when the block is evicted.
	t.evicted.OnTrigger(
		t.isMarkedOrphaned.DeriveValueFrom(reactive.NewDerivedVariable2[bool, bool](func(_ bool, isLivenessThresholdReached bool, isPreAccepted bool) bool {
			return isLivenessThresholdReached && !isPreAccepted
		}, t.livenessThresholdReached, t.block.PreAccepted())),
	)
}

// registerAsLatestValidationBlock registers the TipMetadata as the latest validation block if it is newer than the
// currently registered block and sets the isLatestValidationBlock variable accordingly. The function returns true if the
// operation was successful.
func (t *TipMetadata) registerAsLatestValidationBlock(latestValidationBlock reactive.Variable[*TipMetadata]) (registered bool) {
	latestValidationBlock.Compute(func(currentBlock *TipMetadata) *TipMetadata {
		if registered = currentBlock == nil || currentBlock.block.IssuingTime().Before(t.block.IssuingTime()); registered {
			return t
		}

		return currentBlock
	})

	if registered {
		t.isLatestValidationBlock.Set(true)

		// Once the latestValidationBlock is updated again (by another block), we need to reset the
		// isLatestValidationBlock variable.
		latestValidationBlock.OnUpdateOnce(func(_ *TipMetadata, _ *TipMetadata) {
			t.isLatestValidationBlock.Set(false)
		}, func(_ *TipMetadata, latestValidationBlock *TipMetadata) bool {
			return latestValidationBlock != t
		})
	}

	return registered
}

// connectStrongParent sets up the parent and children related properties for a strong parent.
func (t *TipMetadata) connectStrongParent(strongParent *TipMetadata) {
	t.stronglyOrphanedStrongParents.Monitor(strongParent.isStronglyOrphaned)
	t.parentsReferencingLatestValidationBlock.Monitor(strongParent.referencesLatestValidationBlock)

	// unsubscribe when the parent is evicted, since we otherwise continue to hold a reference to it.
	unsubscribe := strongParent.stronglyConnectedStrongChildren.Monitor(t.isStronglyConnectedToTips)
	strongParent.evicted.OnTrigger(unsubscribe)
}

// connectWeakParent sets up the parent and children related properties for a weak parent.
func (t *TipMetadata) connectWeakParent(weakParent *TipMetadata) {
	t.weaklyOrphanedWeakParents.Monitor(weakParent.isWeaklyOrphaned)

	// unsubscribe when the parent is evicted, since we otherwise continue to hold a reference to it.
	unsubscribe := weakParent.connectedWeakChildren.Monitor(t.isConnectedToTips)
	weakParent.evicted.OnUpdate(func(_ bool, _ bool) { unsubscribe() })
}
