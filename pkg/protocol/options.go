package protocol

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/attestation"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/clock"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/slotgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/therealledger"
	"github.com/iotaledger/iota-core/pkg/protocol/tipmanager"
	"github.com/iotaledger/iota-core/pkg/storage"
	iotago "github.com/iotaledger/iota.go/v4"
)

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

func WithAttestationProvider(optsAttestationProvider module.Provider[*engine.Engine, attestation.Attestation]) options.Option[Protocol] {
	return func(n *Protocol) {
		n.optsAttestationProvider = optsAttestationProvider
	}
}

func WithLedgerProvider(optsLedgerProvider module.Provider[*engine.Engine, therealledger.Ledger]) options.Option[Protocol] {
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
