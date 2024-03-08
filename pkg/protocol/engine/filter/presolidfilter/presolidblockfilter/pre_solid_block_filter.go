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
		f := New(e, opts...)
		f.TriggerConstructed()

		e.Constructed.OnTrigger(func() {
			e.Events.PreSolidFilter.LinkTo(f.events)
			e.SybilProtection.HookInitialized(func() {
				f.committeeFunc = e.SybilProtection.SeatManager().CommitteeInSlot
			})
			f.TriggerInitialized()
		})

		return f
	})
}

var _ presolidfilter.PreSolidFilter = new(PreSolidBlockFilter)

// New creates a new PreSolidBlockFilter.
func New(apiProvider iotago.APIProvider, opts ...options.Option[PreSolidBlockFilter]) *PreSolidBlockFilter {
	return options.Apply(&PreSolidBlockFilter{
		events:      presolidfilter.NewEvents(),
		apiProvider: apiProvider,
	}, opts,
		(*PreSolidBlockFilter).TriggerConstructed,
		(*PreSolidBlockFilter).TriggerInitialized,
	)
}

// ProcessReceivedBlock processes block from the given source.
func (f *PreSolidBlockFilter) ProcessReceivedBlock(block *model.Block, source peer.ID) {
	// Verify the block's version corresponds to the protocol version for the slot.
	apiForSlot := f.apiProvider.APIForSlot(block.ID().Slot())
	if apiForSlot.Version() != block.ProtocolBlock().Header.ProtocolVersion {
		f.events.BlockPreFiltered.Trigger(&presolidfilter.BlockPreFilteredEvent{
			Block:  block,
			Reason: ierrors.Wrapf(ErrInvalidBlockVersion, "invalid protocol version %d (expected %d) for epoch %d", block.ProtocolBlock().Header.ProtocolVersion, apiForSlot.Version(), apiForSlot.TimeProvider().EpochFromSlot(block.ID().Slot())),
			Source: source,
		})

		return
	}

	if _, isValidation := block.ValidationBlock(); isValidation {
		blockSlot := block.ProtocolBlock().API.TimeProvider().SlotFromTime(block.ProtocolBlock().Header.IssuingTime)
		committee, exists := f.committeeFunc(blockSlot)
		if !exists {
			f.events.BlockPreFiltered.Trigger(&presolidfilter.BlockPreFilteredEvent{
				Block:  block,
				Reason: ierrors.Wrapf(ErrValidatorNotInCommittee, "no committee for slot %d", blockSlot),
				Source: source,
			})

			return
		}

		if !committee.HasAccount(block.ProtocolBlock().Header.IssuerID) {
			f.events.BlockPreFiltered.Trigger(&presolidfilter.BlockPreFilteredEvent{
				Block:  block,
				Reason: ierrors.Wrapf(ErrValidatorNotInCommittee, "validation block issuer %s is not part of the committee for slot %d", block.ProtocolBlock().Header.IssuerID, blockSlot),
				Source: source,
			})

			return
		}
	}

	f.events.BlockPreAllowed.Trigger(block)
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (f *PreSolidBlockFilter) Reset() { /* nothing to reset but comply with interface */ }

func (f *PreSolidBlockFilter) Shutdown() {
	f.TriggerStopped()
}
