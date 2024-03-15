package protocol

import (
	"time"

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
	"github.com/iotaledger/iota-core/pkg/protocol/engine/congestioncontrol/scheduler"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/congestioncontrol/scheduler/drr"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget/thresholdblockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/slotgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/slotgadget/totalweightslotgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/postsolidfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/postsolidfilter/postsolidblockfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/presolidfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/presolidfilter/presolidblockfilter"
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
	"github.com/iotaledger/iota-core/pkg/retainer/blockretainer"
	"github.com/iotaledger/iota-core/pkg/retainer/txretainer"
	"github.com/iotaledger/iota-core/pkg/storage"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Options contains the options for the Protocol.
type Options struct {
	// BaseDirectory is the directory where the protocol will store its data.
	BaseDirectory string

	// SnapshotPath is the path to the snapshot file that should be used to initialize the protocol.
	SnapshotPath string

	// CommitmentCheck is an opt flag that allows engines check correctness of commitment and ledger state upon startup.
	CommitmentCheck bool

	// MaxAllowedWallClockDrift specifies how far in the future are blocks allowed to be ahead of our own wall clock (defaults to 0 seconds).
	MaxAllowedWallClockDrift time.Duration

	// EngineOptions contains the options for the Engines.
	EngineOptions []options.Option[engine.Engine]

	// StorageOptions contains the options for the Storage.
	StorageOptions []options.Option[storage.Storage]

	CommitmentRequesterOptions  []options.Option[eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]]
	AttestationRequesterOptions []options.Option[eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]]
	WarpSyncRequesterOptions    []options.Option[eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]]

	// PreSolidFilterProvider contains the provider for the PreSolidFilter engine modules.
	PreSolidFilterProvider module.Provider[*engine.Engine, presolidfilter.PreSolidFilter]

	// PostSolidFilterProvider contains the provider for the PostSolidFilter engine modules.
	PostSolidFilterProvider module.Provider[*engine.Engine, postsolidfilter.PostSolidFilter]

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

	// SyncManagerProvider contains the provider for the SyncManager engine modules.
	SyncManagerProvider module.Provider[*engine.Engine, syncmanager.SyncManager]

	// AttestationProvider contains the provider for the Attestation engine modules.
	AttestationProvider module.Provider[*engine.Engine, attestation.Attestations]

	// LedgerProvider contains the provider for the Ledger engine modules.
	LedgerProvider module.Provider[*engine.Engine, ledger.Ledger]

	// BlockRetainerProvider contains the provider for the BlockRetainer engine modules.
	BlockRetainerProvider module.Provider[*engine.Engine, retainer.BlockRetainer]

	// TransactionRetainerProvider contains the provider for the TransactionRetainer engine modules.
	TransactionRetainerProvider module.Provider[*engine.Engine, retainer.TransactionRetainer]

	// SchedulerProvider contains the provider for the Scheduler engine modules.
	SchedulerProvider module.Provider[*engine.Engine, scheduler.Scheduler]

	// UpgradeOrchestratorProvider contains the provider for the UpgradeOrchestrator engine modules.
	UpgradeOrchestratorProvider module.Provider[*engine.Engine, upgrade.Orchestrator]
}

// NewDefaultOptions creates new default options instance for the Protocol.
func NewDefaultOptions() *Options {
	return &Options{
		BaseDirectory: "",

		PreSolidFilterProvider:      presolidblockfilter.NewProvider(),
		PostSolidFilterProvider:     postsolidblockfilter.NewProvider(),
		BlockDAGProvider:            inmemoryblockdag.NewProvider(),
		TipManagerProvider:          tipmanagerv1.NewProvider(),
		TipSelectionProvider:        tipselectionv1.NewProvider(),
		BookerProvider:              inmemorybooker.NewProvider(),
		ClockProvider:               blocktime.NewProvider(),
		BlockGadgetProvider:         thresholdblockgadget.NewProvider(),
		SlotGadgetProvider:          totalweightslotgadget.NewProvider(),
		SybilProtectionProvider:     sybilprotectionv1.NewProvider(),
		NotarizationProvider:        slotnotarization.NewProvider(),
		SyncManagerProvider:         trivialsyncmanager.NewProvider(),
		AttestationProvider:         slotattestation.NewProvider(),
		LedgerProvider:              ledger1.NewProvider(),
		BlockRetainerProvider:       blockretainer.NewProvider(),
		TransactionRetainerProvider: txretainer.NewProvider(),
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

// WithCommitmentCheck is an option for the Protocol that allows to check the commitment and ledger state upon startup.
func WithCommitmentCheck(commitmentCheck bool) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.CommitmentCheck = commitmentCheck
	}
}

