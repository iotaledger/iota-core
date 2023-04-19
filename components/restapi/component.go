package restapi

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/pkg/errors"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/jwt"
)

func init() {
	Component = &app.Component{
		Name:             "RestAPI",
		DepsFunc:         func(cDeps dependencies) { deps = cDeps },
		Params:           params,
		InitConfigParams: initConfigParams,
		Provide:          provide,
		Configure:        configure,
		Run:              run,
		IsEnabled: func(c *dig.Container) bool {
			return ParamsRestAPI.Enabled
		},
	}
}

var (
	Component *app.Component
	deps      dependencies
	jwtAuth   *jwt.Auth
)

type dependencies struct {
	dig.In
	Echo               *echo.Echo
	Host               host.Host
	RestAPIBindAddress string         `name:"restAPIBindAddress"`
	NodePrivateKey     crypto.PrivKey `name:"nodePrivateKey"`
	RestRouteManager   *RestRouteManager
}

func initConfigParams(c *dig.Container) error {
	type cfgResult struct {
		dig.Out
		RestAPIBindAddress      string `name:"restAPIBindAddress"`
		RestAPILimitsMaxResults int    `name:"restAPILimitsMaxResults"`
	}

	if err := c.Provide(func() cfgResult {
		return cfgResult{
			RestAPIBindAddress:      ParamsRestAPI.BindAddress,
			RestAPILimitsMaxResults: ParamsRestAPI.Limits.MaxResults,
		}
	}); err != nil {
		Component.LogPanic(err)
	}

	return nil
}

func provide(c *dig.Container) error {
	if err := c.Provide(func() *echo.Echo {
		e := httpserver.NewEcho(
			Component.Logger(),
			nil,
			ParamsRestAPI.DebugRequestLoggerEnabled,
		)
		e.Use(middleware.CORS())
		e.Use(middleware.Gzip())
		e.Use(middleware.BodyLimit(ParamsRestAPI.Limits.MaxBodyLength))

		return e
	}); err != nil {
		Component.LogPanic(err)
	}

	type proxyDeps struct {
		dig.In
		Echo *echo.Echo
	}

	if err := c.Provide(func(deps proxyDeps) *RestRouteManager {
		return newRestRouteManager(deps.Echo)
	}); err != nil {
		Component.LogPanic(err)
	}

	return nil
}

func configure() error {
	deps.Echo.Use(apiMiddleware())
	setupRoutes()

	return nil
}

func run() error {
	Component.LogInfo("Starting REST-API server ...")

	if err := Component.Daemon().BackgroundWorker("REST-API server", func(ctx context.Context) {
		Component.LogInfo("Starting REST-API server ... done")

		bindAddr := deps.RestAPIBindAddress

		go func() {
			Component.LogInfof("You can now access the API using: http://%s", bindAddr)
			if err := deps.Echo.Start(bindAddr); err != nil && !errors.Is(err, http.ErrServerClosed) {
				Component.LogWarnf("Stopped REST-API server due to an error (%s)", err)
			}
		}()

		<-ctx.Done()
		Component.LogInfo("Stopping REST-API server ...")

		shutdownCtx, shutdownCtxCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCtxCancel()

		//nolint:contextcheck // false positive
		if err := deps.Echo.Shutdown(shutdownCtx); err != nil {
			Component.LogWarn(err)
		}

		Component.LogInfo("Stopping REST-API server ... done")
	}, daemon.PriorityRestAPI); err != nil {
		Component.LogPanicf("failed to start worker: %s", err)
	}

	return nil
}
