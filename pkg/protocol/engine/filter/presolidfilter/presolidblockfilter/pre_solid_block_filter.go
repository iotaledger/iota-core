package presolidblockfilter

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/presolidfilter"
	iotago "github.com/iotaledger/iota.go/v4"
)

var (
	ErrValidatorNotInCommittee = ierrors.New("validation block issuer is not in the committee")
	ErrInvalidBlockVersion     = ierrors.New("block has invalid protocol version")
)

// PreSolidBlockFilter filters blocks.
type PreSolidBlockFilter struct {
	events *presolidfilter.Events

	apiProvider iotago.APIProvider

	committeeFunc func(iotago.SlotIndex) (*account.SeatedAccounts, bool)

	module.Module
}

func NewProvider(opts ...options.Option[PreSolidBlockFilter]) module.Provider[*engine.Engine, presolidfilter.PreSolidFilter] {
	return module.Provide(func(e *engine.Engine) presolidfilter.PreSolidFilter {
		f := New(e.NewSubModule("PreSolidBlockFilter"), e, opts...)

		e.ConstructedEvent().OnTrigger(func() {
			e.SybilProtection.InitializedEvent().OnTrigger(func() {
				f.Init(e.SybilProtection.SeatManager().CommitteeInSlot)
			})

			e.Events.PreSolidFilter.LinkTo(f.events)

			f.InitializedEvent().Trigger()
		})

		return f
	})
}

// New creates a new PreSolidBlockFilter.
func New(subModule module.Module, apiProvider iotago.APIProvider, opts ...options.Option[PreSolidBlockFilter]) *PreSolidBlockFilter {
	return options.Apply(&PreSolidBlockFilter{
		Module:      subModule,
		events:      presolidfilter.NewEvents(),
		apiProvider: apiProvider,
	}, opts, func(p *PreSolidBlockFilter) {
		p.ShutdownEvent().OnTrigger(func() {
			p.StoppedEvent().Trigger()
		})

		p.ConstructedEvent().Trigger()
	})
}

// Init initializes the PreSolidBlockFilter.
func (f *PreSolidBlockFilter) Init(committeeFunc func(iotago.SlotIndex) (*account.SeatedAccounts, bool)) {
	f.committeeFunc = committeeFunc

	f.InitializedEvent().Trigger()
}

// ProcessReceivedBlock processes block from the given source.
func (f *PreSolidBlockFilter) ProcessReceivedBlock(block *model.Block, source peer.ID) {
	// Verify the block's version corresponds to the protocol version for the slot.
	apiForSlot := f.apiProvider.APIForSlot(block.ID().Slot())
	if apiForSlot.Version() != block.ProtocolBlock().Header.ProtocolVersion {
		f.events.BlockPreFiltered.Trigger(&presolidfilter.BlockPreFilteredEvent{
			Block:  block,
			Reason: ierrors.WithMessagef(ErrInvalidBlockVersion, "invalid protocol version %d (expected %d) for epoch %d", block.ProtocolBlock().Header.ProtocolVersion, apiForSlot.Version(), apiForSlot.TimeProvider().EpochFromSlot(block.ID().Slot())),
			Source: source,
		})

		return
	}

	if _, isValidation := block.ValidationBlock(); isValidation {
		blockSlot := block.ProtocolBlock().Slot()
		committee, exists := f.committeeFunc(blockSlot)
		if !exists {
			f.events.BlockPreFiltered.Trigger(&presolidfilter.BlockPreFilteredEvent{
				Block:  block,
				Reason: ierrors.WithMessagef(ErrValidatorNotInCommittee, "no committee for slot %d", blockSlot),
				Source: source,
			})

			return
		}

		if !committee.HasAccount(block.ProtocolBlock().Header.IssuerID) {
			f.events.BlockPreFiltered.Trigger(&presolidfilter.BlockPreFilteredEvent{
				Block:  block,
				Reason: ierrors.WithMessagef(ErrValidatorNotInCommittee, "validation block issuer %s is not part of the committee for slot %d", block.ProtocolBlock().Header.IssuerID, blockSlot),
				Source: source,
			})

			return
		}
	}

	f.events.BlockPreAllowed.Trigger(block)
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (f *PreSolidBlockFilter) Reset() { /* nothing to reset but comply with interface */ }
