package metrics

// metrics is the plugin instance responsible for collection of prometheus metrics.
// All metrics should be defined in metrics_namespace.go files with different namespace for each new collection.
// Metrics naming should follow the guidelines from: https://prometheus.io/docs/practices/naming/
// In short:
// 	all metrics should be in base units, do not mix units,
// 	add suffix describing the unit,
// 	use 'total' suffix for accumulating counter

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/autopeering/peer"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/components/metrics/collector"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/protocol"
)

func init() {
	Component = &app.Component{
		Name:     "Metrics",
		DepsFunc: func(cDeps dependencies) { deps = cDeps },
		Params:   params,
		Run:      run,
		IsEnabled: func(container *dig.Container) bool {
			if err := container.Provide(createCollector); err != nil {
				panic(ierrors.Wrap(err, "failed to provide collector"))
			}
			return ParamsMetrics.Enabled
		},
	}
}

// PluginName is the name of the metrics collector plugin.
var (
	Component *app.Component
	deps      dependencies

	server *http.Server
)

type dependencies struct {
	dig.In

	Local    *peer.Local
	Protocol *protocol.Protocol

	Collector *collector.Collector
}

func run() error {
	Component.LogInfo("Starting Prometheus exporter ...")

	if ParamsMetrics.GoMetrics {
		deps.Collector.Registry.MustRegister(collectors.NewGoCollector())
	}
	if ParamsMetrics.ProcessMetrics {
		deps.Collector.Registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	}

	registerMetrics()

	return Component.Daemon().BackgroundWorker("Prometheus exporter", func(ctx context.Context) {
		Component.LogInfo("Starting Prometheus exporter ... done")

		engine := echo.New()
		engine.Use(middleware.Recover())

		engine.GET("/metrics", func(c echo.Context) error {
			deps.Collector.Collect()

			handler := promhttp.HandlerFor(
				deps.Collector.Registry,
				promhttp.HandlerOpts{
					EnableOpenMetrics: true,
				},
			)
			if ParamsMetrics.PromhttpMetrics {
				handler = promhttp.InstrumentMetricHandler(deps.Collector.Registry, handler)
			}
			handler.ServeHTTP(c.Response().Writer, c.Request())

			return nil
		})
		bindAddr := ParamsMetrics.BindAddress
		server = &http.Server{Addr: bindAddr, Handler: engine, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}

		go func() {
			Component.LogInfof("You can now access the Prometheus exporter using: http://%s/metrics", bindAddr)
			if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				Component.LogError("Stopping Prometheus exporter due to an error ... done")
			}
		}()

		<-ctx.Done()
		Component.LogInfo("Stopping Prometheus exporter ...")

		if server != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := server.Shutdown(ctx)
			if err != nil {
				Component.LogError(err.Error())
			}
			cancel()
		}
		Component.LogInfo("Stopping Prometheus exporter ... done")
	}, daemon.PriorityMetrics)
}

func createCollector() *collector.Collector {
	return collector.New()
}

func registerMetrics() {
	deps.Collector.RegisterCollection(TangleMetrics)
	deps.Collector.RegisterCollection(ConflictMetrics)
	deps.Collector.RegisterCollection(InfoMetrics)
	deps.Collector.RegisterCollection(DBMetrics)
	deps.Collector.RegisterCollection(CommitmentsMetrics)
	deps.Collector.RegisterCollection(SlotMetrics)
	deps.Collector.RegisterCollection(AccountMetrics)
}
