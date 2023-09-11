package protocol

import (
	"context"
	"fmt"
	"time"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
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
	"github.com/iotaledger/iota-core/pkg/protocol/engine/commitmentfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/commitmentfilter/accountsfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/congestioncontrol/scheduler"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/congestioncontrol/scheduler/drr"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget/thresholdblockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/slotgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/slotgadget/totalweightslotgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/blockfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	ledger1 "github.com/iotaledger/iota-core/pkg/protocol/engine/ledger/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/syncmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/syncmanager/trivialsyncmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	tipmanagerv1 "github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager/v1"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipselection"
	tipselectionv1 "github.com/iotaledger/iota-core/pkg/protocol/engine/tipselection/v1"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/upgrade"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/upgrade/signalingupgradeorchestrator"
	"github.com/iotaledger/iota-core/pkg/protocol/enginemanager"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1"
	"github.com/iotaledger/iota-core/pkg/retainer"
	retainer1 "github.com/iotaledger/iota-core/pkg/retainer/retainer"
	"github.com/iotaledger/iota-core/pkg/storage"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type Protocol struct {
	context         context.Context
	Events          *Events
	BlockDispatcher *BlockDispatcher
	engineManager   *enginemanager.EngineManager
	ChainManager    *chainmanager.Manager

	Workers           *workerpool.Group
	networkDispatcher network.Endpoint
	networkProtocol   *core.Protocol

	activeEngineMutex syncutils.RWMutex
	mainEngine        *engine.Engine
	candidateEngine   *candidateEngine

	optsBaseDirectory           string
	optsSnapshotPath            string
	optsChainSwitchingThreshold int

	optsEngineOptions       []options.Option[engine.Engine]
	optsChainManagerOptions []options.Option[chainmanager.Manager]
	optsStorageOptions      []options.Option[storage.Storage]

	optsFilterProvider              module.Provider[*engine.Engine, filter.Filter]
	optsCommitmentFilterProvider    module.Provider[*engine.Engine, commitmentfilter.CommitmentFilter]
	optsBlockDAGProvider            module.Provider[*engine.Engine, blockdag.BlockDAG]
	optsTipManagerProvider          module.Provider[*engine.Engine, tipmanager.TipManager]
	optsTipSelectionProvider        module.Provider[*engine.Engine, tipselection.TipSelection]
	optsBookerProvider              module.Provider[*engine.Engine, booker.Booker]
	optsClockProvider               module.Provider[*engine.Engine, clock.Clock]
	optsBlockGadgetProvider         module.Provider[*engine.Engine, blockgadget.Gadget]
	optsSlotGadgetProvider          module.Provider[*engine.Engine, slotgadget.Gadget]
	optsSybilProtectionProvider     module.Provider[*engine.Engine, sybilprotection.SybilProtection]
	optsNotarizationProvider        module.Provider[*engine.Engine, notarization.Notarization]
	optsAttestationProvider         module.Provider[*engine.Engine, attestation.Attestations]
	optsSyncManagerProvider         module.Provider[*engine.Engine, syncmanager.SyncManager]
	optsLedgerProvider              module.Provider[*engine.Engine, ledger.Ledger]
	optsRetainerProvider            module.Provider[*engine.Engine, retainer.Retainer]
	optsSchedulerProvider           module.Provider[*engine.Engine, scheduler.Scheduler]
	optsUpgradeOrchestratorProvider module.Provider[*engine.Engine, upgrade.Orchestrator]

	optsAttestationRequesterTryInterval time.Duration
	optsAttestationRequesterMaxRetries  int

	module.Module
}

func New(workers *workerpool.Group, dispatcher network.Endpoint, opts ...options.Option[Protocol]) (protocol *Protocol) {
	return options.Apply(&Protocol{
		Events:                          NewEvents(),
		Workers:                         workers,
		networkDispatcher:               dispatcher,
		optsFilterProvider:              blockfilter.NewProvider(),
		optsCommitmentFilterProvider:    accountsfilter.NewProvider(),
		optsBlockDAGProvider:            inmemoryblockdag.NewProvider(),
		optsTipManagerProvider:          tipmanagerv1.NewProvider(),
		optsTipSelectionProvider:        tipselectionv1.NewProvider(),
		optsBookerProvider:              inmemorybooker.NewProvider(),
		optsClockProvider:               blocktime.NewProvider(),
		optsBlockGadgetProvider:         thresholdblockgadget.NewProvider(),
		optsSlotGadgetProvider:          totalweightslotgadget.NewProvider(),
		optsSybilProtectionProvider:     sybilprotectionv1.NewProvider(),
		optsNotarizationProvider:        slotnotarization.NewProvider(),
		optsAttestationProvider:         slotattestation.NewProvider(),
		optsSyncManagerProvider:         trivialsyncmanager.NewProvider(),
		optsLedgerProvider:              ledger1.NewProvider(),
		optsRetainerProvider:            retainer1.NewProvider(),
		optsSchedulerProvider:           drr.NewProvider(),
		optsUpgradeOrchestratorProvider: signalingupgradeorchestrator.NewProvider(),

		optsBaseDirectory:           "",
		optsChainSwitchingThreshold: 3,

		optsAttestationRequesterTryInterval: 3 * time.Second,
		optsAttestationRequesterMaxRetries:  3,
	}, opts, func(p *Protocol) {
		p.BlockDispatcher = NewBlockDispatcher(p)
	}, (*Protocol).initEngineManager, (*Protocol).initChainManager, (*Protocol).initNetworkProtocol, (*Protocol).TriggerConstructed)
}

// Run runs the protocol.
func (p *Protocol) Run(ctx context.Context) error {
	var innerCtxCancel func()

	p.context, innerCtxCancel = context.WithCancel(ctx)
	defer innerCtxCancel()

	p.linkToEngine(p.mainEngine)

	rootCommitment := p.mainEngine.EarliestRootCommitment(p.mainEngine.Storage.Settings().LatestFinalizedSlot())

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

	p.TriggerInitialized()

	<-p.context.Done()

	p.TriggerShutdown()

	p.shutdown()

	p.TriggerStopped()

	return p.context.Err()
}

func (p *Protocol) linkToEngine(engineInstance *engine.Engine) {
	p.Events.Engine.LinkTo(engineInstance.Events)
}

func (p *Protocol) shutdown() {
	if p.networkProtocol != nil {
		p.networkProtocol.Shutdown()
	}

	p.ChainManager.Shutdown()
	p.Workers.Shutdown()

	p.activeEngineMutex.RLock()
	p.mainEngine.Shutdown()
	if p.candidateEngine != nil {
		p.candidateEngine.engine.Shutdown()
	}
	p.activeEngineMutex.RUnlock()
}

func (p *Protocol) initEngineManager() {
	p.engineManager = enginemanager.New(
		p.Workers.CreateGroup("EngineManager"),
		p.HandleError,
		p.optsBaseDirectory,
		DatabaseVersion,
		p.optsStorageOptions,
		p.optsEngineOptions,
		p.optsFilterProvider,
		p.optsCommitmentFilterProvider,
		p.optsBlockDAGProvider,
		p.optsBookerProvider,
		p.optsClockProvider,
		p.optsBlockGadgetProvider,
		p.optsSlotGadgetProvider,
		p.optsSybilProtectionProvider,
		p.optsNotarizationProvider,
		p.optsAttestationProvider,
		p.optsLedgerProvider,
		p.optsSchedulerProvider,
		p.optsTipManagerProvider,
		p.optsTipSelectionProvider,
		p.optsRetainerProvider,
		p.optsUpgradeOrchestratorProvider,
		p.optsSyncManagerProvider,
	)

	mainEngine, err := p.engineManager.LoadActiveEngine(p.optsSnapshotPath)
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
		rootCommitment := p.MainEngineInstance().EarliestRootCommitment(index)

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

func (p *Protocol) IssueBlock(block *model.Block) error {
	return p.BlockDispatcher.Dispatch(block, p.networkDispatcher.LocalPeerID())
}

func (p *Protocol) MainEngineInstance() *engine.Engine {
	p.activeEngineMutex.RLock()
	defer p.activeEngineMutex.RUnlock()

	return p.mainEngine
}

func (p *Protocol) CandidateEngineInstance() *engine.Engine {
	p.activeEngineMutex.RLock()
	defer p.activeEngineMutex.RUnlock()

	if p.candidateEngine == nil {
		return nil
	}

	return p.candidateEngine.engine
}

func (p *Protocol) Network() *core.Protocol {
	return p.networkProtocol
}

func (p *Protocol) LatestAPI() iotago.API {
	return p.MainEngineInstance().LatestAPI()
}

func (p *Protocol) CurrentAPI() iotago.API {
	return p.MainEngineInstance().CurrentAPI()
}

func (p *Protocol) APIForVersion(version iotago.Version) (iotago.API, error) {
	return p.MainEngineInstance().APIForVersion(version)
}

func (p *Protocol) APIForSlot(slot iotago.SlotIndex) iotago.API {
	return p.MainEngineInstance().APIForSlot(slot)
}

func (p *Protocol) APIForEpoch(epoch iotago.EpochIndex) iotago.API {
	return p.MainEngineInstance().APIForEpoch(epoch)
}

func (p *Protocol) HandleError(err error) {
	if err != nil {
		p.Events.Error.Trigger(err)
	}
}

var _ api.Provider = &Protocol{}
