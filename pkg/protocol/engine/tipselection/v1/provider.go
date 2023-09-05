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
	"github.com/iotaledger/iota.go/v4/api"
)

// NewProvider creates a new TipSelection provider, that can be used to inject the component into an engine.
func NewProvider(opts ...options.Option[TipSelection]) module.Provider[*engine.Engine, tipselection.TipSelection] {
	return module.Provide(func(e *engine.Engine) tipselection.TipSelection {
		t := New(opts...)

		e.HookConstructed(func() {
			// wait for submodules to be constructed (so all of their properties are available)
			module.OnAllConstructed(func() {
				t.Construct(e.TipManager, e.Ledger.ConflictDAG(), e.Ledger.MemPool().TransactionMetadata, e.EvictionState.LatestRootBlocks, DynamicLivenessThreshold(e, e.SybilProtection.SeatManager().OnlineCommittee().Size))

				e.Events.AcceptedBlockProcessed.Hook(func(block *blocks.Block) {
					t.SetAcceptanceTime(block.IssuingTime())
				})
			}, e.TipManager, e.Ledger, e.SybilProtection)
		})

		e.HookShutdown(t.Shutdown)

		return t
	})
}

// DynamicLivenessThreshold returns a function that calculates the liveness threshold for a tip.
func DynamicLivenessThreshold(apiProvider api.Provider, committeeSizeProvider func() int) func(tip tipmanager.TipMetadata) time.Duration {
	return func(tip tipmanager.TipMetadata) time.Duration {
		var (
			params                      = apiProvider.APIForSlot(tip.Block().ID().Index()).ProtocolParameters()
			livenessThresholdLowerBound = params.LivenessThresholdLowerBound()
			livenessWindow              = float64(params.LivenessThresholdUpperBound() - livenessThresholdLowerBound)
			approvalModifier            = math.Min(float64(tip.Block().WitnessCount())/float64(committeeSizeProvider())/3.0, 1.0)
		)

		return livenessThresholdLowerBound + time.Duration(approvalModifier*livenessWindow)
	}
}
