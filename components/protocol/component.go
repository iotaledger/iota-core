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
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network/p2p"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag"
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

	Peer     *peer.Local
	Protocol *protocol.Protocol
}

func provide(c *dig.Container) error {
	return c.Provide(func(p2pManager *p2p.Manager) *protocol.Protocol {
		return protocol.New(
			workerpool.NewGroup("Protocol"),
			p2pManager,

			protocol.WithBaseDirectory(ParamsDatabase.Directory),
			protocol.WithSnapshotPath(ParamsProtocol.Snapshot.Path),
		)
	})
}

func configure() error {
	deps.Protocol.Events.Error.Hook(func(err error) {
		Component.LogErrorf("Error in Protocol: %s", err)
	})
	// TODO: forward engine errors to protocol?
	deps.Protocol.Events.Engine.Error.Hook(func(err error) {
		Component.LogErrorf("Error in Engine: %s", err)
	})

	deps.Protocol.Events.Network.BlockReceived.Hook(func(block *model.Block, source identity.ID) {
		Component.LogInfof("BlockReceived: %s", block.ID())
	})

	deps.Protocol.Events.Engine.BlockDAG.BlockSolid.Hook(func(block *blockdag.Block) {
		Component.LogInfof("BlockSolid: %s", block.ID())
	})

	deps.Protocol.Events.Engine.BlockDAG.BlockInvalid.Hook(func(event *blockdag.BlockInvalidEvent) {
		Component.LogInfof("BlockInvalid: %s: %s", event.Block.ID(), event.Reason)
	})

	return nil
}

func run() error {
	return Component.Daemon().BackgroundWorker(Component.Name, func(ctx context.Context) {
		deps.Protocol.Run()

		// TODO: remove dummy issuance
		issuerKey := lo.PanicOnErr(deps.Peer.Database().LocalPrivateKey())
		pubKey := issuerKey.Public()
		addr := iotago.Ed25519AddressFromPubKey(pubKey[:])

		ticker := timeutil.NewTicker(func() {
			block, err := builder.NewBlockBuilder().
				StrongParents(deps.Protocol.TipManager.Tips(iotago.BlockMaxParents)).
				Sign(&addr, issuerKey[:]).
				Build()
			if err != nil {
				Component.LogWarnf("Error building block: %s", err.Error())
				return
			}
			modelBlock, err := model.BlockFromBlock(block, deps.Protocol.API())
			if err != nil {
				Component.LogWarnf("Error creating model.Block from block: %s", err.Error())
			}

			err = deps.Protocol.ProcessBlock(modelBlock, deps.Peer.ID())
			if err != nil {
				Component.LogWarnf("Error processing block in Protocol: %s", err.Error())
			}
		}, 1*time.Second, ctx)
		ticker.WaitForGracefulShutdown()

		<-ctx.Done()
		Component.LogInfo("Gracefully shutting down the Protocol...")
		deps.Protocol.Shutdown()
	}, daemon.PriorityProtocol)
}
