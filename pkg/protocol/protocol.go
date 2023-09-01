package protocol

import (
	"fmt"

	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/attestation"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/clock"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/commitmentfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/congestioncontrol/scheduler"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/slotgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipselection"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/upgrade"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection"
	"github.com/iotaledger/iota-core/pkg/protocol/syncmanager"
	"github.com/iotaledger/iota-core/pkg/retainer"
	"github.com/iotaledger/iota-core/pkg/storage"
)

type Protocol struct {
	optsBaseDirectory string
	optsSnapshotPath  string

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

	*EngineManager
	*ChainManager
}

func NewProtocol(workers *workerpool.Group, dispatcher network.Endpoint, opts ...options.Option[Protocol]) *Protocol {
	return options.Apply(&Protocol{}, opts, func(p *Protocol) {
		p.EngineManager = NewEngineManager(
			p,
			workers.CreateGroup("EngineManager"),
			func(err error) {
				fmt.Println(err)
			},
			p.optsBaseDirectory,
			protocol.DatabaseVersion,
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
		)

		p.ChainManager = NewChainManager(p.EngineManager.MainInstance())
	})
}
