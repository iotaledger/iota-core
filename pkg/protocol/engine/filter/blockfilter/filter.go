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
	"github.com/iotaledger/iota.go/v4/api"
)

var ErrBlockTimeTooFarAheadInFuture = ierrors.New("a block cannot be too far ahead in the future")
var ErrValidatorNotInCommittee = ierrors.New("validation block issuer is not in the committee")

// Filter filters blocks.
type Filter struct {
	events *filter.Events

	apiProvider api.Provider

	optsMaxAllowedWallClockDrift time.Duration

	committeeFunc func(iotago.SlotIndex) *account.SeatedAccounts

	module.Module
}

func NewProvider(opts ...options.Option[Filter]) module.Provider[*engine.Engine, filter.Filter] {
	return module.Provide(func(e *engine.Engine) filter.Filter {
		f := New(e, opts...)
		f.TriggerConstructed()

		e.HookConstructed(func() {
			e.Events.Filter.LinkTo(f.events)
			e.SybilProtection.HookInitialized(func() {
				f.committeeFunc = e.SybilProtection.SeatManager().Committee
			})
			f.TriggerInitialized()
		})

		return f
	})
}

var _ filter.Filter = new(Filter)

// New creates a new Filter.
func New(apiProvider api.Provider, opts ...options.Option[Filter]) *Filter {
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
	// Verify the timestamp is not too far in the future.
	timeDelta := time.Since(block.ProtocolBlock().IssuingTime)
	if timeDelta < -f.optsMaxAllowedWallClockDrift {
		f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
			Block:  block,
			Reason: ierrors.Wrapf(ErrBlockTimeTooFarAheadInFuture, "issuing time ahead %s vs %s allowed", -timeDelta, f.optsMaxAllowedWallClockDrift),
			Source: source,
		})

		return
	}

	if _, isValidation := block.ValidationBlock(); isValidation {
		blockAPI, err := f.apiProvider.APIForVersion(block.ProtocolBlock().ProtocolVersion)
		if err != nil {
			f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
				Block:  block,
				Reason: ierrors.Wrapf(err, "could not get API for version %d", block.ProtocolBlock().ProtocolVersion),
				Source: source,
			})

			return
		}
		blockSlot := blockAPI.TimeProvider().SlotFromTime(block.ProtocolBlock().IssuingTime)
		if !f.committeeFunc(blockSlot).HasAccount(block.ProtocolBlock().IssuerID) {
			f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
				Block:  block,
				Reason: ierrors.Wrapf(ErrValidatorNotInCommittee, "validation block issuer %s is not part of the committee for slot %d", block.ProtocolBlock().IssuerID, blockSlot),
				Source: source,
			})

			return
		}
	}

	f.events.BlockPreAllowed.Trigger(block)
}

func (f *Filter) Shutdown() {
	f.TriggerStopped()
}

// WithMaxAllowedWallClockDrift specifies how far in the future are blocks allowed to be ahead of our own wall clock (defaults to 0 seconds).
func WithMaxAllowedWallClockDrift(d time.Duration) options.Option[Filter] {
	return func(filter *Filter) {
		filter.optsMaxAllowedWallClockDrift = d
	}
}
