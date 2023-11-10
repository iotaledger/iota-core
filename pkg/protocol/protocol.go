package protocol

import (
	"context"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/network/protocols/core"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Protocol implements the meta-protocol that is responsible for syncing with the heaviest chain.
type Protocol struct {
	Events               *Events
	Workers              *workerpool.Group
	Network              *core.Protocol
	NetworkClock         *NetworkClock
	Commitments          *Commitments
	Chains               *Chains
	BlocksProtocol       *BlocksProtocol
	CommitmentsProtocol  *CommitmentsProtocol
	AttestationsProtocol *AttestationsProtocol
	WarpSyncProtocol     *WarpSyncProtocol
	Engines              *Engines
	Options              *Options

	*APIProvider

	reactive.EvictionState[iotago.SlotIndex]
	*module.ReactiveModule
}

// New creates a new protocol instance.
func New(logger log.Logger, workers *workerpool.Group, networkEndpoint network.Endpoint, opts ...options.Option[Protocol]) *Protocol {
	return options.Apply(&Protocol{
		Events:         NewEvents(),
		Workers:        workers,
		Options:        NewDefaultOptions(),
		ReactiveModule: module.NewReactiveModule(logger),
		EvictionState:  reactive.NewEvictionState[iotago.SlotIndex](),
	}, opts, func(p *Protocol) {
		p.Network = core.NewProtocol(networkEndpoint, workers.CreatePool("NetworkProtocol"), p)
		p.NetworkClock = NewNetworkClock(p)
		p.BlocksProtocol = NewBlocksProtocol(p)
		p.CommitmentsProtocol = NewCommitmentsProtocol(p)
		p.AttestationsProtocol = NewAttestationsProtocol(p)
		p.WarpSyncProtocol = NewWarpSyncProtocol(p)
		p.Commitments = newCommitments(p)
		p.Chains = newChains(p)
		p.Engines = NewEngines(p)
		p.APIProvider = NewAPIProvider(p)

		p.Initialized.OnTrigger(func() {
			unsubscribeFromNetwork := lo.Batch(
				p.Network.OnError(func(err error, peer peer.ID) { p.LogError("network error", "peer", peer, "error", err) }),
				p.Network.OnBlockReceived(p.BlocksProtocol.ProcessResponse),
				p.Network.OnBlockRequestReceived(p.BlocksProtocol.ProcessRequest),
				p.Network.OnCommitmentReceived(p.CommitmentsProtocol.ProcessResponse),
				p.Network.OnCommitmentRequestReceived(p.CommitmentsProtocol.ProcessRequest),
				p.Network.OnAttestationsReceived(p.AttestationsProtocol.ProcessResponse),
				p.Network.OnAttestationsRequestReceived(p.AttestationsProtocol.ProcessRequest),
				p.Network.OnWarpSyncResponseReceived(p.WarpSyncProtocol.ProcessResponse),
				p.Network.OnWarpSyncRequestReceived(p.WarpSyncProtocol.ProcessRequest),
			)

			p.Shutdown.OnTrigger(func() {
				unsubscribeFromNetwork()

				p.BlocksProtocol.Shutdown()
				p.CommitmentsProtocol.Shutdown()
				p.AttestationsProtocol.Shutdown()
				p.WarpSyncProtocol.Shutdown()
				p.Network.Shutdown()
				p.Engines.Shutdown.Trigger()
			})
		})

		p.Constructed.Trigger()

		// wait for the main engine to be initialized
		var waitInitialized sync.WaitGroup
		waitInitialized.Add(1)
		p.Engines.Main.OnUpdateOnce(func(_ *engine.Engine, engine *engine.Engine) {
			engine.Initialized.OnTrigger(waitInitialized.Done)
		})
		waitInitialized.Wait()
	})
}

// IssueBlock issues a block to the node.
func (p *Protocol) IssueBlock(block *model.Block) error {
	p.Network.Events.BlockReceived.Trigger(block, "self")

	return nil
}

// Run starts the protocol.
func (p *Protocol) Run(ctx context.Context) error {
	p.Initialized.Trigger()

	<-ctx.Done()

	p.Shutdown.Trigger()
	p.Workers.Shutdown()
	p.Stopped.Trigger()

	return ctx.Err()
}
