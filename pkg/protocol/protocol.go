package protocol

import (
	"context"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/network/protocols/core"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
)

// Protocol implements the meta-protocol that is responsible for syncing with the heaviest chain.
type Protocol struct {
	Events               *Events
	Workers              *workerpool.Group
	Network              *core.Protocol
	BlocksProtocol       *BlocksProtocol
	CommitmentsProtocol  *CommitmentsProtocol
	AttestationsProtocol *AttestationsProtocol
	WarpSyncProtocol     *WarpSyncProtocol
	Options              *Options

	*ChainManager
	*EngineManager
	*APIProvider

	log.Logger
	module.Module
}

// New creates a new protocol instance.
func New(logger log.Logger, workers *workerpool.Group, networkEndpoint network.Endpoint, opts ...options.Option[Protocol]) *Protocol {
	return options.Apply(&Protocol{
		Events:  NewEvents(),
		Workers: workers,
		Logger:  logger,
		Options: NewDefaultOptions(),
	}, opts, func(p *Protocol) {
		p.Network = core.NewProtocol(networkEndpoint, workers.CreatePool("NetworkProtocol"), p)
		p.BlocksProtocol = NewBlocksProtocol(p)
		p.CommitmentsProtocol = NewCommitmentsProtocol(p)
		p.AttestationsProtocol = NewAttestationsProtocol(p)
		p.WarpSyncProtocol = NewWarpSyncProtocol(p)
		p.ChainManager = newChainManager(p)
		p.EngineManager = NewEngineManager(p)
		p.APIProvider = NewAPIProvider(p)

		p.HookInitialized(func() {
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

			p.HookShutdown(func() {
				unsubscribeFromNetwork()

				p.BlocksProtocol.Shutdown()
				p.CommitmentsProtocol.Shutdown()
				p.AttestationsProtocol.Shutdown()
				p.WarpSyncProtocol.Shutdown()
				p.Network.Shutdown()
				p.EngineManager.Shutdown()
			})
		})
	}, (*Protocol).TriggerConstructed)
}

// IssueBlock issues a block to the node.
func (p *Protocol) IssueBlock(block *model.Block) error {
	p.BlocksProtocol.ProcessResponse(block, "self")

	return nil
}

// Run starts the protocol.
func (p *Protocol) Run(ctx context.Context) error {
	p.TriggerInitialized()

	<-ctx.Done()

	p.TriggerShutdown()
	p.TriggerStopped()

	p.Workers.Shutdown()

	return ctx.Err()
}

// TriggerInitialized overrides the Module.TriggerInitialized method to only trigger once the main chain and engine were
// initialized.
func (p *Protocol) TriggerInitialized() {
	var waitEngineInitialized sync.WaitGroup

	waitEngineInitialized.Add(1)
	p.MainChain.OnUpdateOnce(func(_, mainChain *Chain) {
		mainChain.Engine.OnUpdateOnce(func(_, engine *engine.Engine) {
			engine.HookInitialized(waitEngineInitialized.Done)
		})
	})
	waitEngineInitialized.Wait()

	p.Module.TriggerInitialized()
}
