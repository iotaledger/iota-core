package metricstracker

import (
	"context"

	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
)

func init() {
	Component = &app.Component{
		Name:     "MetricsTracker",
		DepsFunc: func(cDeps dependencies) { deps = cDeps },
		Params:   params,
		Provide:  provide,
		Run:      run,
		IsEnabled: func(_ *dig.Container) bool {
			return ParamsMetricsTracker.Enabled
		},
	}
}

var (
	Component *app.Component
	deps      dependencies
)

type dependencies struct {
	dig.In
	Protocol       *protocol.Protocol
	MetricsTracker *MetricsTracker
}

func provide(c *dig.Container) error {
	type metricsTrackerDeps struct {
		dig.In

		Protocol *protocol.Protocol
	}

	if err := c.Provide(func(deps metricsTrackerDeps) *MetricsTracker {
		m := New(deps.Protocol.Engines.Main.Get().SyncManager.IsBootstrapped)

		return m
	}); err != nil {
		Component.LogPanic(err.Error())
	}

	return nil
}

func run() error {
	Component.LogInfo("Starting Metrics Tracker ...")

	if err := Component.Daemon().BackgroundWorker("Metrics Tracker", func(ctx context.Context) {
		Component.LogInfo("Starting Metrics Tracker ... done")

		unhook := lo.Batch(
			deps.Protocol.Events.Engine.PostSolidFilter.BlockAllowed.Hook(func(_ *blocks.Block) {
				deps.MetricsTracker.metrics.Blocks.Inc()
			}, event.WithWorkerPool(Component.WorkerPool)).Unhook,
			deps.Protocol.Events.Engine.Notarization.SlotCommitted.Hook(func(_ *notarization.SlotCommittedDetails) {
				deps.MetricsTracker.measure()
			}, event.WithWorkerPool(Component.WorkerPool)).Unhook,
			deps.Protocol.Events.Engine.BlockGadget.BlockConfirmed.Hook(func(_ *blocks.Block) {
				deps.MetricsTracker.metrics.ConfirmedBlocks.Inc()
			}, event.WithWorkerPool(Component.WorkerPool)).Unhook,
		)

		<-ctx.Done()
		Component.LogInfo("Stopping Metrics Tracker ...")

		unhook()
		Component.LogInfo("Stopping Metrics Tracker ... done")
	}, daemon.PriorityDashboardMetrics); err != nil {
		Component.LogPanicf("failed to start worker: %s", err)
	}

	return nil
}