// WithMaxAllowedWallClockDrift specifies how far in the future are blocks allowed to be ahead of our own wall clock (defaults to 0 seconds).
func WithMaxAllowedWallClockDrift(d time.Duration) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.MaxAllowedWallClockDrift = d
	}
}

// WithPreSolidFilterProvider is an option for the Protocol that allows to set the PreSolidFilterProvider.
func WithPreSolidFilterProvider(optsFilterProvider module.Provider[*engine.Engine, presolidfilter.PreSolidFilter]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.PreSolidFilterProvider = optsFilterProvider
	}
}

// WithPostSolidFilterProvider is an option for the Protocol that allows to set the PostSolidFilterProvider.
func WithPostSolidFilterProvider(optsCommitmentFilterProvider module.Provider[*engine.Engine, postsolidfilter.PostSolidFilter]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.PostSolidFilterProvider = optsCommitmentFilterProvider
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

// WithSybilProtectionProvider is an option for the Protocol that allows to set the SybilProtectionProvider.
func WithSybilProtectionProvider(optsSybilProtectionProvider module.Provider[*engine.Engine, sybilprotection.SybilProtection]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.SybilProtectionProvider = optsSybilProtectionProvider
	}
}

// WithNotarizationProvider is an option for the Protocol that allows to set the NotarizationProvider.
func WithNotarizationProvider(optsNotarizationProvider module.Provider[*engine.Engine, notarization.Notarization]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.NotarizationProvider = optsNotarizationProvider
	}
}

// WithSyncManagerProvider is an option for the Protocol that allows to set the SyncManagerProvider.
func WithSyncManagerProvider(optsSyncManagerProvider module.Provider[*engine.Engine, syncmanager.SyncManager]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.SyncManagerProvider = optsSyncManagerProvider
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

// WithBlockRetainerProvider is an option for the Protocol that allows to set the BlockRetainerProvider.
func WithBlockRetainerProvider(optsBlockRetainerProvider module.Provider[*engine.Engine, retainer.BlockRetainer]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.BlockRetainerProvider = optsBlockRetainerProvider
	}
}

// WithTransactionRetainerProvider is an option for the Protocol that allows to set the TransactionRetainerProvider.
func WithTransactionRetainerProvider(optsTransactionRetainerProvider module.Provider[*engine.Engine, retainer.TransactionRetainer]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.TransactionRetainerProvider = optsTransactionRetainerProvider
	}
}

// WithSchedulerProvider is an option for the Protocol that allows to set the SchedulerProvider.
func WithSchedulerProvider(optsSchedulerProvider module.Provider[*engine.Engine, scheduler.Scheduler]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.SchedulerProvider = optsSchedulerProvider
	}
}

// WithUpgradeOrchestratorProvider is an option for the Protocol that allows to set the UpgradeOrchestratorProvider.
func WithUpgradeOrchestratorProvider(optsUpgradeOrchestratorProvider module.Provider[*engine.Engine, upgrade.Orchestrator]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.UpgradeOrchestratorProvider = optsUpgradeOrchestratorProvider
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

func WithCommitmentRequesterOptions(opts ...options.Option[eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.CommitmentRequesterOptions = append(p.Options.CommitmentRequesterOptions, opts...)
	}
}

func WithAttestationRequesterOptions(opts ...options.Option[eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.AttestationRequesterOptions = append(p.Options.AttestationRequesterOptions, opts...)
	}
}

func WithWarpSyncRequesterOptions(opts ...options.Option[eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]]) options.Option[Protocol] {
	return func(p *Protocol) {
		p.Options.WarpSyncRequesterOptions = append(p.Options.WarpSyncRequesterOptions, opts...)
	}
}
