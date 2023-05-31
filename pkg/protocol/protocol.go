package protocol

import (
	"fmt"
	"sync"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/lo"
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
	"github.com/iotaledger/iota-core/pkg/protocol/syncmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/syncmanager/trivialsyncmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/tipmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/tipmanager/trivialtipmanager"
	"github.com/iotaledger/iota-core/pkg/storage"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Protocol struct {
	Events        *Events
	TipManager    tipmanager.TipManager
	SyncManager   syncmanager.SyncManager
	engineManager *enginemanager.EngineManager
	ChainManager  *chainmanager.Manager

	Workers         *workerpool.Group
	dispatcher      network.Endpoint
	networkProtocol *core.Protocol

	activeEngineMutex sync.RWMutex
	mainEngine        *engine.Engine
	candidateEngine   *engine.Engine

	optsBaseDirectory string
	optsSnapshotPath  string

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
		optsTipManagerProvider:      trivialtipmanager.NewProvider(),
		optsBookerProvider:          inmemorybooker.NewProvider(),
		optsClockProvider:           blocktime.NewProvider(),
		optsSybilProtectionProvider: poa.NewProvider(map[iotago.AccountID]int64{}),
		optsBlockGadgetProvider:     thresholdblockgadget.NewProvider(),
		optsSlotGadgetProvider:      totalweightslotgadget.NewProvider(),
		optsNotarizationProvider:    slotnotarization.NewProvider(slotnotarization.DefaultMinSlotCommittableAge),
		optsAttestationProvider:     slotattestation.NewProvider(slotattestation.DefaultAttestationCommitmentOffset),
		optsSyncManagerProvider:     trivialsyncmanager.NewProvider(),
		optsLedgerProvider:          utxoledger.NewProvider(),

		optsBaseDirectory: "",
	}, opts,
		(*Protocol).initEngineManager,
		(*Protocol).initChainManager,
	)
}

