package blockissuer

import (
	"context"

	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/iota-core/components/restapi"
	"github.com/iotaledger/iota-core/pkg/blockfactory"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/protocol"
)

func init() {
	Component = &app.Component{
		Name:     "BlockIssuer",
		DepsFunc: func(cDeps dependencies) { deps = cDeps },
		Params:   params,
		Provide:  provide,
		Run:      run,
		IsEnabled: func(_ *dig.Container) bool {
			return ParamsBlockIssuer.Enabled
		},
	}
}

var (
	Component *app.Component
	deps      dependencies
)

type dependencies struct {
	dig.In

	BlockIssuer *blockfactory.BlockIssuer
}

func provide(c *dig.Container) error {
	type innerDependencies struct {
		dig.In

		Protocol *protocol.Protocol
	}

	return c.Provide(func(deps innerDependencies) *blockfactory.BlockIssuer {
		return blockfactory.New(deps.Protocol,
			blockfactory.WithTipSelectionTimeout(ParamsBlockIssuer.TipSelectionTimeout),
			blockfactory.WithTipSelectionRetryInterval(ParamsBlockIssuer.TipSelectionRetryInterval),
			blockfactory.WithIncompleteBlockAccepted(restapi.ParamsRestAPI.AllowIncompleteBlock),
			blockfactory.WithRateSetterEnabled(ParamsBlockIssuer.RateSetterEnabled),
		)
	})
}

func run() error {
	return Component.Daemon().BackgroundWorker(Component.Name, func(ctx context.Context) {
		Component.LogInfof("Starting BlockIssuer")
		<-ctx.Done()
		deps.BlockIssuer.Shutdown()
		Component.LogInfo("Stopping BlockIssuer... done")
	}, daemon.PriorityBlockIssuer)
}
