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
)

var (
	ErrCommitmentTooOld                   = ierrors.New("a block cannot commit to a slot that is older than the block's slot minus maxCommittableAge")
	ErrCommitmentTooRecent                = ierrors.New("a block cannot commit to a slot that is more recent than the block's slot minus minCommittableAge")
	ErrCommitmentInputTooOld              = ierrors.New("a block cannot contain a commitment input with index older than the block's slot minus maxCommittableAge")
	ErrCommitmentInputTooRecent           = ierrors.New("a block cannot contain a commitment input with index more recent than the block's slot minus minCommittableAge")
	ErrBlockTimeTooFarAheadInFuture       = ierrors.New("a block cannot be too far ahead in the future")
	ErrInvalidBlockVersion                = ierrors.New("block has invalid protocol version")
	ErrCommitmentInputNewerThanCommitment = ierrors.New("a block cannot contain a commitment input with index newer than the commitment index")
)

// Filter filters blocks.
type Filter struct {
	events *filter.Events

	apiProvider api.Provider

	optsMaxAllowedWallClockDrift time.Duration

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
		events:      filter.NewEvents(),
		apiProvider: apiProvider,
	}, opts,
		(*Filter).TriggerConstructed,
		(*Filter).TriggerInitialized,
	)
}

// ProcessReceivedBlock processes block from the given source.
func (f *Filter) ProcessReceivedBlock(block *model.Block, source network.PeerID) {
	blockSlotAPI := f.apiProvider.APIForSlot(block.ID().Index())

	if blockSlotAPI.Version() != block.ProtocolBlock().ProtocolVersion {
		f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
			Block:  block,
			Reason: ierrors.Wrapf(ErrInvalidBlockVersion, "invalid protocol version %d (expected %d) for epoch %d", block.ProtocolBlock().ProtocolVersion, blockSlotAPI.Version(), blockSlotAPI.TimeProvider().EpochFromSlot(block.ID().Index())),
			Source: source,
		})

		return
	}
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

	minCommittableAge := blockSlotAPI.ProtocolParameters().MinCommittableAge()
	maxCommittableAge := blockSlotAPI.ProtocolParameters().MaxCommittableAge()

	// check that commitment is within allowed range.
	if minCommittableAge > 0 &&
		block.ProtocolBlock().SlotCommitmentID.Index() > 0 &&
		(block.ProtocolBlock().SlotCommitmentID.Index() > block.ID().Index() ||
			block.ID().Index()-block.ProtocolBlock().SlotCommitmentID.Index() < minCommittableAge) {
		f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
			Block:  block,
			Reason: ierrors.Wrapf(ErrCommitmentTooRecent, "block at slot %d committing to slot %d", block.ID().Index(), block.ProtocolBlock().SlotCommitmentID.Index()),
			Source: source,
		})

		return
	}
	if block.ID().Index() >= block.ProtocolBlock().SlotCommitmentID.Index() && block.ID().Index()-block.ProtocolBlock().SlotCommitmentID.Index() > maxCommittableAge {
		f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
			Block:  block,
			Reason: ierrors.Wrapf(ErrCommitmentTooOld, "block at slot %d committing to slot %d, max committable age %d", block.ID().Index(), block.ProtocolBlock().SlotCommitmentID.Index(), maxCommittableAge),
			Source: source,
		})

		return
	}

	// check that commitment input (if any) is within allowed range.
	if basicBlock, isBasic := block.BasicBlock(); isBasic {
		if tx, isTX := basicBlock.Payload.(*iotago.Transaction); isTX {
			if cInput := tx.CommitmentInput(); cInput != nil {
				if minCommittableAge > 0 &&
					cInput.CommitmentID.Index() > 0 &&
					(cInput.CommitmentID.Index() > block.ID().Index() ||
						block.ID().Index()-cInput.CommitmentID.Index() < minCommittableAge) {
					f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
						Block:  block,
						Reason: ierrors.Wrapf(ErrCommitmentInputTooRecent, "block at slot %d with commitment input to slot %d", block.ID().Index(), cInput.CommitmentID.Index()),
						Source: source,
					})

					return
				}
				if block.ID().Index() >= cInput.CommitmentID.Index() && block.ID().Index()-cInput.CommitmentID.Index() > maxCommittableAge {
					f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
						Block:  block,
						Reason: ierrors.Wrapf(ErrCommitmentInputTooOld, "block at slot %d committing to slot %d, max committable age %d", block.ID().Index(), cInput.CommitmentID.Index(), maxCommittableAge),
						Source: source,
					})

					return
				}

				if cInput.CommitmentID.Index() > block.ProtocolBlock().SlotCommitmentID.Index() {
					f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
						Block:  block,
						Reason: ierrors.Wrapf(ErrCommitmentInputNewerThanCommitment, "transaction in a block contains CommitmentInput to slot %d while max allowed is %d", cInput.CommitmentID.Index(), block.ProtocolBlock().SlotCommitmentID.Index()),
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

// WithMaxAllowedWallClockDrift specifies how far in the future are blocks allowed to be ahead of our own wall clock (defaults to 0 seconds).
func WithMaxAllowedWallClockDrift(d time.Duration) options.Option[Filter] {
	return func(filter *Filter) {
		filter.optsMaxAllowedWallClockDrift = d
	}
}
