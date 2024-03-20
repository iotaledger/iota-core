package dashboard

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/libp2p/go-libp2p/core/host"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/components/metricstracker"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol"
)

func init() {
	Component = &app.Component{
		Name:      "Dashboard",
		DepsFunc:  func(cDeps dependencies) { deps = cDeps },
		Params:    params,
		Configure: configure,
		Run:       run,
		IsEnabled: func(*dig.Container) bool {
			return ParamsDashboard.Enabled
		},
	}
}

var NodeStartupTimestamp = time.Now()

var (
	Component *app.Component
	deps      dependencies

	server *echo.Echo
)

type dependencies struct {
	dig.In

	Host           host.Host
	Protocol       *protocol.Protocol
	AppInfo        *app.Info
	NetworkManager network.Manager
	MetricsTracker *metricstracker.MetricsTracker
}

func configure() error {
	configureServer()
	return nil
}

func run() error {
	runWebSocketStreams(Component)
	runLiveFeed(Component)
	runVisualizer(Component)
	runSlotsLiveFeed(Component)

	if err := Component.Daemon().BackgroundWorker("Dashboard", func(ctx context.Context) {
		Component.LogInfo("Starting Dashboard ... done")

		stopped := make(chan struct{})
		go func(ctx context.Context) {
			server.Server.BaseContext = func(_ net.Listener) context.Context {
				// set BaseContext to be the same as the plugin, so that requests being processed don't hang the shutdown procedure
				return ctx
			}

			Component.LogInfof("%s started, bind-address=%s, basic-auth=%v", Component.Name, ParamsDashboard.BindAddress, ParamsDashboard.BasicAuth.Enabled)
			if err := server.Start(ParamsDashboard.BindAddress); err != nil {
				if !ierrors.Is(err, http.ErrServerClosed) {
					Component.LogErrorf("Error serving: %w", err)
				}
				close(stopped)
			}
		}(ctx)

		// stop if we are shutting down or the server could not be started
		select {
		case <-ctx.Done():
		case <-stopped:
		}

		Component.LogInfof("Stopping %s ...", Component.Name)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			Component.LogWarnf("Error stopping: %s", err)
		}

		Component.LogInfo("Stopping Dashboard ... done")
	}, daemon.PriorityDashboard); err != nil {
		Component.LogPanicf("failed to start worker: %s", err)
	}

	return nil
}

func configureServer() {
	server = echo.New()
	server.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		Skipper:      middleware.DefaultSkipper,
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPatch, http.MethodPost, http.MethodDelete},
	}))
	server.HideBanner = true
	server.HidePort = true
	server.Use(middleware.Recover())

	if ParamsDashboard.BasicAuth.Enabled {
		server.Use(middleware.BasicAuth(func(username, password string, _ echo.Context) (bool, error) {
			if username == ParamsDashboard.BasicAuth.Username &&
				password == ParamsDashboard.BasicAuth.Password {
				return true, nil
			}

			return false, nil
		}))
	}

	setupRoutes(server)
}

func currentNodeStatus() *nodestatus {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	status := &nodestatus{}
	status.ID = deps.Host.ID().String()

	// node status
	status.Version = deps.AppInfo.Version
	status.Uptime = time.Since(NodeStartupTimestamp).Milliseconds()

	// memory metrics
	status.Mem = &memmetrics{
		HeapSys:      m.HeapSys,
		HeapAlloc:    m.HeapAlloc,
		HeapIdle:     m.HeapIdle,
		HeapReleased: m.HeapReleased,
		HeapObjects:  m.HeapObjects,
		NumGC:        m.NumGC,
		LastPauseGC:  m.PauseNs[(m.NumGC+255)%256],
	}
	// get TangleTime
	cl := deps.Protocol.Engines.Main.Get().Clock
	syncStatus := deps.Protocol.Engines.Main.Get().SyncManager.SyncStatus()

	status.TangleTime = tangleTime{
		Synced:             syncStatus.NodeSynced,
		Bootstrapped:       syncStatus.NodeBootstrapped,
		AcceptedBlockSlot:  int64(syncStatus.LastAcceptedBlockSlot),
		ConfirmedBlockSlot: int64(syncStatus.LastConfirmedBlockSlot),
		CommittedSlot:      int64(syncStatus.LatestCommitment.Slot()),
		ConfirmedSlot:      int64(syncStatus.LatestFinalizedSlot),
		ATT:                cl.Accepted().Time().UnixNano(),
		RATT:               cl.Accepted().RelativeTime().UnixNano(),
		CTT:                cl.Confirmed().Time().UnixNano(),
		RCTT:               cl.Confirmed().RelativeTime().UnixNano(),
	}

	return status
}

func neighborMetrics() []neighbormetric {
	if deps.NetworkManager == nil {
		return []neighbormetric{}
	}

	// gossip plugin might be disabled
	neighbors := deps.NetworkManager.AllNeighbors()
	if neighbors == nil {
		return []neighbormetric{}
	}

	stats := make([]neighbormetric, 0, len(neighbors))
	for _, neighbor := range neighbors {
		stats = append(stats, neighbormetric{
			ID:             neighbor.Peer().ID.String(),
			Addresses:      fmt.Sprintf("%s", neighbor.Peer().PeerAddresses),
			PacketsRead:    neighbor.PacketsRead(),
			PacketsWritten: neighbor.PacketsWritten(),
		})
	}

	return stats
}
