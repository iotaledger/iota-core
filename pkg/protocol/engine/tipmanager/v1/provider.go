package tipmanagerv1

import (
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
)

// NewProvider creates a new TipManager provider, that can be used to inject the component into an engine.
func NewProvider() module.Provider[*engine.Engine, tipmanager.TipManager] {
	return module.Provide(func(e *engine.Engine) tipmanager.TipManager {
		t := New(e.BlockCache.Block)

		e.Constructed.OnTrigger(func() {
			tipWorker := e.Workers.CreatePool("AddTip", workerpool.WithWorkerCount(2))
			e.Events.Scheduler.BlockScheduled.Hook(lo.Void(t.AddBlock), event.WithWorkerPool(tipWorker))
			e.Events.Scheduler.BlockSkipped.Hook(lo.Void(t.AddBlock), event.WithWorkerPool(tipWorker))
			e.BlockCache.Evict.Hook(t.Evict)

			e.Events.TipManager.BlockAdded.LinkTo(t.blockAdded)

			t.TriggerInitialized()
		})

		e.Shutdown.OnTrigger(func() {
			t.TriggerShutdown()
			t.TriggerStopped()
		})

		return t
	})
}
