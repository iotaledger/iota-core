package tipselectionv1

import (
	"math"
	"time"

	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipselection"
)

// NewProvider creates a new TipSelection provider, that can be used to inject the component into an engine.
func NewProvider(opts ...options.Option[TipSelection]) module.Provider[*engine.Engine, tipselection.TipSelection] {
	return module.Provide(func(e *engine.Engine) tipselection.TipSelection {
		livenessThresholdFunc := func(tip tipmanager.TipMetadata) time.Duration {
			protocolParameters := e.APIForSlot(tip.Block().ID().Index()).ProtocolParameters()

			livenessThresholdLowerBound := protocolParameters.LivenessThresholdLowerBound()

			livenessWindow := protocolParameters.LivenessThresholdUpperBound() - livenessThresholdLowerBound

			approvalModifier := math.Min(
				float64(tip.Block().WitnessCount())/float64(e.SybilProtection.SeatManager().OnlineCommittee().Size())/3.0,
				1.0,
			)

			return livenessThresholdLowerBound + time.Duration(float64(livenessWindow)*approvalModifier)
		}

		t := New(e.TipManager, e.Ledger.ConflictDAG(), e.Ledger.MemPool().TransactionMetadata, e.EvictionState.LatestRootBlocks, livenessThresholdFunc, opts...)

		e.HookConstructed(func() {
			e.Events.AcceptedBlockProcessed.Hook(func(block *blocks.Block) {
				t.SetAcceptanceTime(block.IssuingTime())
			})
		})

		e.HookShutdown(t.Shutdown)

		return t
	})
}
