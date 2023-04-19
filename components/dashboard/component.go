package dashboard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/autopeering/peer"
	"github.com/iotaledger/hive.go/autopeering/peer/service"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/network/p2p"
	"github.com/iotaledger/iota-core/pkg/protocol"
)

func init() {
	Component = &app.Component{
		Name:      "Dashboard",
		DepsFunc:  func(cDeps dependencies) { deps = cDeps },
		Params:    params,
		Configure: configure,
		Run:       run,
		IsEnabled: func(c *dig.Container) bool {
			return ParamsDashboard.Enabled
		},
	}
}

var (
	NodeStartupTimestamp = time.Now()
)

var (
	Component *app.Component
	deps      dependencies

	server *echo.Echo
)

type dependencies struct {
	dig.In

	Protocol   *protocol.Protocol
	LocalPeer  *peer.Local
	AppInfo    *app.Info
	P2PManager *p2p.Manager
}

func configure() error {
	// TODO: register events here
	configureServer()
	return nil
}

func run() error {
	runWebSocketStreams(Component)
	runLiveFeed(Component)
	runVisualizer(Component)

	if err := Component.Daemon().BackgroundWorker("Dashboard", func(ctx context.Context) {
		Component.LogInfo("Starting Dashboard ... done")

		stopped := make(chan struct{})
		go func() {
			Component.LogInfof("%s started, bind-address=%s, basic-auth=%v", Component.Name, ParamsDashboard.BindAddress, ParamsDashboard.BasicAuth.Enabled)
			if err := server.Start(ParamsDashboard.BindAddress); err != nil {
				if !errors.Is(err, http.ErrServerClosed) {
					log.Errorf("Error serving: %s", err)
				}
				close(stopped)
			}
		}()

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
		server.Use(middleware.BasicAuth(func(username, password string, c echo.Context) (bool, error) {
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
	status.ID = deps.LocalPeer.ID().String()

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

	//get TangleTime
	cl := deps.Protocol.MainEngineInstance().Clock
	status.TangleTime = tangleTime{
		Synced:       deps.Protocol.MainEngineInstance().IsSynced(),
		Bootstrapped: deps.Protocol.MainEngineInstance().IsBootstrapped(),
		//AcceptedBlockID:  lastAcceptedBlock.BlockID().Base58(),
		//ConfirmedBlockID: lastConfirmedBlock.BlockID().Base58(),
		//ConfirmedSlot:    int64(deps.Protocol.Engine().LastConfirmedSlot()),
		ATT:  cl.Accepted().Time().UnixNano(),
		RATT: cl.Accepted().RelativeTime().UnixNano(),
		CTT:  cl.Confirmed().Time().UnixNano(),
		RCTT: cl.Confirmed().RelativeTime().UnixNano(),
	}
	//
	//scheduler := deps.Protocol.CongestionControl.Scheduler()
	//if scheduler == nil {
	//	return nil
	//}
	//
	//deficit, _ := scheduler.Deficit(deps.Local.ID()).Float64()

	//status.Scheduler = schedulerMetric{
	//	Running:           deps.Protocol.CongestionControl.Scheduler().IsRunning(),
	//	Rate:              deps.Protocol.CongestionControl.Scheduler().Rate().String(),
	//	MaxBufferSize:     deps.Protocol.CongestionControl.Scheduler().MaxBufferSize(),
	//	CurrentBufferSize: deps.Protocol.CongestionControl.Scheduler().BufferSize(),
	//	Deficit:           deficit,
	//}
	return status
}

func neighborMetrics() []neighbormetric {
	var stats []neighbormetric
	if deps.P2PManager == nil {
		return stats
	}

	// gossip plugin might be disabled
	neighbors := deps.P2PManager.AllNeighbors()
	if neighbors == nil {
		return stats
	}

	for _, neighbor := range neighbors {
		//origin := "Inbound"
		//for _, p := range deps.P2PManager.AllNeighbors() {
		//	if neighbor.Peer == peer {
		//		origin = "Outbound"
		//		break
		//	}
		//}

		host := neighbor.Peer.IP().String()
		port := neighbor.Peer.Services().Get(service.P2PKey).Port()
		stats = append(stats, neighbormetric{
			ID:               neighbor.Peer.ID().String(),
			Address:          net.JoinHostPort(host, strconv.Itoa(port)),
			PacketsRead:      neighbor.PacketsRead(),
			PacketsWritten:   neighbor.PacketsWritten(),
			ConnectionOrigin: "Inbound", //origin
		})
	}
	return stats
}
