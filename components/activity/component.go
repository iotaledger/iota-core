package activity

import (
	"context"

	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/runtime/timeutil"
	"github.com/iotaledger/iota-core/pkg/blockissuer"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/protocol"
)

func init() {
	Component = &app.Component{
		Name:     "Activity",
		DepsFunc: func(cDeps dependencies) { deps = cDeps },
		Params:   params,
		Run:      run,
		IsEnabled: func(_ *dig.Container) bool {
			return ParamsActivity.Enabled
		},
	}
}

var (
	Component *app.Component
	deps      dependencies
)

type dependencies struct {
	dig.In

	Protocol    *protocol.Protocol
	BlockIssuer *blockissuer.BlockIssuer
}

func run() error {
	return Component.Daemon().BackgroundWorker(Component.Name, func(ctx context.Context) {
		Component.LogInfof("Starting Activity with IssuerID: %s", deps.BlockIssuer.Account.ID())
		ticker := timeutil.NewTicker(func() { issueActivityBlock(ctx) }, ParamsActivity.BroadcastInterval, ctx)
		ticker.WaitForGracefulShutdown()

		<-ctx.Done()
		Component.LogInfo("Stopping Activity... done")
	}, daemon.PriorityActivity)
}
