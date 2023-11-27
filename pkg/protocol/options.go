package protocol

import (
	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
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
	iotago "github.com/iotaledger/iota.go/v4"
)

// Options contains the options for the Protocol.
type Options struct {
	// BaseDirectory is the directory where the protocol will store its data.
	BaseDirectory string

	// SnapshotPath is the path to the snapshot file that should be used to initialize the protocol.
	SnapshotPath string

	// ChainSwitchingThreshold is the threshold that defines how far away a heavier chain needs to be from its forking
	// point to be considered for switching.
	ChainSwitchingThreshold iotago.SlotIndex

	// EngineOptions contains the options for the Engines.
	EngineOptions []options.Option[engine.Engine]

	// StorageOptions contains the options for the Storage.
	StorageOptions []options.Option[storage.Storage]

	CommitmentRequesterOptions []options.Option[eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]]

	// FilterProvider contains the provider for the Filter engine modules.
	FilterProvider module.Provider[*engine.Engine, filter.Filter]

	// CommitmentFilterProvider contains the provider for the CommitmentFilter engine modules.
	CommitmentFilterProvider module.Provider[*engine.Engine, commitmentfilter.CommitmentFilter]

	// BlockDAGProvider contains the provider for the BlockDAG engine modules.
	BlockDAGProvider module.Provider[*engine.Engine, blockdag.BlockDAG]

	// TipManagerProvider contains the provider for the TipManager engine modules.
	TipManagerProvider module.Provider[*engine.Engine, tipmanager.TipManager]

	// TipSelectionProvider contains the provider for the TipSelection engine modules.
	TipSelectionProvider module.Provider[*engine.Engine, tipselection.TipSelection]

	// BookerProvider contains the provider for the Booker engine modules.
	BookerProvider module.Provider[*engine.Engine, booker.Booker]

	// ClockProvider contains the provider for the Clock engine modules.
	ClockProvider module.Provider[*engine.Engine, clock.Clock]

	// BlockGadgetProvider contains the provider for the BlockGadget engine modules.
	BlockGadgetProvider module.Provider[*engine.Engine, blockgadget.Gadget]

	// SlotGadgetProvider contains the provider for the SlotGadget engine modules.
	SlotGadgetProvider module.Provider[*engine.Engine, slotgadget.Gadget]

	// SybilProtectionProvider contains the provider for the SybilProtection engine modules.
	SybilProtectionProvider module.Provider[*engine.Engine, sybilprotection.SybilProtection]

	// NotarizationProvider contains the provider for the Notarization engine modules.
	NotarizationProvider module.Provider[*engine.Engine, notarization.Notarization]

	// AttestationProvider contains the provider for the Attestation engine modules.
	AttestationProvider module.Provider[*engine.Engine, attestation.Attestations]

	// SyncManagerProvider contains the provider for the SyncManager engine modules.
	SyncManagerProvider module.Provider[*engine.Engine, syncmanager.SyncManager]

	// LedgerProvider contains the provider for the Ledger engine modules.
	LedgerProvider module.Provider[*engine.Engine, ledger.Ledger]

	// RetainerProvider contains the provider for the Retainer engine modules.
	RetainerProvider module.Provider[*engine.Engine, retainer.Retainer]

	// SchedulerProvider contains the provider for the Scheduler engine modules.
	SchedulerProvider module.Provider[*engine.Engine, scheduler.Scheduler]

	// UpgradeOrchestratorProvider contains the provider for the UpgradeOrchestrator engine modules.
	UpgradeOrchestratorProvider module.Provider[*engine.Engine, upgrade.Orchestrator]
}

// NewDefaultOptions creates new default options instance for the Protocol.
func NewDefaultOptions() *Options {
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

// WithBaseDirectory is an option for the Protocol that allows to set the base directory.
func WithBaseDirectory(baseDirectory string) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.BaseDirectory = baseDirectory
	}
}

// WithSnapshotPath is an option for the Protocol that allows to set the snapshot path.
func WithSnapshotPath(snapshot string) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.SnapshotPath = snapshot
	}
}

// WithChainSwitchingThreshold is an option for the Protocol that allows to set the chain switching threshold.
func WithChainSwitchingThreshold(threshold iotago.SlotIndex) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.ChainSwitchingThreshold = threshold
	}
}

// WithFilterProvider is an option for the Protocol that allows to set the FilterProvider.
func WithFilterProvider(optsFilterProvider module.Provider[*engine.Engine, filter.Filter]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.FilterProvider = optsFilterProvider
	}
}

