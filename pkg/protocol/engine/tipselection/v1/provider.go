package tipselectionv1

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipselection"
)

// NewProvider creates a new TipManager provider for an engine.
func NewProvider(opts ...options.Option[TipSelection]) module.Provider[*engine.Engine, tipselection.TipSelection] {
	return module.Provide(func(e *engine.Engine) tipselection.TipSelection {
		t := New(e.TipManager, e.Ledger.ConflictDAG(), e.EvictionState.LatestRootBlocks, opts...)

		e.HookConstructed(func() {
			e.TipManager.OnBlockAdded(t.classifyTip)

			t.TriggerInitialized()
		})

		e.HookStopped(t.TriggerStopped)

		return t
	})
}
