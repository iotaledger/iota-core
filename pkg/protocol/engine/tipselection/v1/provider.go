package tipselectionv1

import (
	"math"
	"time"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipselection"
	iotago "github.com/iotaledger/iota.go/v4"
)

// NewProvider creates a new TipSelection provider, that can be used to inject the component into an engine.
func NewProvider(opts ...options.Option[TipSelection]) module.Provider[*engine.Engine, tipselection.TipSelection] {
	return module.Provide(func(e *engine.Engine) tipselection.TipSelection {
		t := New(e.NewSubModule("TipSelection"), opts...)

		e.ConstructedEvent().OnTrigger(func() {
			// wait for submodules to be constructed (so all of their properties are available)
			module.WaitAll(module.Module.ConstructedEvent, e.TipManager, e.Ledger, e.SybilProtection).OnTrigger(func() {
				t.Construct(e.TipManager, e.Ledger.SpendDAG(), e.Ledger.MemPool().TransactionMetadata, func() iotago.BlockID { return lo.Return1(e.EvictionState.LatestActiveRootBlock()) }, DynamicLivenessThreshold(func() int {
					return e.SybilProtection.SeatManager().OnlineCommittee().Size()
				}))
			})

			t.InitializedEvent().Trigger()
		})

		return t
	})
}

// DynamicLivenessThreshold returns a function that calculates the liveness threshold for a tip.
func DynamicLivenessThreshold(committeeSizeProvider func() int) func(tip tipmanager.TipMetadata) time.Duration {
	return func(tip tipmanager.TipMetadata) time.Duration {
		// We want to scale the liveness threshold based on the number of witnesses:
		//  0 witnesses: approval modifier is 0 -> LivenessThresholdLowerBound
		//  <=1/3: scale linearly
		//  >1/3: approval modifier is 1 -> LivenessThresholdUpperBound
		var (
			params                      = tip.Block().ModelBlock().ProtocolBlock().API.ProtocolParameters()
			livenessThresholdLowerBound = params.LivenessThresholdLowerBound()
			livenessWindow              = float64(params.LivenessThresholdUpperBound() - livenessThresholdLowerBound)
			expectedWitnessCount        = math.Ceil(float64(committeeSizeProvider()) / 3.0)
			approvalModifier            = math.Min(float64(tip.Block().WitnessCount())/expectedWitnessCount, 1.0)
		)

		return livenessThresholdLowerBound + time.Duration(approvalModifier*livenessWindow)
	}
}
