package protocol

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/network/protocols/core"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag/inmemoryblockdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker/inmemorybooker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/clock"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/clock/blocktime"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget/thresholdblockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/slotgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/slotgadget/totalweightslotgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/blockfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger/utxoledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection/poa"
	"github.com/iotaledger/iota-core/pkg/protocol/enginemanager"
	"github.com/iotaledger/iota-core/pkg/protocol/tipmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/tipmanager/trivialtipmanager"
	"github.com/iotaledger/iota-core/pkg/storage"
	iotago "github.com/iotaledger/iota.go/v4"
)

// region Protocol /////////////////////////////////////////////////////////////////////////////////////////////////////

type Protocol struct {
	Events        *Events
	TipManager    tipmanager.TipManager
	engineManager *enginemanager.EngineManager
	ChainManager  *chainmanager.Manager

	Workers         *workerpool.Group
	dispatcher      network.Endpoint
	networkProtocol *core.Protocol

	mainEngine *engine.Engine

	optsBaseDirectory string
	optsSnapshotPath  string
	optsPruningDelay  iotago.SlotIndex

	optsEngineOptions       []options.Option[engine.Engine]
	optsChainManagerOptions []options.Option[chainmanager.Manager]
	optsStorageOptions      []options.Option[storage.Storage]

	optsFilterProvider          module.Provider[*engine.Engine, filter.Filter]
	optsBlockDAGProvider        module.Provider[*engine.Engine, blockdag.BlockDAG]
	optsTipManagerProvider      module.Provider[*engine.Engine, tipmanager.TipManager]
	optsBookerProvider          module.Provider[*engine.Engine, booker.Booker]
	optsClockProvider           module.Provider[*engine.Engine, clock.Clock]
	optsSybilProtectionProvider module.Provider[*engine.Engine, sybilprotection.SybilProtection]
	optsBlockGadgetProvider     module.Provider[*engine.Engine, blockgadget.Gadget]
	optsSlotGadgetProvider      module.Provider[*engine.Engine, slotgadget.Gadget]
	optsNotarizationProvider    module.Provider[*engine.Engine, notarization.Notarization]
	optsLedgerProvider          module.Provider[*engine.Engine, ledger.Ledger]
}

func New(workers *workerpool.Group, dispatcher network.Endpoint, opts ...options.Option[Protocol]) (protocol *Protocol) {
	return options.Apply(&Protocol{
		Events:                      NewEvents(),
		Workers:                     workers,
		dispatcher:                  dispatcher,
		optsFilterProvider:          blockfilter.NewProvider(),
		optsBlockDAGProvider:        inmemoryblockdag.NewProvider(),
		optsTipManagerProvider:      trivialtipmanager.NewProvider(),
		optsBookerProvider:          inmemorybooker.NewProvider(),
		optsClockProvider:           blocktime.NewProvider(),
		optsSybilProtectionProvider: poa.NewProvider(map[iotago.AccountID]int64{}),
		optsBlockGadgetProvider:     thresholdblockgadget.NewProvider(),
		optsSlotGadgetProvider:      totalweightslotgadget.NewProvider(),
		optsNotarizationProvider:    slotnotarization.NewProvider(),
		optsLedgerProvider:          utxoledger.NewProvider(),

		optsBaseDirectory: "",
		optsPruningDelay:  360,
	}, opts,
		(*Protocol).initNetworkEvents,
		(*Protocol).initEngineManager,
		(*Protocol).initChainManager,
	)
}

