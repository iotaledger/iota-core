package inx

import (
	"context"

	"github.com/labstack/echo/v4"
	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/iota-core/components/protocol"
	"github.com/iotaledger/iota-core/pkg/blockfactory"
	"github.com/iotaledger/iota-core/pkg/daemon"
	protocolpkg "github.com/iotaledger/iota-core/pkg/protocol"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
)

func init() {
	Component = &app.Component{
		Name:     "INX",
		DepsFunc: func(cDeps dependencies) { deps = cDeps },
		Params:   params,
		IsEnabled: func(c *dig.Container) bool {
			return ParamsINX.Enabled
		},
		Provide:   provide,
		Configure: configure,
		Run:       run,
	}
}

var (
	Component          *app.Component
	deps               dependencies
	blockIssuerAccount blockfactory.Account
)

type dependencies struct {
	dig.In
	Protocol         *protocolpkg.Protocol
	BlockIssuer      *blockfactory.BlockIssuer
	Echo             *echo.Echo `optional:"true"`
	RestRouteManager *restapipkg.RestRouteManager
	INXServer        *Server
	BaseToken        *protocol.BaseToken
}

func provide(c *dig.Container) error {
	if err := c.Provide(func() *Server {
		return newServer()
	}); err != nil {
		Component.LogPanic(err)
	}

	return nil
}

func configure() error {
	blockIssuerAccount = blockfactory.AccountFromParams(ParamsINX.BlockIssuerAccount, ParamsINX.BlockIssuerPrivateKey)

	return nil
}

func run() error {
	if err := Component.Daemon().BackgroundWorker("INX", func(ctx context.Context) {
		Component.LogInfo("Starting INX ... done")
		deps.INXServer.Start()
		<-ctx.Done()
		Component.LogInfo("Stopping INX ...")
		deps.INXServer.Stop()
		Component.LogInfo("Stopping INX ... done")
	}, daemon.PriorityINX); err != nil {
		Component.LogPanicf("failed to start worker: %s", err)
	}

	return nil
}