// WithCommitmentFilterProvider is an option for the Protocol that allows to set the CommitmentFilterProvider.
func WithCommitmentFilterProvider(optsCommitmentFilterProvider module.Provider[*engine.Engine, commitmentfilter.CommitmentFilter]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.CommitmentFilterProvider = optsCommitmentFilterProvider
	}
}

// WithBlockDAGProvider is an option for the Protocol that allows to set the BlockDAGProvider.
func WithBlockDAGProvider(optsBlockDAGProvider module.Provider[*engine.Engine, blockdag.BlockDAG]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.BlockDAGProvider = optsBlockDAGProvider
	}
}

// WithTipManagerProvider is an option for the Protocol that allows to set the TipManagerProvider.
func WithTipManagerProvider(optsTipManagerProvider module.Provider[*engine.Engine, tipmanager.TipManager]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.TipManagerProvider = optsTipManagerProvider
	}
}

// WithTipSelectionProvider is an option for the Protocol that allows to set the TipSelectionProvider.
func WithTipSelectionProvider(optsTipSelectionProvider module.Provider[*engine.Engine, tipselection.TipSelection]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.TipSelectionProvider = optsTipSelectionProvider
	}
}

// WithBookerProvider is an option for the Protocol that allows to set the BookerProvider.
func WithBookerProvider(optsBookerProvider module.Provider[*engine.Engine, booker.Booker]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.BookerProvider = optsBookerProvider
	}
}

// WithClockProvider is an option for the Protocol that allows to set the ClockProvider.
func WithClockProvider(optsClockProvider module.Provider[*engine.Engine, clock.Clock]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.ClockProvider = optsClockProvider
	}
}

// WithSybilProtectionProvider is an option for the Protocol that allows to set the SybilProtectionProvider.
func WithSybilProtectionProvider(optsSybilProtectionProvider module.Provider[*engine.Engine, sybilprotection.SybilProtection]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.SybilProtectionProvider = optsSybilProtectionProvider
	}
}

// WithBlockGadgetProvider is an option for the Protocol that allows to set the BlockGadgetProvider.
func WithBlockGadgetProvider(optsBlockGadgetProvider module.Provider[*engine.Engine, blockgadget.Gadget]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.BlockGadgetProvider = optsBlockGadgetProvider
	}
}

// WithSlotGadgetProvider is an option for the Protocol that allows to set the SlotGadgetProvider.
func WithSlotGadgetProvider(optsSlotGadgetProvider module.Provider[*engine.Engine, slotgadget.Gadget]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.SlotGadgetProvider = optsSlotGadgetProvider
	}
}

// WithEpochGadgetProvider is an option for the Protocol that allows to set the EpochGadgetProvider.
func WithEpochGadgetProvider(optsEpochGadgetProvider module.Provider[*engine.Engine, sybilprotection.SybilProtection]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.SybilProtectionProvider = optsEpochGadgetProvider
	}
}

// WithNotarizationProvider is an option for the Protocol that allows to set the NotarizationProvider.
func WithNotarizationProvider(optsNotarizationProvider module.Provider[*engine.Engine, notarization.Notarization]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.NotarizationProvider = optsNotarizationProvider
	}
}

// WithAttestationProvider is an option for the Protocol that allows to set the AttestationProvider.
func WithAttestationProvider(optsAttestationProvider module.Provider[*engine.Engine, attestation.Attestations]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.AttestationProvider = optsAttestationProvider
	}
}

// WithLedgerProvider is an option for the Protocol that allows to set the LedgerProvider.
func WithLedgerProvider(optsLedgerProvider module.Provider[*engine.Engine, ledger.Ledger]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.LedgerProvider = optsLedgerProvider
	}
}

// WithUpgradeOrchestratorProvider is an option for the Protocol that allows to set the UpgradeOrchestratorProvider.
func WithUpgradeOrchestratorProvider(optsUpgradeOrchestratorProvider module.Provider[*engine.Engine, upgrade.Orchestrator]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.UpgradeOrchestratorProvider = optsUpgradeOrchestratorProvider
	}
}

// WithSyncManagerProvider is an option for the Protocol that allows to set the SyncManagerProvider.
func WithSyncManagerProvider(optsSyncManagerProvider module.Provider[*engine.Engine, syncmanager.SyncManager]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.SyncManagerProvider = optsSyncManagerProvider
	}
}

// WithEngineOptions is an option for the Protocol that allows to set the EngineOptions.
func WithEngineOptions(opts ...options.Option[engine.Engine]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.EngineOptions = append(p.Options.EngineOptions, opts...)
	}
}

// WithStorageOptions is an option for the Protocol that allows to set the StorageOptions.
func WithStorageOptions(opts ...options.Option[storage.Storage]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.StorageOptions = append(p.Options.StorageOptions, opts...)
	}
}
