package protocol

import (
	"github.com/iotaledger/hive.go/crypto/identity"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/database"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/network/protocols/core"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag/inmemoryblockdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker/inmemorybooker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/clock"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/clock/blocktime"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget/tresholdblockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/blockfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection/poa"
	"github.com/iotaledger/iota-core/pkg/protocol/enginemanager"
	"github.com/iotaledger/iota-core/pkg/protocol/tipmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/tipmanager/trivialtipmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

// region Protocol /////////////////////////////////////////////////////////////////////////////////////////////////////

type Protocol struct {
	Events        *Events
	engineManager *enginemanager.EngineManager
	TipManager    tipmanager.TipManager

	Workers         *workerpool.Group
	dispatcher      network.Endpoint
	networkProtocol *core.Protocol

	mainEngine *engine.Engine

	optsBaseDirectory    string
	optsSnapshotPath     string
	optsPruningThreshold uint64

	optsEngineOptions                 []options.Option[engine.Engine]
	optsStorageDatabaseManagerOptions []options.Option[database.Manager]

	optsFilterProvider          module.Provider[*engine.Engine, filter.Filter]
	optsBlockDAGProvider        module.Provider[*engine.Engine, blockdag.BlockDAG]
	optsTipManagerProvider      module.Provider[*engine.Engine, tipmanager.TipManager]
	optsBookerProvider          module.Provider[*engine.Engine, booker.Booker]
	optsClockProvider           module.Provider[*engine.Engine, clock.Clock]
	optsSybilProtectionProvider module.Provider[*engine.Engine, sybilprotection.SybilProtection]
	optsBlockGadgetProvider     module.Provider[*engine.Engine, blockgadget.Gadget]
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
		optsSybilProtectionProvider: poa.NewProvider(map[identity.ID]int64{}),
		optsBlockGadgetProvider:     tresholdblockgadget.NewProvider(),

		optsBaseDirectory:    "",
		optsPruningThreshold: 6 * 60, // 1 hour given that slot duration is 10 seconds
	}, opts,
		(*Protocol).initNetworkEvents,
		(*Protocol).initEngineManager,
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

	// p.linkTo(p.mainEngine) -> CC and TipManager
	// TODO: why do we create a protocol only when running?
	// TODO: fill up protocol params
	p.networkProtocol = core.NewProtocol(p.dispatcher, p.Workers.CreatePool("NetworkProtocol"), p.API()) // Use max amount of workers for networking
	p.Events.Network.LinkTo(p.networkProtocol.Events)
}

func (p *Protocol) Shutdown() {
	if p.networkProtocol != nil {
		p.networkProtocol.Unregister()
	}

	p.TipManager.Shutdown()

	p.mainEngine.Shutdown()

	p.Workers.Shutdown()
}

func (p *Protocol) initNetworkEvents() {
	wpBlocks := p.Workers.CreatePool("NetworkEvents.Blocks") // Use max amount of workers for sending, receiving and requesting blocks

	p.Events.Network.BlockReceived.Hook(func(block *model.Block, id identity.ID) {
		if err := p.ProcessBlock(block, id); err != nil {
			p.Events.Error.Trigger(err)
		}
	}, event.WithWorkerPool(wpBlocks))
	p.Events.Network.BlockRequestReceived.Hook(func(blockID iotago.BlockID, id identity.ID) {
		if block, exists := p.MainEngineInstance().Block(blockID); exists && !block.IsMissing() && !block.IsRootBlock() {
			p.networkProtocol.SendBlock(block.Block(), id)
		}
	}, event.WithWorkerPool(wpBlocks))
	p.Events.Engine.BlockRequester.Tick.Hook(func(blockID iotago.BlockID) {
		p.networkProtocol.RequestBlock(blockID)
	}, event.WithWorkerPool(wpBlocks))

	p.Events.Engine.BlockDAG.BlockSolid.Hook(func(block *blocks.Block) {
		p.networkProtocol.SendBlock(block.Block())
	}, event.WithWorkerPool(wpBlocks))
}

func (p *Protocol) initEngineManager() {
	p.engineManager = enginemanager.New(
		p.Workers.CreateGroup("EngineManager"),
		p.optsBaseDirectory,
		DatabaseVersion,
		p.optsStorageDatabaseManagerOptions,
		p.optsEngineOptions,
		p.optsFilterProvider,
		p.optsBlockDAGProvider,
		p.optsBookerProvider,
		p.optsClockProvider,
		p.optsSybilProtectionProvider,
		p.optsBlockGadgetProvider,
	)

	mainEngine, err := p.engineManager.LoadActiveEngine()
	if err != nil {
		panic(err)
	}
	p.mainEngine = mainEngine
}

func (p *Protocol) ProcessBlock(block *model.Block, src identity.ID) error {
	mainEngine := p.MainEngineInstance()

	mainEngine.ProcessBlockFromPeer(block, src)

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

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region Options //////////////////////////////////////////////////////////////////////////////////////////////////////

func WithBaseDirectory(baseDirectory string) options.Option[Protocol] {
	return func(n *Protocol) {
		n.optsBaseDirectory = baseDirectory
	}
}

func WithPruningThreshold(pruningThreshold uint64) options.Option[Protocol] {
	return func(n *Protocol) {
		n.optsPruningThreshold = pruningThreshold
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

func WithEngineOptions(opts ...options.Option[engine.Engine]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.optsEngineOptions = append(p.optsEngineOptions, opts...)
	}
}

func WithStorageDatabaseManagerOptions(opts ...options.Option[database.Manager]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.optsStorageDatabaseManagerOptions = append(p.optsStorageDatabaseManagerOptions, opts...)
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
