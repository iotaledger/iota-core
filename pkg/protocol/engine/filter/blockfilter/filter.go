package blockfilter

import (
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	iotago "github.com/iotaledger/iota.go/v4"
)

var (
	ErrBlockTimeTooFarAheadInFuture = ierrors.New("a block cannot be too far ahead in the future")
	ErrValidatorNotInCommittee      = ierrors.New("validation block issuer is not in the committee")
	ErrInvalidBlockVersion          = ierrors.New("block has invalid protocol version")
)

// Filter filters blocks.
type Filter struct {
	events *filter.Events

	apiProvider iotago.APIProvider

	optsMaxAllowedWallClockDrift time.Duration

	committeeFunc func(iotago.SlotIndex) (*account.SeatedAccounts, bool)

	module.Module
}

func NewProvider(opts ...options.Option[Filter]) module.Provider[*engine.Engine, filter.Filter] {
	return module.Provide(func(e *engine.Engine) filter.Filter {
		f := New(e, opts...)
		f.TriggerConstructed()

		e.HookConstructed(func() {
			e.Events.Filter.LinkTo(f.events)
			e.SybilProtection.HookInitialized(func() {
				f.committeeFunc = e.SybilProtection.SeatManager().CommitteeInSlot
			})
			f.TriggerInitialized()
		})

		return f
	})
}

var _ filter.Filter = new(Filter)

// New creates a new Filter.
func New(apiProvider iotago.APIProvider, opts ...options.Option[Filter]) *Filter {
	return options.Apply(&Filter{
		events:      filter.NewEvents(),
		apiProvider: apiProvider,
	}, opts,
		(*Filter).TriggerConstructed,
		(*Filter).TriggerInitialized,
	)
}

// ProcessReceivedBlock processes block from the given source.
func (f *Filter) ProcessReceivedBlock(block *model.Block, source peer.ID) {
	// Verify the block's version corresponds to the protocol version for the slot.
	apiForSlot := f.apiProvider.APIForSlot(block.ID().Slot())
	if apiForSlot.Version() != block.ProtocolBlock().Header.ProtocolVersion {
		f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
			Block:  block,
			Reason: ierrors.Wrapf(ErrInvalidBlockVersion, "invalid protocol version %d (expected %d) for epoch %d", block.ProtocolBlock().Header.ProtocolVersion, apiForSlot.Version(), apiForSlot.TimeProvider().EpochFromSlot(block.ID().Slot())),
			Source: source,
		})

		return
	}

	// Verify the timestamp is not too far in the future.
	timeDelta := time.Since(block.ProtocolBlock().Header.IssuingTime)
	if timeDelta < -f.optsMaxAllowedWallClockDrift {
		f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
			Block:  block,
			Reason: ierrors.Wrapf(ErrBlockTimeTooFarAheadInFuture, "issuing time ahead %s vs %s allowed", -timeDelta, f.optsMaxAllowedWallClockDrift),
			Source: source,
		})

		return
	}

	if _, isValidation := block.ValidationBlock(); isValidation {
		blockSlot := block.ProtocolBlock().API.TimeProvider().SlotFromTime(block.ProtocolBlock().Header.IssuingTime)
		committee, exists := f.committeeFunc(blockSlot)
		if !exists {
			f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
				Block:  block,
				Reason: ierrors.Wrapf(ErrValidatorNotInCommittee, "no committee for slot %d", blockSlot),
				Source: source,
			})

			return
		}

		if !committee.HasAccount(block.ProtocolBlock().Header.IssuerID) {
			f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
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
func (f *Filter) Reset() { /* nothing to reset but comply with interface */ }

func (f *Filter) Shutdown() {
	f.TriggerStopped()
}

// WithMaxAllowedWallClockDrift specifies how far in the future are blocks allowed to be ahead of our own wall clock (defaults to 0 seconds).
func WithMaxAllowedWallClockDrift(d time.Duration) options.Option[Filter] {
	return func(filter *Filter) {
		filter.optsMaxAllowedWallClockDrift = d
	}
}