// Run runs the protocol.
func (p *Protocol) Run() {
	p.Events.Engine.LinkTo(p.mainEngine.Events)
	p.TipManager = p.optsTipManagerProvider(p.mainEngine)
	p.Events.TipManager.LinkTo(p.TipManager.Events())

	if err := p.mainEngine.Initialize(p.optsSnapshotPath); err != nil {
		panic(err)
	}

	rootCommitment := p.mainEngine.EarliestRootCommitment()

	// The root commitment is the earliest commitment we will ever need to know to solidify commitment chains, we can
	// then initialize the chain manager with it, and identify our engine to be on such chain.
	// Upon engine restart, such chain will be loaded with the latest finalized slot, and the chain manager, not needing
	// persistent storage, will be able to continue from there.
	p.mainEngine.SetChainID(rootCommitment.ID())
	p.ChainManager.Initialize(rootCommitment)

	// Fill the chain manager with all our known commitments so that the chain is solid
	for i := rootCommitment.Index(); i <= p.mainEngine.Storage.Settings().LatestCommitment().Index(); i++ {
		if cm, err := p.mainEngine.Storage.Commitments().Load(i); err == nil {
			p.ChainManager.ProcessCommitment(cm)
		}
	}

	// p.linkTo(p.mainEngine) -> CC and TipManager
	// TODO: why do we create a protocol only when running?
	// TODO: fill up protocol params
	p.networkProtocol = core.NewProtocol(p.dispatcher, p.Workers.CreatePool("NetworkProtocol"), p.API()) // Use max amount of workers for networking
	p.Events.Network.LinkTo(p.networkProtocol.Events)
}

func (p *Protocol) Shutdown() {
	if p.networkProtocol != nil {
		p.networkProtocol.Shutdown()
	}

	p.Workers.Shutdown()
	p.mainEngine.Shutdown()
	p.ChainManager.Shutdown()
	p.TipManager.Shutdown()
}

func (p *Protocol) initNetworkEvents() {
	wpBlocks := p.Workers.CreatePool("NetworkEvents.Blocks") // Use max amount of workers for sending, receiving and requesting blocks

	p.Events.Network.BlockReceived.Hook(func(block *model.Block, id network.PeerID) {
		if err := p.ProcessBlock(block, id); err != nil {
			p.Events.Error.Trigger(err)
		}
	}, event.WithWorkerPool(wpBlocks))

	p.Events.Network.BlockRequestReceived.Hook(func(blockID iotago.BlockID, id network.PeerID) {
		if block, exists := p.MainEngineInstance().Block(blockID); exists {
			p.networkProtocol.SendBlock(block, id)
		}
	}, event.WithWorkerPool(wpBlocks))

	p.Events.Engine.BlockRequester.Tick.Hook(func(blockID iotago.BlockID) {
		p.networkProtocol.RequestBlock(blockID)
	}, event.WithWorkerPool(wpBlocks))

	p.Events.Engine.BlockDAG.BlockSolid.Hook(func(block *blocks.Block) {
		p.networkProtocol.SendBlock(block.ModelBlock())
	}, event.WithWorkerPool(wpBlocks))

	wpCommitments := p.Workers.CreatePool("NetworkEvents.SlotCommitments")

	p.Events.Network.SlotCommitmentRequestReceived.Hook(func(commitmentID iotago.CommitmentID, source network.PeerID) {
		// when we receive a commitment request, do not look it up in the ChainManager but in the storage, else we might answer with commitments we did not issue ourselves and for which we cannot provide attestations
		if requestedCommitment, err := p.MainEngineInstance().Storage.Commitments().Load(commitmentID.Index()); err == nil && requestedCommitment.ID() == commitmentID {
			p.networkProtocol.SendSlotCommitment(requestedCommitment, source)
		}
	}, event.WithWorkerPool(wpCommitments))

	p.Events.Network.SlotCommitmentReceived.Hook(func(commitment *model.Commitment, source network.PeerID) {
		p.ChainManager.ProcessCommitmentFromSource(commitment, source)
	}, event.WithWorkerPool(wpCommitments))

	p.Events.ChainManager.RequestCommitment.Hook(func(commitmentID iotago.CommitmentID) {
		p.networkProtocol.RequestCommitment(commitmentID)
	}, event.WithWorkerPool(wpCommitments))
}

