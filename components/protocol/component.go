package protocol

import (
	"context"
	"time"

	"go.uber.org/dig"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/autopeering/peer"
	"github.com/iotaledger/hive.go/crypto/identity"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/timeutil"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network/p2p"
	"github.com/iotaledger/iota-core/pkg/network/protocols/core"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
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

	if err := c.Provide(func() *iotago.SlotTimeProvider {
		return iotago.NewSlotTimeProvider(1680370391, 10)
	}); err != nil {
		return nil
	}

	return c.Provide(func(p2pManager *p2p.Manager, provider *iotago.SlotTimeProvider) *core.Protocol {
		// TODO: fill up protocol params
		return core.NewProtocol(p2pManager, Component.WorkerPool, &iotago.ProtocolParameters{}, provider)
	})
}

func configure() error {
	deps.CoreProtocol.Events.BlockReceived.Hook(func(block *model.Block, source identity.ID) {
		Component.LogInfof("BlockReceived: %s", block.ID)
	})

	return nil
}

func run() error {
	return Component.Daemon().BackgroundWorker(Component.Name, func(ctx context.Context) {
		issuerKey := lo.PanicOnErr(deps.Peer.Database().LocalPrivateKey())
		pubKey := issuerKey.Public()
		addr := iotago.Ed25519AddressFromPubKey(pubKey[:])

		ticker := timeutil.NewTicker(func() {
			block, err := builder.NewBlockBuilder().
				StrongParents(iotago.StrongParentsIDs{iotago.BlockID{}}).
				Sign(&addr, issuerKey[:]).
				Build()
			if err != nil {
				Component.LogWarnf("Error building block: %s", err.Error())
				return
			}
			deps.CoreProtocol.SendBlock(block)
		}, 1*time.Second, ctx)
		ticker.WaitForGracefulShutdown()
	}, daemon.PriorityProtocol)
}
