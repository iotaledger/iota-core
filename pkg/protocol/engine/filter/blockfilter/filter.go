package blockfilter

import (
	"time"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	iotago "github.com/iotaledger/iota.go/v4"
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

	protocolParamsFunc func() *iotago.ProtocolParameters

	optsMaxAllowedWallClockDrift time.Duration
	optsMinCommittableAge        iotago.SlotIndex
	optsMaxCommittableAge        iotago.SlotIndex
	optsSignatureValidation      bool

	module.Module
}

func NewProvider(opts ...options.Option[Filter]) module.Provider[*engine.Engine, filter.Filter] {
	return module.Provide(func(e *engine.Engine) filter.Filter {
		f := New(e.Storage.Settings().ProtocolParameters, opts...)

		e.HookConstructed(func() {
			e.Events.Filter.LinkTo(f.events)
		})

		return f
	})
}

var _ filter.Filter = new(Filter)

// New creates a new Filter.
func New(protocolParamsFunc func() *iotago.ProtocolParameters, opts ...options.Option[Filter]) *Filter {
	return options.Apply(&Filter{
		events:                  filter.NewEvents(),
		protocolParamsFunc:      protocolParamsFunc,
		optsSignatureValidation: true,
	}, opts,
		(*Filter).TriggerConstructed,
		(*Filter).TriggerInitialized,
	)
}

// ProcessReceivedBlock processes block from the given source.
func (f *Filter) ProcessReceivedBlock(block *model.Block, source network.PeerID) {
	// TODO: if TX add check for TX timestamp

	protocolParams := f.protocolParamsFunc()
	if protocolParams.MinPoWScore > 0 {
		// Check if the block has enough PoW score.
		score, _, err := block.Block().POW()
		if err != nil {
			f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
				Block:  block,
				Reason: errors.WithMessage(ErrInvalidProofOfWork, "error calculating PoW score"),
				Source: source,
			})

			return
		}

		if score < float64(protocolParams.MinPoWScore) {
			f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
				Block:  block,
				Reason: errors.WithMessagef(ErrInvalidProofOfWork, "score %f is less than min score %d", score, protocolParams.MinPoWScore),
				Source: source,
			})

			return
		}
	}

	// Verify the timestamp is not too far in the future.
	timeDelta := time.Since(block.Block().IssuingTime)
	if timeDelta < -f.optsMaxAllowedWallClockDrift {
		f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
			Block:  block,
			Reason: errors.WithMessagef(ErrBlockTimeTooFarAheadInFuture, "issuing time ahead %s vs %s allowed", -timeDelta, f.optsMaxAllowedWallClockDrift),
			Source: source,
		})

		return
	}

	// check that commitment is within allowed range.
	if f.optsMinCommittableAge > 0 &&
		block.SlotCommitment().Index() > 0 &&
		(block.SlotCommitment().Index() > block.ID().Index() ||
			block.ID().Index()-block.SlotCommitment().Index() < f.optsMinCommittableAge) {
		f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
			Block:  block,
			Reason: errors.WithMessagef(ErrCommitmentTooRecent, "block at slot %d committing to slot %d", block.ID().Index(), block.SlotCommitment().Index()),
			Source: source,
		})

		return
	}
	if block.ID().Index()-block.SlotCommitment().Index() > f.optsMaxCommittableAge {
		f.events.BlockPreFiltered.Trigger(&filter.BlockPreFilteredEvent{
			Block:  block,
			Reason: errors.WithMessagef(ErrCommitmentTooOld, "block at slot %d committing to slot %d", block.ID().Index(), block.SlotCommitment().Index()),
			Source: source,
		})

		return
	}

	// check that commitment inputs (if any) are within allowed range.
	tx, isTX := block.Block().Payload.(*iotago.Transaction)
	if isTX {
		cInputs, err := tx.CommitmentInputs()
		if err == nil {
			for _, cInput := range cInputs {
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
