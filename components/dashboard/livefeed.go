package dashboard

import (
	"context"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

func runLiveFeed(component *app.Component) {
	if err := component.Daemon().BackgroundWorker("Dashboard[Livefeed]", func(ctx context.Context) {
		hook := deps.Protocol.Events.Engine.BlockDAG.BlockAttached.Hook(func(b *blocks.Block) {
			// TODO: use actual payload type
			broadcastWsBlock(&wsblk{MsgTypeBlock, &blk{b.ID().ToHex(), 0, 5}})
		}, event.WithWorkerPool(Component.WorkerPool))

		<-ctx.Done()
		component.LogInfo("Stopping Dashboard[Livefeed] ...")
		hook.Unhook()
		component.LogInfo("Stopping Dashboard[Livefeed] ... done")
	}, daemon.PriorityDashboard); err != nil {
		component.LogPanicf("Failed to start as daemon: %s", err)
	}
}
