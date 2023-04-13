package trivialtipmanager

import (
	"time"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag"
)

func (t *TipManager) isValidTip(tip *blockdag.Block) (err error) {
	// TODO: add when we have acceptance and clock
	// if !t.isPastConeTimestampCorrect(tip) {
	// 	return errors.Errorf("cannot select tip due to TSC condition tip issuing time (%s), time (%s), min supported time (%s), block id (%s), tip pool size (%d), scheduled: (%t), orphaned: (%t), accepted: (%t)",
	// 		tip.IssuingTime(),
	// 		t.engine.Clock.Accepted().Time(),
	// 		t.engine.Clock.Accepted().Time().Add(-t.optsTimeSinceConfirmationThreshold),
	// 		tip.ID().Base58(),
	// 		t.tips.Size(),
	// 		tip.IsScheduled(),
	// 		tip.IsOrphaned(),
	// 		t.blockAcceptanceGadget.IsBlockAccepted(tip.ID()),
	// 	)
	// }

	return nil
}

// isPastConeTimestampCorrect performs the TSC check for the given tip.
// Conceptually, this involves the following steps:
//  1. Collect all accepted blocks in the tip's past cone at the boundary of accepted/unaccapted.
//  2. Order by timestamp (ascending), if the oldest accepted block > TSC threshold then return false.
//
// This function is optimized through the use of markers and the following assumption:
//
//	If there's any unaccepted block >TSC threshold, then the oldest accepted block will be >TSC threshold, too.
func (t *TipManager) isPastConeTimestampCorrect(block *blockdag.Block) (timestampValid bool) {
	// TODO: add when we have acceptance and clock
	// minSupportedTimestamp := t.engine.Clock.Accepted().Time().Add(-t.optsTimeSinceConfirmationThreshold)

	if !t.isBootstrappedFunc() {
		// If the node is not bootstrapped we do not have a valid timestamp to compare against.
		// In any case, a node should never perform tip selection if not bootstrapped (via issuer plugin).
		return true
	}

	// TODO: add when we have acceptance and clock
	// timestampValid = t.checkBlockRecursive(block, minSupportedTimestamp)
	timestampValid = true

	return
}

func (t *TipManager) checkBlockRecursive(block *blockdag.Block, minSupportedTimestamp time.Time) (timestampValid bool) {
	if storage := t.walkerCache.Get(block.ID().Index(), false); storage != nil {
		if _, exists := storage.Get(block.ID()); exists {
			return true
		}
	}

	// TODO: add when we have a block type with IssuingTime
	// if block is older than TSC then it's incorrect no matter the acceptance status
	// if block.IssuingTime().Before(minSupportedTimestamp) {
	// 	return false
	// }

	// TODO: add when we have acceptance
	// if block is younger than TSC and accepted, then return timestampValid=true
	// if t.blockAcceptanceGadget.IsBlockAccepted(block.ID()) {
	// 	t.walkerCache.Get(block.ID().Index(), true).Set(block.ID(), types.Void)
	// 	return true
	// }

	if block.IsOrphaned() {
		return false
	}

	// if block is younger than TSC and not accepted, walk through strong parents' past cones
	for _, strongParentID := range block.Block().StrongParents {
		strongParentBlock, exists := t.blockRetrieverFunc(strongParentID)
		if !exists {
			return false
		}

		if !t.checkBlockRecursive(strongParentBlock, minSupportedTimestamp) {
			return false
		}
	}

	t.walkerCache.Get(block.ID().Index(), true).Set(block.ID(), types.Void)
	return true
}

// WithTimeSinceConfirmationThreshold returns an option that sets the time since confirmation threshold.
func WithTimeSinceConfirmationThreshold(timeSinceConfirmationThreshold time.Duration) options.Option[TipManager] {
	return func(o *TipManager) {
		o.optsTimeSinceConfirmationThreshold = timeSinceConfirmationThreshold
	}
}