// Run runs the protocol.
func (p *Protocol) Run() {
	p.linkToEngine(p.mainEngine)

	if err := p.mainEngine.Initialize(p.optsSnapshotPath); err != nil {
		panic(err)
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
}

func (p *Protocol) linkToEngine(engineInstance *engine.Engine) {
	if p.TipManager != nil {
		p.TipManager.Shutdown()
		p.TipManager = nil
	}

	if p.SyncManager != nil {
		p.SyncManager.Shutdown()
		p.SyncManager = nil
	}

	p.Events.Engine.LinkTo(engineInstance.Events)

	p.TipManager = p.optsTipManagerProvider(engineInstance)
	p.Events.TipManager.LinkTo(p.TipManager.Events())

	p.SyncManager = p.optsSyncManagerProvider(engineInstance)
}

func (p *Protocol) Shutdown() {
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
	p.TipManager.Shutdown()
	p.SyncManager.Shutdown()
}

func (p *Protocol) runNetworkProtocol() {
	p.networkProtocol = core.NewProtocol(p.dispatcher, p.Workers.CreatePool("NetworkProtocol"), p.API()) // Use max amount of workers for networking
	p.Events.Network.LinkTo(p.networkProtocol.Events)

	wpBlocks := p.Workers.CreatePool("NetworkEvents.Blocks") // Use max amount of workers for sending, receiving and requesting blocks

	p.Events.Network.BlockReceived.Hook(func(block *model.Block, id network.PeerID) {
		if err := p.ProcessBlock(block, id); err != nil {
			p.ErrorHandler()(err)
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
		p.networkProtocol.RequestSlotCommitment(commitmentID)
	}, event.WithWorkerPool(wpCommitments))

	wpAttestations := p.Workers.CreatePool("NetworkEvents.Attestations", 1) // Using just 1 worker to avoid contention

	p.Events.Network.AttestationsRequestReceived.Hook(p.ProcessAttestationsRequest, event.WithWorkerPool(wpAttestations))
	p.Events.Network.AttestationsReceived.Hook(p.ProcessAttestations, event.WithWorkerPool(wpAttestations))
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

	wp := p.Workers.CreatePool("ChainManager", 1) // Using just 1 worker to avoid contention

	p.Events.Engine.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
		p.ChainManager.ProcessCommitment(details.Commitment)
	}, event.WithWorkerPool(wp))

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

	if candidateEngine := p.CandidateEngineInstance(); candidateEngine != nil {
		if candidateChain := candidateEngine.ChainID(); chain.ForkingPoint.ID() == candidateChain || candidateEngine.BlockRequester.HasTicker(block.ID()) {
			candidateEngine.ProcessBlockFromPeer(block, src)
			if candidateEngine.IsBootstrapped() &&
				candidateEngine.Storage.Settings().LatestCommitment().Index() >= mainEngine.Storage.Settings().LatestCommitment().Index() &&
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

func (p *Protocol) ProcessAttestationsRequest(commitmentID iotago.CommitmentID, src network.PeerID) {
	mainEngine := p.MainEngineInstance()

	if mainEngine.Storage.Settings().LatestCommitment().Index() < commitmentID.Index() {
		return
	}

	commitment, err := mainEngine.Storage.Commitments().Load(commitmentID.Index())
	if err != nil {
		return
	}

	if commitment.ID() != commitmentID {
		return
	}

	store, err := mainEngine.Attestation.Get(commitmentID.Index())
	if err != nil {
		return
	}

	attestations := make([]*iotago.Attestation, 0)
	if err := store.Stream(func(_ iotago.AccountID, attestation *iotago.Attestation) bool {
		attestations = append(attestations, attestation)

		return true
	}); err != nil {
		return
	}

	p.networkProtocol.SendAttestations(commitment, attestations, src)
}

func (p *Protocol) ProcessAttestations(commitment *model.Commitment, attestations []*iotago.Attestation, src network.PeerID) {
	if len(attestations) == 0 {
		p.Events.Error.Trigger(errors.Errorf("received attestations from peer %s are empty", src.String()))
		return
	}

	fork, exists := p.ChainManager.ForkByForkingPoint(commitment.ID())
	if !exists {
		p.Events.Error.Trigger(errors.Errorf("failed to get forking point for commitment %s", commitment.ID()))
		return
	}

	mainEngine := p.MainEngineInstance()

	snapshotTargetIndex := fork.ForkingPoint.Index() - 1

	snapshotTargetCommitment, err := mainEngine.Storage.Commitments().Load(snapshotTargetIndex)
	if err != nil {
		p.Events.Error.Trigger(errors.Wrapf(err, "failed to get commitment at snapshotTargetIndex %s", snapshotTargetIndex))
		return
	}

	calculatedCumulativeWeight := snapshotTargetCommitment.CumulativeWeight()

	visitedIdentities := make(map[iotago.AccountID]types.Empty)
	var blockIDs iotago.BlockIDs
	for _, att := range attestations {
		if valid, err := att.VerifySignature(); !valid {
			if err != nil {
				p.Events.Error.Trigger(errors.Wrapf(err, "error validating attestation signature provided by %s", src))
				return
			}

			p.Events.Error.Trigger(errors.Errorf("invalid attestation signature provided by %s", src))
			return
		}

		if _, alreadyVisited := visitedIdentities[att.IssuerID]; alreadyVisited {
			p.Events.Error.Trigger(errors.Errorf("invalid attestation from source %s, issuerID %s contains multiple attestations", src, att.IssuerID))
			// TODO: ban source!
			return
		}

		// TODO: this might differ if we have a Accounts with changing weights depending on the SlotIndex
		if weight, weightExists := mainEngine.SybilProtection.Accounts().Get(att.IssuerID); weightExists {
			calculatedCumulativeWeight += uint64(weight)
		}

		visitedIdentities[att.IssuerID] = types.Void

		blockID, err := att.BlockID(mainEngine.API().SlotTimeProvider())
		if err != nil {
			p.Events.Error.Trigger(errors.Wrapf(err, "error calculating blockID from attestation"))
			return
		}

		blockIDs = append(blockIDs, blockID)
	}

	// Compare the calculated cumulative weight with ours to verify it is really higher
	weightAtForkedEventEnd := lo.PanicOnErr(mainEngine.Storage.Commitments().Load(fork.Commitment.Index())).CumulativeWeight()
	if calculatedCumulativeWeight <= weightAtForkedEventEnd {
		forkedEventClaimedWeight := fork.Commitment.CumulativeWeight()
		forkedEventMainWeight := lo.PanicOnErr(mainEngine.Storage.Commitments().Load(fork.Commitment.Index())).CumulativeWeight()
		p.Events.Error.Trigger(errors.Errorf("fork at point %d does not accumulate enough weight at slot %d calculated %d CW <= main chain %d CW. fork event detected at %d was %d CW > %d CW",
			fork.ForkingPoint.Index(),
			fork.Commitment.Index(),
			calculatedCumulativeWeight,
			weightAtForkedEventEnd,
			fork.Commitment.Index(),
			forkedEventClaimedWeight,
			forkedEventMainWeight))
		// TODO: ban source?
		return
	}

	candidateEngine, err := p.engineManager.ForkEngineAtSlot(snapshotTargetIndex)
	if err != nil {
		p.Events.Error.Trigger(errors.Wrap(err, "error creating new candidate engine"))
		return
	}

	// Set the chain to the correct forking point
	candidateEngine.SetChainID(fork.ForkingPoint.ID())

	// Attach the engine block requests to the protocol and detach as soon as we switch to that engine
	wp := candidateEngine.Workers.CreatePool("CandidateBlockRequester", 2)
	detachRequestBlocks := candidateEngine.Events.BlockRequester.Tick.Hook(func(blockID iotago.BlockID) {
		p.networkProtocol.RequestBlock(blockID)
	}, event.WithWorkerPool(wp)).Unhook

	// Attach slot commitments to the chain manager and detach as soon as we switch to that engine
	detachProcessCommitment := candidateEngine.Events.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
		p.ChainManager.ProcessCandidateCommitment(details.Commitment)
	}, event.WithWorkerPool(candidateEngine.Workers.CreatePool("ProcessCandidateCommitment", 2))).Unhook

	p.Events.MainEngineSwitched.Hook(func(_ *engine.Engine) {
		detachRequestBlocks()
		detachProcessCommitment()
	}, event.WithMaxTriggerCount(1))

	// Add all the blocks from the forking point attestations to the requester since those will not be passed to the engine by the protocol
	candidateEngine.BlockRequester.StartTickers(blockIDs)

	// Set the engine as the new candidate
	p.activeEngineMutex.Lock()
	oldCandidateEngine := p.candidateEngine
	p.candidateEngine = candidateEngine
	p.activeEngineMutex.Unlock()

	p.Events.CandidateEngineActivated.Trigger(candidateEngine)

	if oldCandidateEngine != nil {
		oldCandidateEngine.Shutdown()
		if err := oldCandidateEngine.RemoveFromFilesystem(); err != nil {
			p.Events.Error.Trigger(errors.Wrap(err, "error cleaning up replaced candidate engine from file system"))
		}
	}

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

func (p *Protocol) onForkDetected(fork *chainmanager.Fork) {
	claimedWeight := fork.Commitment.CumulativeWeight()
	mainChainCommitment, err := p.MainEngineInstance().Storage.Commitments().Load(fork.Commitment.Index())
	if err != nil {
		p.Events.Error.Trigger(errors.Errorf("failed to load commitment for main chain tip at index %d", fork.Commitment.Index()))
		return
	}

	mainChainWeight := mainChainCommitment.CumulativeWeight()

	if claimedWeight <= mainChainWeight {
		// TODO: ban source?
		p.Events.Error.Trigger(errors.Errorf("dot not process fork with %d CW <= than main chain %d CW received from %s", claimedWeight, mainChainWeight, fork.Source))
		return
	}

	p.networkProtocol.RequestAttestations(fork.ForkingPoint.ID(), fork.Source)
}

func (p *Protocol) switchEngines() {
	p.activeEngineMutex.Lock()

	if p.candidateEngine == nil {
		p.activeEngineMutex.Unlock()
		return
	}

	// Try to re-org the chain manager
	if err := p.ChainManager.SwitchMainChain(p.candidateEngine.Storage.Settings().LatestCommitment().ID()); err != nil {
		p.activeEngineMutex.Unlock()
		p.Events.Error.Trigger(errors.Wrap(err, "switching main chain failed"))
		return
	}

	if err := p.engineManager.SetActiveInstance(p.candidateEngine); err != nil {
		p.activeEngineMutex.Unlock()
		p.Events.Error.Trigger(errors.Wrap(err, "error switching engines"))
		return
	}

	//// Stop current Scheduler
	//p.CongestionControl.Shutdown()

	p.linkToEngine(p.candidateEngine)

	// Save a reference to the current main engine and storage so that we can shut it down and prune it after switching
	oldEngine := p.mainEngine
	oldEngine.Shutdown()

	p.mainEngine = p.candidateEngine
	p.candidateEngine = nil

	p.activeEngineMutex.Unlock()

	p.Events.MainEngineSwitched.Trigger(p.MainEngineInstance())

	// TODO: copy over old slots from the old engine to the new one

	// Cleanup filesystem
	if err := oldEngine.RemoveFromFilesystem(); err != nil {
		p.Events.Error.Trigger(errors.Wrap(err, "error removing storage directory after switching engines"))
	}
}
