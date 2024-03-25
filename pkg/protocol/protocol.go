package protocol

import (
	"context"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Protocol is an implementation of the IOTA core protocol.
type Protocol struct {
	// Events contains a centralized access point for all events that are triggered by the main engine of the protocol.
	Events *Events

	// Workers contains the worker pools that are used by the protocol.
	Workers *workerpool.Group

	// Network contains the network endpoint of the protocol.
	Network *Network

	// Commitments contains the commitments that are managed by the protocol.
	Commitments *Commitments

	// Chains contains the chains that are managed by the protocol.
	Chains *Chains

	// Blocks contains the subcomponent that is responsible for handling block requests and responses.
	Blocks *Blocks

	// Attestations contains the subcomponent that is responsible for handling attestation requests and responses.
	Attestations *Attestations

	// WarpSync contains the subcomponent that is responsible for handling warp sync requests and responses.
	WarpSync *WarpSync

	// Engines contains the engines that are managed by the protocol.
	Engines *Engines

	// Options contains the options that were used to create the protocol.
	Options *Options

	// EvictionState contains the eviction state of the protocol.
	reactive.EvictionState[iotago.SlotIndex]

	// ReactiveModule embeds the reactive module logic of the protocol.
	module.Module
}

// New creates a new protocol instance from the given parameters.
func New(logger log.Logger, workers *workerpool.Group, networkEndpoint network.Endpoint, opts ...options.Option[Protocol]) *Protocol {
	return options.Apply(&Protocol{
		Events:        NewEvents(),
		Workers:       workers,
		Options:       NewDefaultOptions(),
		Module:        module.New(logger),
		EvictionState: reactive.NewEvictionState[iotago.SlotIndex](),
	}, opts, func(p *Protocol) {
		shutdownSubComponents := p.initSubcomponents(networkEndpoint)

		p.InitializedEvent().OnTrigger(func() {
			shutdown := lo.BatchReverse(
				p.initEviction(),
				p.initGlobalEventsRedirection(),
				p.initNetwork(),

				shutdownSubComponents,

				logger.Shutdown,
			)

			p.ShutdownEvent().OnTrigger(shutdown)
		})

		p.ConstructedEvent().Trigger()

		p.waitInitialized()
	})
}

// IssueBlock issues a block to the node.
func (p *Protocol) IssueBlock(block *model.Block) error {
	p.Network.Events.BlockReceived.Trigger(block, "self")

	return nil
}

// Run starts the protocol.
func (p *Protocol) Run(ctx context.Context) error {
	p.InitializedEvent().Trigger()

	<-ctx.Done()

	p.ShutdownEvent().Trigger()
	p.StoppedEvent().Trigger()

	return ctx.Err()
}

// APIForVersion returns the API for the given version.
func (p *Protocol) APIForVersion(version iotago.Version) (api iotago.API, err error) {
	if mainEngineInstance := p.Engines.Main.Get(); mainEngineInstance != nil {
		return mainEngineInstance.APIForVersion(version)
	}

	return nil, ierrors.New("no engine instance available")
}

// APIForSlot returns the API for the given slot.
func (p *Protocol) APIForSlot(slot iotago.SlotIndex) iotago.API {
	return p.Engines.Main.Get().APIForSlot(slot)
}

// APIForEpoch returns the API for the given epoch.
func (p *Protocol) APIForEpoch(epoch iotago.EpochIndex) iotago.API {
	return p.Engines.Main.Get().APIForEpoch(epoch)
}

// APIForTime returns the API for the given time.
func (p *Protocol) APIForTime(t time.Time) iotago.API {
	return p.Engines.Main.Get().APIForTime(t)
}

// CommittedAPI returns the API for the committed state.
func (p *Protocol) CommittedAPI() iotago.API {
	return p.Engines.Main.Get().CommittedAPI()
}

// LatestAPI returns the latest API.
func (p *Protocol) LatestAPI() iotago.API {
	return p.Engines.Main.Get().LatestAPI()
}

// initSubcomponents initializes the subcomponents of the protocol and returns a function that shuts them down.
func (p *Protocol) initSubcomponents(networkEndpoint network.Endpoint) (shutdown func()) {
	p.Network = newNetwork(p, networkEndpoint)
	p.Blocks = newBlocks(p)
	p.Attestations = newAttestations(p)
	p.WarpSync = newWarpSync(p)
	p.Commitments = newCommitments(p)
	p.Chains = newChains(p)
	p.Engines = newEngines(p)

	return func() {
		p.Blocks.Shutdown()
		p.WarpSync.Shutdown()
		p.Network.Protocol.Shutdown()
		p.Workers.WaitChildren()
		p.Engines.ShutdownEvent().Trigger()
		p.Workers.Shutdown()
	}
}

// initEviction initializes the eviction of old data when the engine advances and returns a function that shuts it down.
func (p *Protocol) initEviction() (shutdown func()) {
	evictionWorker := p.Workers.CreatePool("Eviction").Start()

	return p.Commitments.Root.OnUpdate(func(_ *Commitment, rootCommitment *Commitment) {
		if rootSlot := rootCommitment.Slot(); rootSlot > 0 {
			evictionWorker.Submit(func() {
				p.Evict(rootSlot - 1)
			})
		}
	})
}

// initGlobalEventsRedirection initializes the global events redirection of the protocol and returns a function that
// shuts it down.
func (p *Protocol) initGlobalEventsRedirection() (shutdown func()) {
	return p.Engines.Main.WithNonEmptyValue(func(mainEngine *engine.Engine) (shutdown func()) {
		p.Events.Engine.LinkTo(mainEngine.Events)

		return func() {
			p.Events.Engine.LinkTo(nil)
		}
	})
}

// initNetwork initializes the network of the protocol and returns a function that shuts it down.
func (p *Protocol) initNetwork() (shutdown func()) {
	return lo.BatchReverse(
		p.Network.OnError(func(err error, peer peer.ID) { p.LogError("network error", "peer", peer, "error", err) }),
		p.Network.OnBlockReceived(p.Blocks.ProcessResponse),
		p.Network.OnBlockRequestReceived(p.Blocks.ProcessRequest),
		p.Network.OnCommitmentReceived(p.Commitments.processResponse),
		p.Network.OnCommitmentRequestReceived(p.Commitments.processRequest),
		p.Network.OnAttestationsReceived(p.Attestations.processResponse),
		p.Network.OnAttestationsRequestReceived(p.Attestations.processRequest),
		p.Network.OnWarpSyncResponseReceived(p.WarpSync.ProcessResponse),
		p.Network.OnWarpSyncRequestReceived(p.WarpSync.ProcessRequest),
	)
}

// waitInitialized waits until the main engine is initialized (published its root commitment).
func (p *Protocol) waitInitialized() {
	var waitInitialized sync.WaitGroup

	waitInitialized.Add(1)
	p.Commitments.Root.OnUpdateOnce(func(_ *Commitment, _ *Commitment) {
		waitInitialized.Done()
	}, func(_ *Commitment, rootCommitment *Commitment) bool { return rootCommitment != nil })

	waitInitialized.Wait()
}
