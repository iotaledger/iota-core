package dashboard

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/timeutil"
	dashboardmetrics "github.com/iotaledger/iota-core/components/dashboard_metrics"
	"github.com/iotaledger/iota-core/pkg/daemon"
)

var (
	// settings
	webSocketWriteTimeout = 3 * time.Second

	// clients
	wsClientsMu    syncutils.RWMutex
	wsClients      = make(map[uint64]*wsclient)
	nextWsClientID uint64

	// gorilla websocket layer
	upgrader = websocket.Upgrader{
		HandshakeTimeout:  webSocketWriteTimeout,
		CheckOrigin:       func(r *http.Request) bool { return true },
		EnableCompression: true,
	}
)

// a websocket client with a channel for downstream blocks.
type wsclient struct {
	// downstream block channel.
	channel chan interface{}
	// a channel which is closed when the websocket client is disconnected.
	exit chan struct{}
}

func runWebSocketStreams(component *app.Component) {
	process := func(msg interface{}) {
		switch x := msg.(type) {
		case float64:
			broadcastWsBlock(&wsblk{MsgTypeBPSMetric, x})
			broadcastWsBlock(&wsblk{MsgTypeNodeStatus, currentNodeStatus()})
			broadcastWsBlock(&wsblk{MsgTypeNeighborMetric, neighborMetrics()})
			broadcastWsBlock(&wsblk{MsgTypeTipsMetric, &tipsInfo{
				TotalTips: len(deps.Protocol.Engines.Main.Get().TipManager.StrongTips()) + len(deps.Protocol.Engines.Main.Get().TipManager.WeakTips()),
			}})
		case *componentsmetric:
			broadcastWsBlock(&wsblk{MsgTypeComponentCounterMetric, x})
		case *rateSetterMetric:
			broadcastWsBlock(&wsblk{MsgTypeRateSetterMetric, x})
		}
	}

	if err := component.Daemon().BackgroundWorker("Dashboard[StatusUpdate]", func(ctx context.Context) {
		defer Component.LogInfo("Stopping Dashboard[StatusUpdate] ... done")

		unhook := lo.Batch(
			dashboardmetrics.Events.ComponentCounterUpdated.Hook(func(event *dashboardmetrics.ComponentCounterUpdatedEvent) {
				process(&componentsmetric{
					Store:      event.ComponentStatus[dashboardmetrics.Allowed],
					Solidifier: event.ComponentStatus[dashboardmetrics.Solidified],
					Scheduler:  event.ComponentStatus[dashboardmetrics.Scheduled],
					Booker:     event.ComponentStatus[dashboardmetrics.Booked],
				})
			}, event.WithWorkerPool(Component.WorkerPool)).Unhook,
		)
		timeutil.NewTicker(func() {
			bpsInfo := deps.MetricsTracker.NodeMetrics()
			process(bpsInfo.BlocksPerSecond)
		}, 2*time.Second, ctx)

		<-ctx.Done()
		Component.LogInfo("Stopping Dashboard[StatusUpdate] ...")
		unhook()
		Component.LogInfo("Stopping Dashboard[StatusUpdate] ... done")
	}, daemon.PriorityDashboard); err != nil {
		Component.LogPanicf("Failed to start as daemon: %s", err)
	}
}

// registers and creates a new websocket client.
func registerWSClient() (uint64, *wsclient) {
	wsClientsMu.Lock()
	defer wsClientsMu.Unlock()
	clientID := nextWsClientID
	wsClient := &wsclient{
		channel: make(chan interface{}, 2000),
		exit:    make(chan struct{}),
	}
	wsClients[clientID] = wsClient
	nextWsClientID++
	return clientID, wsClient
}

// removes the websocket client with the given id.
func removeWsClient(clientID uint64) {
	wsClientsMu.Lock()
	defer wsClientsMu.Unlock()

	close(wsClients[clientID].exit)
	delete(wsClients, clientID)
}

// broadcasts the given block to all connected websocket clients.
func broadcastWsBlock(blk interface{}, dontDrop ...bool) {
	wsClientsMu.RLock()
	wsClientsCopy := lo.MergeMaps(make(map[uint64]*wsclient), wsClients)
	wsClientsMu.RUnlock()
	for _, wsClient := range wsClientsCopy {
		if len(dontDrop) > 0 {
			select {
			case <-wsClient.exit:
			case wsClient.channel <- blk:
			}
			return
		}

		select {
		case <-wsClient.exit:
		case wsClient.channel <- blk:
		default:
			// potentially drop if slow consumer
		}
	}
}

func websocketRoute(c echo.Context) error {
	defer func() {
		if r := recover(); r != nil {
			Component.LogErrorf("recovered from websocket handle func: %s", r)
		}
	}()

	// upgrade to websocket connection
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()
	ws.EnableWriteCompression(true)

	// cleanup client websocket
	clientID, wsClient := registerWSClient()
	defer removeWsClient(clientID)

	for {
		var blk interface{}
		select {
		case <-wsClient.exit:
			return nil
		case blk = <-wsClient.channel:
		}

		if err := ws.WriteJSON(blk); err != nil {
			break
		}
		if err := ws.SetWriteDeadline(time.Now().Add(webSocketWriteTimeout)); err != nil {
			break
		}
	}
	return nil
}

func sendJSON(ws *websocket.Conn, blk *wsblk) error {
	if err := ws.WriteJSON(blk); err != nil {
		return err
	}
	if err := ws.SetWriteDeadline(time.Now().Add(webSocketWriteTimeout)); err != nil {
		return err
	}
	return nil
}
