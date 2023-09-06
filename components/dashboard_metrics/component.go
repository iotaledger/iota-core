package dashboardmetrics

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/dig"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/runtime/timeutil"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/components/restapi"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
)

const (
	// RouteNodeInfoExtended is the route to get additional info about the node.
	// GET returns the extended info of the node.
	RouteNodeInfoExtended = "/info"

	// RouteDatabaseSizes is the route to get the size of the databases.
	// GET returns the sizes of the databases.
	RouteDatabaseSizes = "/database/sizes"

	// RouteGossipMetrics is the route to get metrics about gossip.
	// GET returns the gossip metrics.
	RouteGossipMetrics = "/gossip"
)

func init() {
	Component = &app.Component{
		Name:      "DashboardMetrics",
		DepsFunc:  func(cDeps dependencies) { deps = cDeps },
		Configure: configure,
		Run:       run,
		IsEnabled: func(c *dig.Container) bool {
			return restapi.ParamsRestAPI.Enabled
		},
	}
}

var (
	Component *app.Component
	deps      dependencies
)

type dependencies struct {
	dig.In

	Host             host.Host
	Protocol         *protocol.Protocol
	RestRouteManager *restapipkg.RestRouteManager
	AppInfo          *app.Info
}

func configure() error {
	// check if RestAPI plugin is disabled
	if !Component.App().IsComponentEnabled(restapi.Component.Identifier()) {
		Component.LogPanicf("RestAPI plugin needs to be enabled to use the %s plugin", Component.Name)
	}
	configureComponentCountersEvents()

	routeGroup := deps.RestRouteManager.AddRoute("dashboard-metrics/v2")

	routeGroup.GET(RouteNodeInfoExtended, func(c echo.Context) error {
		return httpserver.JSONResponse(c, http.StatusOK, nodeInfoExtended())
	})

	routeGroup.GET(RouteDatabaseSizes, func(c echo.Context) error {
		resp, err := databaseSizesMetrics()
		if err != nil {
			return err
		}

		return httpserver.JSONResponse(c, http.StatusOK, resp)
	})

	return nil
}

func run() error {
	Component.Logger().Infof("Starting %s ...", Component.Name)
	if err := Component.Daemon().BackgroundWorker("DashboardMetricsUpdater", func(ctx context.Context) {
		// Do not block until the Ticker is shutdown because we might want to start multiple Tickers and we can
		// safely ignore the last execution when shutting down.
		timeutil.NewTicker(func() {
			measurePerComponentCounter()
		}, 1*time.Second, ctx)

		// Wait before terminating so we get correct log blocks from the daemon regarding the shutdown order.
		<-ctx.Done()
	}, daemon.PriorityDashboardMetrics); err != nil {
		Component.LogPanicf("failed to start worker: %s", err)
	}

	return nil
}

func configureComponentCountersEvents() {
	deps.Protocol.Events.Network.BlockReceived.Hook(func(_ *model.Block, _ peer.ID) {
		incComponentCounter(Received)
	})

	deps.Protocol.Events.Engine.BlockDAG.BlockAttached.Hook(func(_ *blocks.Block) {
		incComponentCounter(Attached)
	})

	deps.Protocol.Events.Engine.BlockDAG.BlockSolid.Hook(func(b *blocks.Block) {
		incComponentCounter(Solidified)
	})

	deps.Protocol.Events.Engine.Booker.BlockBooked.Hook(func(b *blocks.Block) {
		incComponentCounter(Booked)
	})

	deps.Protocol.Events.Engine.Scheduler.BlockScheduled.Hook(func(b *blocks.Block) {
		incComponentCounter(Scheduled)
	})
}