func (p *Protocol) initEngineManager() {
	p.engineManager = enginemanager.New(
		p.Workers.CreateGroup("EngineManager"),
		p.optsBaseDirectory,
		DatabaseVersion,
		p.optsStorageOptions,
		p.optsEngineOptions,
		p.optsFilterProvider,
		p.optsBlockDAGProvider,
		p.optsBookerProvider,
		p.optsClockProvider,
		p.optsSybilProtectionProvider,
		p.optsBlockGadgetProvider,
		p.optsSlotGadgetProvider,
		p.optsNotarizationProvider,
		p.optsLedgerProvider,
	)

	p.Events.Engine.SlotGadget.SlotFinalized.Hook(func(index iotago.SlotIndex) {
		// TODO: fix pruning
		if index < p.optsPruningDelay {
			return
		}
		p.MainEngineInstance().Storage.PruneUntilSlot(index - p.optsPruningDelay)
	}, event.WithWorkerPool(p.Workers.CreatePool("PruneEngine", 2)))

	mainEngine, err := p.engineManager.LoadActiveEngine()
	if err != nil {
		panic(fmt.Sprintf("could not load active engine: %s", err))
	}
	p.mainEngine = mainEngine
}

func (p *Protocol) initChainManager() {
	p.ChainManager = chainmanager.NewManager(p.optsChainManagerOptions...)
	p.Events.ChainManager.LinkTo(p.ChainManager.Events)

	wp := p.Workers.CreatePool("ChainManager", 1) // Using just 1 worker to avoid contention

	p.Events.Engine.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
		p.ChainManager.ProcessCommitment(details.Commitment)
	}, event.WithWorkerPool(wp))

	p.Events.Engine.SlotGadget.SlotFinalized.Hook(func(index iotago.SlotIndex) {
		rootCommitment := p.MainEngineInstance().EarliestRootCommitment()

		// It is essential that we set the rootCommitment before evicting the chainManager's state, this way
		// we first specify the chain's cut-off point, and only then evict the state. It is also important to
		// note that no multiple goroutines should be allowed to perform this operation at once, hence the
		// hooking worker pool should always have a single worker or these two calls should be protected by a lock.
		p.ChainManager.SetRootCommitment(rootCommitment)

		// We want to evict just below the height of our new root commitment (so that the slot of the root commitment
		// stays in memory storage and with it the root commitment itself as well).
		if rootCommitment.ID().Index() > 0 {
			p.ChainManager.EvictUntil(rootCommitment.ID().Index() - 1)
		}
	}, event.WithWorkerPool(wp))

	p.Events.ChainManager.ForkDetected.Hook(p.onForkDetected, event.WithWorkerPool(wp))
}

func (p *Protocol) ProcessOwnBlock(block *model.Block) error {
	return p.ProcessBlock(block, p.dispatcher.LocalPeerID())
}

func (p *Protocol) ProcessBlock(block *model.Block, src network.PeerID) error {
	mainEngine := p.MainEngineInstance()

	if !mainEngine.WasInitialized() {
		return errors.Errorf("protocol engine not yet initialized")
	}

	isSolid, chain := p.ChainManager.ProcessCommitmentFromSource(block.SlotCommitment(), src)
	if !isSolid {
		if block.Block().SlotCommitment.PrevID == mainEngine.Storage.Settings().LatestCommitment().ID() {
			return nil
		}

		return errors.Errorf("protocol ProcessBlock failed. chain is not solid: %s, latest commitment: %s, block ID: %s", block.Block().SlotCommitment.MustID(), mainEngine.Storage.Settings().LatestCommitment().ID(), block.ID())
	}

	processed := false

	if mainChain := mainEngine.ChainID(); chain.ForkingPoint.ID() == mainChain || mainEngine.BlockRequester.HasTicker(block.ID()) {
		mainEngine.ProcessBlockFromPeer(block, src)
		processed = true
	}

	// if candidateEngine := p.CandidateEngineInstance(); candidateEngine != nil {
	//	if candidateChain := candidateEngine.Storage.Settings.ChainID(); chain.ForkingPoint.ID() == candidateChain || candidateEngine.BlockRequester.HasTicker(block.ID()) {
	//		candidateEngine.ProcessBlockFromPeer(block, src)
	//		if candidateEngine.IsBootstrapped() &&
	//			candidateEngine.Storage.Settings.LatestCommitment().Index() >= mainEngine.Storage.Settings.LatestCommitment().Index() &&
	//			candidateEngine.Storage.Settings.LatestCommitment().CumulativeWeight() > mainEngine.Storage.Settings.LatestCommitment().CumulativeWeight() {
	//			p.switchEngines()
	//		}
	//		processed = true
	//	}
	// }

	if !processed {
		return errors.Errorf("block from source %s was not processed: %s; commits to: %s", src, block.ID(), block.Block().SlotCommitment.MustID())
	}

	return nil
}

