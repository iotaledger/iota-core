package activity

import (
	"context"

	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/autopeering/peer"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/timeutil"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/protocol"
	iotago "github.com/iotaledger/iota.go/v4"
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

	Peer     *peer.Local
	Protocol *protocol.Protocol
}

func issuerID() iotago.AccountID {
	issuerKey := lo.PanicOnErr(deps.Peer.Database().LocalPrivateKey())
	pubKey := issuerKey.Public()

	return iotago.AccountID(iotago.Ed25519AddressFromPubKey(pubKey[:]))
}


func run() error {
	return Component.Daemon().BackgroundWorker(Component.Name, func(ctx context.Context) {
		Component.LogInfof("Starting Activity with IssuerID: %s", issuerID().ToHex())
		ticker := timeutil.NewTicker(issueActivityBlock, ParamsActivity.BroadcastInterval, ctx)
		ticker.WaitForGracefulShutdown()

		<-ctx.Done()
		Component.LogInfo("Stopping Activity... done")
	}, daemon.PriorityActivity)
}
