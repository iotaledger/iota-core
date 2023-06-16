package protocol

import (
	"context"
	"fmt"
	"sync"

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
	"github.com/iotaledger/iota-core/pkg/protocol/engine/attestation"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/attestation/slotattestation"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag/inmemoryblockdag"
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
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	tipmanagerv1 "github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager/v1"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipselection"
	tipselectionv1 "github.com/iotaledger/iota-core/pkg/protocol/engine/tipselection/v1"
	"github.com/iotaledger/iota-core/pkg/protocol/enginemanager"
	"github.com/iotaledger/iota-core/pkg/protocol/syncmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/syncmanager/trivialsyncmanager"
	"github.com/iotaledger/iota-core/pkg/storage"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Protocol struct {
	context       context.Context
	Events        *Events
	SyncManager   syncmanager.SyncManager
	engineManager *enginemanager.EngineManager
	ChainManager  *chainmanager.Manager

	Workers         *workerpool.Group
	dispatcher      network.Endpoint
	networkProtocol *core.Protocol

	activeEngineMutex sync.RWMutex
	mainEngine        *engine.Engine
	candidateEngine   *engine.Engine

	optsBaseDirectory           string
	optsSnapshotPath            string
	optsChainSwitchingThreshold int

	optsEngineOptions       []options.Option[engine.Engine]
	optsChainManagerOptions []options.Option[chainmanager.Manager]
	optsStorageOptions      []options.Option[storage.Storage]

	optsFilterProvider          module.Provider[*engine.Engine, filter.Filter]
	optsBlockDAGProvider        module.Provider[*engine.Engine, blockdag.BlockDAG]
	optsTipManagerProvider      module.Provider[*engine.Engine, tipmanager.TipManager]
	optsTipSelectionProvider    module.Provider[*engine.Engine, tipselection.TipSelection]
	optsBookerProvider          module.Provider[*engine.Engine, booker.Booker]
	optsClockProvider           module.Provider[*engine.Engine, clock.Clock]
	optsSybilProtectionProvider module.Provider[*engine.Engine, sybilprotection.SybilProtection]
	optsBlockGadgetProvider     module.Provider[*engine.Engine, blockgadget.Gadget]
	optsSlotGadgetProvider      module.Provider[*engine.Engine, slotgadget.Gadget]
	optsNotarizationProvider    module.Provider[*engine.Engine, notarization.Notarization]
	optsAttestationProvider     module.Provider[*engine.Engine, attestation.Attestations]
	optsSyncManagerProvider     module.Provider[*engine.Engine, syncmanager.SyncManager]
	optsLedgerProvider          module.Provider[*engine.Engine, ledger.Ledger]
}

func New(workers *workerpool.Group, dispatcher network.Endpoint, opts ...options.Option[Protocol]) (protocol *Protocol) {
	return options.Apply(&Protocol{
		Events:                      NewEvents(),
		Workers:                     workers,
		dispatcher:                  dispatcher,
		optsFilterProvider:          blockfilter.NewProvider(),
		optsBlockDAGProvider:        inmemoryblockdag.NewProvider(),
		optsTipManagerProvider:      tipmanagerv1.NewProvider(),
		optsTipSelectionProvider:    tipselectionv1.NewProvider(),
		optsBookerProvider:          inmemorybooker.NewProvider(),
		optsClockProvider:           blocktime.NewProvider(),
		optsSybilProtectionProvider: poa.NewProvider(map[iotago.AccountID]int64{}),
		optsBlockGadgetProvider:     thresholdblockgadget.NewProvider(),
		optsSlotGadgetProvider:      totalweightslotgadget.NewProvider(),
		optsNotarizationProvider:    slotnotarization.NewProvider(slotnotarization.DefaultMinSlotCommittableAge),
		optsAttestationProvider:     slotattestation.NewProvider(slotattestation.DefaultAttestationCommitmentOffset),
		optsSyncManagerProvider:     trivialsyncmanager.NewProvider(),
		optsLedgerProvider:          utxoledger.NewProvider(),

		optsBaseDirectory:           "",
		optsChainSwitchingThreshold: 3,
	}, opts,
		(*Protocol).initEngineManager,
		(*Protocol).initChainManager,
	)
}

// Run runs the protocol.
func (p *Protocol) Run(ctx context.Context) error {
	var innerCtxCancel func()

	p.context, innerCtxCancel = context.WithCancel(ctx)
	defer innerCtxCancel()

	p.linkToEngine(p.mainEngine)

	//nolint:contextcheck // false positive
	if err := p.mainEngine.Initialize(p.optsSnapshotPath); err != nil {
		return errors.Wrapf(err, "mainEngine initialize failed")
	}

	rootCommitment, valid := p.mainEngine.EarliestRootCommitment(p.mainEngine.Storage.Settings().LatestFinalizedSlot())
	if !valid {
		panic("no root commitment found")
	}

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

	p.runNetworkProtocol()

	p.Events.Started.Trigger()

	<-p.context.Done()

	p.shutdown()

	p.Events.Stopped.Trigger()

	return p.context.Err()
}

func (p *Protocol) linkToEngine(engineInstance *engine.Engine) {
	if p.SyncManager != nil {
		p.SyncManager.Shutdown()
		p.SyncManager = nil
	}
	p.SyncManager = p.optsSyncManagerProvider(engineInstance)

	p.Events.Engine.LinkTo(engineInstance.Events)
}

func (p *Protocol) shutdown() {
	if p.networkProtocol != nil {
		p.networkProtocol.Shutdown()
	}

	p.Workers.Shutdown()

	p.activeEngineMutex.RLock()
	p.mainEngine.Shutdown()
	if p.candidateEngine != nil {
		p.candidateEngine.Shutdown()
	}
	p.activeEngineMutex.RUnlock()

	p.ChainManager.Shutdown()
	p.SyncManager.Shutdown()
}

func (p *Protocol) initEngineManager() {
	p.engineManager = enginemanager.New(
		p.Workers.CreateGroup("EngineManager"),
		p.ErrorHandler(),
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
		p.optsAttestationProvider,
		p.optsLedgerProvider,
		p.optsTipManagerProvider,
		p.optsTipSelectionProvider,
	)

	mainEngine, err := p.engineManager.LoadActiveEngine()
	if err != nil {
		panic(fmt.Sprintf("could not load active engine: %s", err))
	}
	p.mainEngine = mainEngine
}

func (p *Protocol) initChainManager() {
	p.ChainManager = chainmanager.NewManager(p.optsChainManagerOptions...)
	p.Events.ChainManager.LinkTo(p.ChainManager.Events)

	// This needs to be hooked so that the ChainManager always knows the commitments we issued.
	// Else our own BlockIssuer might use a commitment that the ChainManager does not know yet.
	p.Events.Engine.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
		p.ChainManager.ProcessCommitment(details.Commitment)
	})

	p.Events.Engine.SlotGadget.SlotFinalized.Hook(func(index iotago.SlotIndex) {
		rootCommitment, valid := p.MainEngineInstance().EarliestRootCommitment(index)
		fmt.Println("Slot finalized:", index, "root commitment:", rootCommitment, "valid:", valid)
		if !valid {
			return
		}

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
	})

	wpForking := p.Workers.CreatePool("Protocol.Forking", 1) // Using just 1 worker to avoid contention
	p.Events.ChainManager.ForkDetected.Hook(p.onForkDetected, event.WithWorkerPool(wpForking))
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

	if candidateEngine := p.CandidateEngineInstance(); candidateEngine != nil {
		if candidateChain := candidateEngine.ChainID(); chain.ForkingPoint.ID() == candidateChain || candidateEngine.BlockRequester.HasTicker(block.ID()) {
			candidateEngine.ProcessBlockFromPeer(block, src)
			if candidateEngine.IsBootstrapped() &&
				candidateEngine.Storage.Settings().LatestCommitment().CumulativeWeight() > mainEngine.Storage.Settings().LatestCommitment().CumulativeWeight() {
				p.switchEngines()
			}
			processed = true
		}
	}

	if !processed {
		return errors.Errorf("block from source %s was not processed: %s; commits to: %s", src, block.ID(), block.Block().SlotCommitment.MustID())
	}

	return nil
}

func (p *Protocol) MainEngineInstance() *engine.Engine {
	p.activeEngineMutex.RLock()
	defer p.activeEngineMutex.RUnlock()

	return p.mainEngine
}

func (p *Protocol) CandidateEngineInstance() *engine.Engine {
	p.activeEngineMutex.RLock()
	defer p.activeEngineMutex.RUnlock()

	return p.candidateEngine
}

func (p *Protocol) Network() *core.Protocol {
	return p.networkProtocol
}

func (p *Protocol) API() iotago.API {
	return p.MainEngineInstance().API()
}

func (p *Protocol) SupportedVersions() Versions {
	return SupportedVersions
}

func (p *Protocol) ErrorHandler() func(error) {
	return func(err error) {
		p.Events.Error.Trigger(err)
	}
}
