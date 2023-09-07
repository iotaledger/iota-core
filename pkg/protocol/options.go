package protocol

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
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
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1"
	"github.com/iotaledger/iota-core/pkg/retainer"
	retainer1 "github.com/iotaledger/iota-core/pkg/retainer/retainer"
	"github.com/iotaledger/iota-core/pkg/storage"
)

type Options struct {
	BaseDirectory           string
	SnapshotPath            string
	ChainSwitchingThreshold int

	EngineOptions       []options.Option[engine.Engine]
	ChainManagerOptions []options.Option[chainmanager.Manager]
	StorageOptions      []options.Option[storage.Storage]

	FilterProvider              module.Provider[*engine.Engine, filter.Filter]
	CommitmentFilterProvider    module.Provider[*engine.Engine, commitmentfilter.CommitmentFilter]
	BlockDAGProvider            module.Provider[*engine.Engine, blockdag.BlockDAG]
	TipManagerProvider          module.Provider[*engine.Engine, tipmanager.TipManager]
	TipSelectionProvider        module.Provider[*engine.Engine, tipselection.TipSelection]
	BookerProvider              module.Provider[*engine.Engine, booker.Booker]
	ClockProvider               module.Provider[*engine.Engine, clock.Clock]
	BlockGadgetProvider         module.Provider[*engine.Engine, blockgadget.Gadget]
	SlotGadgetProvider          module.Provider[*engine.Engine, slotgadget.Gadget]
	SybilProtectionProvider     module.Provider[*engine.Engine, sybilprotection.SybilProtection]
	NotarizationProvider        module.Provider[*engine.Engine, notarization.Notarization]
	AttestationProvider         module.Provider[*engine.Engine, attestation.Attestations]
	SyncManagerProvider         module.Provider[*engine.Engine, syncmanager.SyncManager]
	LedgerProvider              module.Provider[*engine.Engine, ledger.Ledger]
	RetainerProvider            module.Provider[*engine.Engine, retainer.Retainer]
	SchedulerProvider           module.Provider[*engine.Engine, scheduler.Scheduler]
	UpgradeOrchestratorProvider module.Provider[*engine.Engine, upgrade.Orchestrator]
}

func newOptions() *Options {
	return &Options{
		BaseDirectory:           "",
		ChainSwitchingThreshold: 3,

		FilterProvider:              blockfilter.NewProvider(),
		CommitmentFilterProvider:    accountsfilter.NewProvider(),
		BlockDAGProvider:            inmemoryblockdag.NewProvider(),
		TipManagerProvider:          tipmanagerv1.NewProvider(),
		TipSelectionProvider:        tipselectionv1.NewProvider(),
		BookerProvider:              inmemorybooker.NewProvider(),
		ClockProvider:               blocktime.NewProvider(),
		BlockGadgetProvider:         thresholdblockgadget.NewProvider(),
		SlotGadgetProvider:          totalweightslotgadget.NewProvider(),
		SybilProtectionProvider:     sybilprotectionv1.NewProvider(),
		NotarizationProvider:        slotnotarization.NewProvider(),
		AttestationProvider:         slotattestation.NewProvider(),
		SyncManagerProvider:         trivialsyncmanager.NewProvider(),
		LedgerProvider:              ledger1.NewProvider(),
		RetainerProvider:            retainer1.NewProvider(),
		SchedulerProvider:           drr.NewProvider(),
		UpgradeOrchestratorProvider: signalingupgradeorchestrator.NewProvider(),
	}
}

func WithBaseDirectory(baseDirectory string) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.BaseDirectory = baseDirectory
	}
}

func WithSnapshotPath(snapshot string) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.SnapshotPath = snapshot
	}
}

func WithChainSwitchingThreshold(threshold int) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.ChainSwitchingThreshold = threshold
	}
}

func WithFilterProvider(optsFilterProvider module.Provider[*engine.Engine, filter.Filter]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.FilterProvider = optsFilterProvider
	}
}

func WithCommitmentFilterProvider(optsCommitmentFilterProvider module.Provider[*engine.Engine, commitmentfilter.CommitmentFilter]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.CommitmentFilterProvider = optsCommitmentFilterProvider
	}
}

func WithBlockDAGProvider(optsBlockDAGProvider module.Provider[*engine.Engine, blockdag.BlockDAG]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.BlockDAGProvider = optsBlockDAGProvider
	}
}

func WithTipManagerProvider(optsTipManagerProvider module.Provider[*engine.Engine, tipmanager.TipManager]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.TipManagerProvider = optsTipManagerProvider
	}
}

func WithTipSelectionProvider(optsTipSelectionProvider module.Provider[*engine.Engine, tipselection.TipSelection]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.TipSelectionProvider = optsTipSelectionProvider
	}
}

func WithBookerProvider(optsBookerProvider module.Provider[*engine.Engine, booker.Booker]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.BookerProvider = optsBookerProvider
	}
}

func WithClockProvider(optsClockProvider module.Provider[*engine.Engine, clock.Clock]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.ClockProvider = optsClockProvider
	}
}

func WithSybilProtectionProvider(optsSybilProtectionProvider module.Provider[*engine.Engine, sybilprotection.SybilProtection]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.SybilProtectionProvider = optsSybilProtectionProvider
	}
}

func WithBlockGadgetProvider(optsBlockGadgetProvider module.Provider[*engine.Engine, blockgadget.Gadget]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.BlockGadgetProvider = optsBlockGadgetProvider
	}
}

func WithSlotGadgetProvider(optsSlotGadgetProvider module.Provider[*engine.Engine, slotgadget.Gadget]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.SlotGadgetProvider = optsSlotGadgetProvider
	}
}

func WithEpochGadgetProvider(optsEpochGadgetProvider module.Provider[*engine.Engine, sybilprotection.SybilProtection]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.SybilProtectionProvider = optsEpochGadgetProvider
	}
}

func WithNotarizationProvider(optsNotarizationProvider module.Provider[*engine.Engine, notarization.Notarization]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.NotarizationProvider = optsNotarizationProvider
	}
}

func WithAttestationProvider(optsAttestationProvider module.Provider[*engine.Engine, attestation.Attestations]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.AttestationProvider = optsAttestationProvider
	}
}

func WithLedgerProvider(optsLedgerProvider module.Provider[*engine.Engine, ledger.Ledger]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.LedgerProvider = optsLedgerProvider
	}
}

func WithUpgradeOrchestratorProvider(optsUpgradeOrchestratorProvider module.Provider[*engine.Engine, upgrade.Orchestrator]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.UpgradeOrchestratorProvider = optsUpgradeOrchestratorProvider
	}
}

func WithEngineOptions(opts ...options.Option[engine.Engine]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.EngineOptions = append(p.options.EngineOptions, opts...)
	}
}

func WithChainManagerOptions(opts ...options.Option[chainmanager.Manager]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.ChainManagerOptions = append(p.options.ChainManagerOptions, opts...)
	}
}

func WithStorageOptions(opts ...options.Option[storage.Storage]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.options.StorageOptions = append(p.options.StorageOptions, opts...)
	}
}
