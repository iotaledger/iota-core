package blockfilter

import (
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/api"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/pkg/errors"
)

var (
	ErrCommitmentTooOld             = errors.New("a block cannot commit to a slot that is older than the block's slot minus maxCommittableAge")
	ErrCommitmentTooRecent          = errors.New("a block cannot commit to a slot that is more recent than the block's slot minus minCommittableAge")
	ErrCommitmentInputTooOld        = errors.New("a block cannot contain a commitment input with index older than the block's slot minus maxCommittableAge")
	ErrCommitmentInputTooRecent     = errors.New("a block cannot contain a commitment input with index more recent than the block's slot minus minCommittableAge")
	ErrBlockTimeTooFarAheadInFuture = errors.New("a block cannot be too far ahead in the future")
	ErrInvalidProofOfWork           = errors.New("error validating PoW")
)

// Filter filters blocks.
type Filter struct {
	events *filter.Events

	apiProvider api.Provider

	optsMaxAllowedWallClockDrift time.Duration
	optsMinCommittableAge        iotago.SlotIndex
	optsMaxCommittableAge        iotago.SlotIndex
	optsSignatureValidation      bool

	module.Module
}

func NewProvider(opts ...options.Option[Filter]) module.Provider[*engine.Engine, filter.Filter] {
	return module.Provide(func(e *engine.Engine) filter.Filter {

		f := New(e, opts...)
		f.TriggerConstructed()

		e.HookConstructed(func() {
			e.Events.Filter.LinkTo(f.events)

			f.TriggerInitialized()
		})

		return f
	})
}

var _ filter.Filter = new(Filter)

// New creates a new Filter.
func New(apiProvider api.Provider, opts ...options.Option[Filter]) *Filter {
	return options.Apply(&Filter{
		events:                  filter.NewEvents(),
		optsSignatureValidation: true,
		apiProvider:             apiProvider,
	}, opts,
		(*Filter).TriggerConstructed,
		(*Filter).TriggerInitialized,
	)
}

// ProcessReceivedBlock processes block from the given source.
func (f *Filter) ProcessReceivedBlock(block *model.Block, source network.PeerID) {
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

	// check that commitment is within allowed range.
	if f.optsMinCommittableAge > 0 &&
		block.ProtocolBlock().SlotCommitmentID.Index() > 0 &&
		(block.ProtocolBlock().SlotCommitmentID.Index() > block.ID().Index() ||
			block.ID().Index()-block.ProtocolBlock().SlotCommitmentID.Index() < f.optsMinCommittableAge) {
		f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
			Block:  block,
			Reason: errors.WithMessagef(ErrCommitmentTooRecent, "block at slot %d committing to slot %d", block.ID().Index(), block.ProtocolBlock().SlotCommitmentID.Index()),
			Source: source,
		})

		return
	}
	if block.ID().Index()-block.ProtocolBlock().SlotCommitmentID.Index() > f.optsMaxCommittableAge {
		f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
			Block:  block,
			Reason: errors.WithMessagef(ErrCommitmentTooOld, "block at slot %d committing to slot %d", block.ID().Index(), block.ProtocolBlock().SlotCommitmentID.Index()),
			Source: source,
		})

		return
	}

	// check that commitment input (if any) is within allowed range.
	if basicBlock, isBasic := block.BasicBlock(); isBasic {
		if tx, isTX := basicBlock.Payload.(*iotago.Transaction); isTX {
			if cInput := tx.CommitmentInput(); cInput != nil {
				if f.optsMinCommittableAge > 0 &&
					cInput.CommitmentID.Index() > 0 &&
					(cInput.CommitmentID.Index() > block.ID().Index() ||
						block.ID().Index()-cInput.CommitmentID.Index() < f.optsMinCommittableAge) {
					f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
						Block:  block,
						Reason: errors.WithMessagef(ErrCommitmentTooRecent, "block at slot %d with commitment input to slot %d", block.ID().Index(), cInput.CommitmentID.Index()),
						Source: source,
					})

					return
				}
				if block.ID().Index()-cInput.CommitmentID.Index() > f.optsMaxCommittableAge {
					f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
						Block:  block,
						Reason: errors.WithMessagef(ErrCommitmentTooOld, "block at slot %d committing to slot %d", block.ID().Index(), cInput.CommitmentID.Index()),
						Source: source,
					})

					return
				}

			}
		}
	}

	f.events.BlockPreAllowed.Trigger(block)
}

func (f *Filter) Shutdown() {
	f.TriggerStopped()
}

// WithMinCommittableAge specifies how old a slot has to be for it to be committable.
func WithMinCommittableAge(age iotago.SlotIndex) options.Option[Filter] {
	return func(filter *Filter) {
		filter.optsMinCommittableAge = age
	}
}

// WithMaxCommittableAge specifies how old a slot has to be for it to be committable.
func WithMaxCommittableAge(age iotago.SlotIndex) options.Option[Filter] {
	return func(filter *Filter) {
		filter.optsMaxCommittableAge = age
	}
}

// WithMaxAllowedWallClockDrift specifies how far in the future are blocks allowed to be ahead of our own wall clock (defaults to 0 seconds).
func WithMaxAllowedWallClockDrift(d time.Duration) options.Option[Filter] {
	return func(filter *Filter) {
		filter.optsMaxAllowedWallClockDrift = d
	}
}