func (p *Protocol) MainEngineInstance() *engine.Engine {
	return p.mainEngine
}

func (p *Protocol) Network() *core.Protocol {
	return p.networkProtocol
}

func (p *Protocol) API() iotago.API {
	return p.MainEngineInstance().API()
}

func (p *Protocol) onForkDetected(fork *chainmanager.Fork) {
	fmt.Printf("================================================================\nFork detected: %s\n================================================================\n", fork)
}

func WithBaseDirectory(baseDirectory string) options.Option[Protocol] {
	return func(n *Protocol) {
		n.optsBaseDirectory = baseDirectory
	}
}

func WithPruningDelay(pruningDelay iotago.SlotIndex) options.Option[Protocol] {
	return func(n *Protocol) {
		n.optsPruningDelay = pruningDelay
	}
}

func WithSnapshotPath(snapshot string) options.Option[Protocol] {
	return func(n *Protocol) {
		n.optsSnapshotPath = snapshot
	}
}

func WithFilterProvider(optsFilterProvider module.Provider[*engine.Engine, filter.Filter]) options.Option[Protocol] {
	return func(n *Protocol) {
		n.optsFilterProvider = optsFilterProvider
	}
}

func WithBlockDAGProvider(optsBlockDAGProvider module.Provider[*engine.Engine, blockdag.BlockDAG]) options.Option[Protocol] {
	return func(n *Protocol) {
		n.optsBlockDAGProvider = optsBlockDAGProvider
	}
}

func WithTipManagerProvider(optsTipManagerProvider module.Provider[*engine.Engine, tipmanager.TipManager]) options.Option[Protocol] {
	return func(n *Protocol) {
		n.optsTipManagerProvider = optsTipManagerProvider
	}
}

func WithBookerProvider(optsBookerProvider module.Provider[*engine.Engine, booker.Booker]) options.Option[Protocol] {
	return func(n *Protocol) {
		n.optsBookerProvider = optsBookerProvider
	}
}

func WithClockProvider(optsClockProvider module.Provider[*engine.Engine, clock.Clock]) options.Option[Protocol] {
	return func(n *Protocol) {
		n.optsClockProvider = optsClockProvider
	}
}

func WithSybilProtectionProvider(optsSybilProtectionProvider module.Provider[*engine.Engine, sybilprotection.SybilProtection]) options.Option[Protocol] {
	return func(n *Protocol) {
		n.optsSybilProtectionProvider = optsSybilProtectionProvider
	}
}

func WithBlockGadgetProvider(optsBlockGadgetProvider module.Provider[*engine.Engine, blockgadget.Gadget]) options.Option[Protocol] {
	return func(n *Protocol) {
		n.optsBlockGadgetProvider = optsBlockGadgetProvider
	}
}

func WithSlotGadgetProvider(optsSlotGadgetProvider module.Provider[*engine.Engine, slotgadget.Gadget]) options.Option[Protocol] {
	return func(n *Protocol) {
		n.optsSlotGadgetProvider = optsSlotGadgetProvider
	}
}

func WithNotarizationProvider(optsNotarizationProvider module.Provider[*engine.Engine, notarization.Notarization]) options.Option[Protocol] {
	return func(n *Protocol) {
		n.optsNotarizationProvider = optsNotarizationProvider
	}
}

func WithLedgerProvider(optsLedgerProvider module.Provider[*engine.Engine, ledger.Ledger]) options.Option[Protocol] {
	return func(n *Protocol) {
		n.optsLedgerProvider = optsLedgerProvider
	}
}

func WithEngineOptions(opts ...options.Option[engine.Engine]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.optsEngineOptions = append(p.optsEngineOptions, opts...)
	}
}

func WithChainManagerOptions(opts ...options.Option[chainmanager.Manager]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.optsChainManagerOptions = append(p.optsChainManagerOptions, opts...)
	}
}

func WithStorageOptions(opts ...options.Option[storage.Storage]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.optsStorageOptions = append(p.optsStorageOptions, opts...)
	}
}
