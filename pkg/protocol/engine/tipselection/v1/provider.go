package tipselectionv1

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipselection"
)

// NewProvider creates a new TipSelection provider, that can be used to inject the component into an engine.
func NewProvider(opts ...options.Option[TipSelection]) module.Provider[*engine.Engine, tipselection.TipSelection] {
	return module.Provide(func(e *engine.Engine) tipselection.TipSelection {
		t := New(e, e.TipManager, e.Ledger.ConflictDAG(), e.Ledger.MemPool(), e.EvictionState.LatestRootBlocks, opts...)

		e.HookConstructed(func() {
			e.Ledger.HookInitialized(func() {
				e.Events.AcceptedBlockProcessed.Hook(func(block *blocks.Block) {
					t.SetLivenessThreshold(block.IssuingTime().Add(-e.CurrentAPI().ProtocolParameters().LivenessThresholdDuration()))
				})

				t.conflictDAG = e.Ledger.ConflictDAG()
				t.memPool = e.Ledger.MemPool()

				t.TriggerInitialized()
			})

			e.TipManager.OnBlockAdded(t.classifyTip)
		})

		e.HookStopped(t.TriggerStopped)

		return t
	})
}
