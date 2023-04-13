package dashboard

import (
	"net"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/autopeering/peer"
	"github.com/iotaledger/hive.go/autopeering/peer/service"
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
		IsEnabled: func() bool {
			return true
		},
	}
}

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
	configureEvents()
	return nil
}

func run() error {
	runWebSocketStreams(Component)
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

	// get TangleTime
	//tm := deps.Protocol.Engine().Clock
	//status.TangleTime = tangleTime{
	//	Synced:           deps.Protocol.Engine().IsSynced(),
	//	Bootstrapped:     deps.Protocol.Engine().IsBootstrapped(),
	//	AcceptedBlockID:  lastAcceptedBlock.BlockID().Base58(),
	//	ConfirmedBlockID: lastConfirmedBlock.BlockID().Base58(),
	//	ConfirmedSlot:    int64(deps.Protocol.Engine().LastConfirmedSlot()),
	//	ATT:              tm.Accepted().Time().UnixNano(),
	//	RATT:             tm.Accepted().RelativeTime().UnixNano(),
	//	CTT:              tm.Confirmed().Time().UnixNano(),
	//	RCTT:             tm.Confirmed().RelativeTime().UnixNano(),
	//}
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
