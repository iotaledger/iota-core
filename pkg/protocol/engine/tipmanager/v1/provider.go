package tipmanagerv1

import (
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

// NewProvider creates a new TipManager provider, that can be used to inject the component into an engine.
func NewProvider() module.Provider[*engine.Engine, tipmanager.TipManager] {
	return module.Provide(func(e *engine.Engine) tipmanager.TipManager {
		t := New(e.NewSubModule("TipManager"), e.BlockCache.Block, e.SybilProtection.SeatManager().CommitteeInSlot)

		e.ConstructedEvent().OnTrigger(func() {
			tipWorker := e.Workers.CreatePool("AddTip", workerpool.WithWorkerCount(2))

			e.Events.Scheduler.BlockScheduled.Hook(lo.Void(t.AddBlock), event.WithWorkerPool(tipWorker))
			e.Events.Scheduler.BlockSkipped.Hook(lo.Void(t.AddBlock), event.WithWorkerPool(tipWorker))
			e.Events.Evict.Hook(t.Evict)

			e.Events.SeatManager.OnlineCommitteeSeatAdded.Hook(func(index account.SeatIndex, _ iotago.AccountID) {
				t.AddSeat(index)
			})
			e.Events.SeatManager.OnlineCommitteeSeatRemoved.Hook(t.RemoveSeat)

			e.Events.TipManager.BlockAdded.LinkTo(t.blockAdded)
		})

		e.ShutdownEvent().OnTrigger(func() {
			t.ShutdownEvent().Trigger()
		})

		return t
	})
}
