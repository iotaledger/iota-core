package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
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
	"github.com/iotaledger/iota-core/pkg/retainer"
	"github.com/iotaledger/iota-core/pkg/storage"
)

type ProtocolParameters struct {
	storageOptions []options.Option[storage.Storage]
	engineOptions  []options.Option[engine.Engine]

	filterProvider              module.Provider[*engine.Engine, filter.Filter]
	commitmentFilterProvider    module.Provider[*engine.Engine, commitmentfilter.CommitmentFilter]
	blockDAGProvider            module.Provider[*engine.Engine, blockdag.BlockDAG]
	bookerProvider              module.Provider[*engine.Engine, booker.Booker]
	clockProvider               module.Provider[*engine.Engine, clock.Clock]
	blockGadgetProvider         module.Provider[*engine.Engine, blockgadget.Gadget]
	slotGadgetProvider          module.Provider[*engine.Engine, slotgadget.Gadget]
	sybilProtectionProvider     module.Provider[*engine.Engine, sybilprotection.SybilProtection]
	notarizationProvider        module.Provider[*engine.Engine, notarization.Notarization]
	attestationProvider         module.Provider[*engine.Engine, attestation.Attestations]
	ledgerProvider              module.Provider[*engine.Engine, ledger.Ledger]
	schedulerProvider           module.Provider[*engine.Engine, scheduler.Scheduler]
	tipManagerProvider          module.Provider[*engine.Engine, tipmanager.TipManager]
	tipSelectionProvider        module.Provider[*engine.Engine, tipselection.TipSelection]
	retainerProvider            module.Provider[*engine.Engine, retainer.Retainer]
	upgradeOrchestratorProvider module.Provider[*engine.Engine, upgrade.Orchestrator]
}
