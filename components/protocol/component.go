package protocol

import (
	"context"
	"time"

	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/autopeering/peer"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/timeutil"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/models"
	"github.com/iotaledger/iota-core/pkg/network/p2p"
	"github.com/iotaledger/iota-core/pkg/network/protocols/core"
	"github.com/iotaledger/iota-core/pkg/slot"
)

func init() {
	Component = &app.Component{
		Name:      "Protocol",
		DepsFunc:  func(cDeps dependencies) { deps = cDeps },
		Params:    params,
		Provide:   provide,
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
)

type dependencies struct {
	dig.In

	Peer         *peer.Local
	CoreProtocol *core.Protocol
}

func provide(c *dig.Container) error {

	if err := c.Provide(func() *slot.TimeProvider {
		return slot.NewTimeProvider(1680370391, 10)
	}); err != nil {
		return nil
	}

	return c.Provide(func(p2pManager *p2p.Manager, provider *slot.TimeProvider) *core.Protocol {
		return core.NewProtocol(p2pManager, Component.WorkerPool, provider)
	})
}

func configure() error {
	deps.CoreProtocol.Events.BlockReceived.Hook(func(event *core.BlockReceivedEvent) {
		Component.LogInfof("BlockReceived: %s", event.Block.ID())
	})

	return nil
}

func run() error {
	return Component.Daemon().BackgroundWorker(Component.Name, func(ctx context.Context) {
		issuerKey := lo.PanicOnErr(deps.Peer.Database().LocalPrivateKey())

		ticker := timeutil.NewTicker(func() {
			block := models.NewBlock(
				models.WithStrongParents(models.NewBlockIDs(models.BlockID{})),
				models.WithIssuer(issuerKey.Public()),
			)
			deps.CoreProtocol.SendBlock(block)
		}, 1*time.Second, ctx)
		ticker.WaitForGracefulShutdown()
	}, daemon.PriorityProtocol)
}
