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
	ErrCommitmentNotCommittable     = errors.New("a block cannot commit to a slot that cannot objectively be committable yet")
	ErrBlockTimeTooFarAheadInFuture = errors.New("a block cannot be too far ahead in the future")
	ErrInvalidSignature             = errors.New("block has invalid signature")
	ErrInvalidProofOfWork           = errors.New("error validating PoW")
	ErrInsufficientBurnedMana       = errors.New("a block must burn at least the reference mana cost")
	ErrNegativeBIC                  = errors.New("a block issuer must have non-negative block issuance credit")
)

// Filter filters blocks.
type Filter struct {
	events *filter.Events

	protocolParamsFunc func() *iotago.ProtocolParameters

	optsMaxAllowedWallClockDrift time.Duration
	optsMinCommittableSlotAge    iotago.SlotIndex
	optsSignatureValidation      bool

	blockIssuerCheck func(*iotago.Block) error

	// TODO: replace this placeholder for RMC with a link to the accounts manager with RMC provider.
	optsReferenceManaCost uint64

	module.Module
}

func NewProvider(opts ...options.Option[Filter]) module.Provider[*engine.Engine, filter.Filter] {
	return module.Provide(func(e *engine.Engine) filter.Filter {
		f := New(e.Storage.Settings().ProtocolParameters, opts...)

		e.HookConstructed(func() {
			e.Events.Filter.LinkTo(f.events)
			f.blockIssuerCheck = e.Ledger.IsBlockIssuerAllowed
			f.TriggerConstructed()
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
			f.events.BlockFiltered.Trigger(&filter.BlockFilteredEvent{
				Block:  block,
				Reason: errors.WithMessage(ErrInvalidProofOfWork, "error calculating PoW score"),
				Source: source,
			})

			return
		}

		if score < float64(protocolParams.MinPoWScore) {
			f.events.BlockFiltered.Trigger(&filter.BlockFilteredEvent{
				Block:  block,
				Reason: errors.WithMessagef(ErrInvalidProofOfWork, "score %f is less than min score %d", score, protocolParams.MinPoWScore),
				Source: source,
			})

			return
		}
	}

	// Check that the block burns sufficient Mana to cover RMC
	if block.Block().BurnedMana < f.optsReferenceManaCost {
		f.events.BlockFiltered.Trigger(&filter.BlockFilteredEvent{
			Block:  block,
			Reason: errors.WithMessagef(ErrInsufficientBurnedMana, "block with burned mana %d while reference mana cost is %d", block.Block().BurnedMana, f.optsReferenceManaCost),
			Source: source,
		})

		return
	}

	// Check that the issuer of this block has non-negative block issuance credit
	if err := f.blockIssuerCheck(block.Block()); err != nil {
		f.events.BlockFiltered.Trigger(&filter.BlockFilteredEvent{
			Block:  block,
			Reason: errors.Wrapf(err, "block issuer account %s is locked due to negative or non-existent BIC", block.Block().IssuerID),
			Source: source,
		})

		return
	}

	// Check if the block is trying to commit to a slot that is not yet committable.
	// This check, together with the optsMaxAllowedWallClockDrift makes sure, that no one can issue blocks with commitments in the future.
	if f.optsMinCommittableSlotAge > 0 &&
		block.SlotCommitment().Index() > 0 &&
		(block.SlotCommitment().Index() > block.ID().Index() ||
			block.ID().Index()-block.SlotCommitment().Index() < f.optsMinCommittableSlotAge) {
		f.events.BlockFiltered.Trigger(&filter.BlockFilteredEvent{
			Block:  block,
			Reason: errors.WithMessagef(ErrCommitmentNotCommittable, "block at slot %d committing to slot %d", block.ID().Index(), block.Block().SlotCommitment.Index),
			Source: source,
		})

		return
	}

	// Verify the timestamp is not too far in the future.
	timeDelta := time.Since(block.Block().IssuingTime)
	if timeDelta < -f.optsMaxAllowedWallClockDrift {
		f.events.BlockFiltered.Trigger(&filter.BlockFilteredEvent{
			Block:  block,
			Reason: errors.WithMessagef(ErrBlockTimeTooFarAheadInFuture, "issuing time ahead %s vs %s allowed", -timeDelta, f.optsMaxAllowedWallClockDrift),
			Source: source,
		})

		return
	}

	if f.optsSignatureValidation {
		// Verify the block signature.
		if valid, err := block.Block().VerifySignature(); !valid {
			if err != nil {
				f.events.BlockFiltered.Trigger(&filter.BlockFilteredEvent{
					Block:  block,
					Reason: errors.WithMessagef(ErrInvalidSignature, "error: %s", err.Error()),
					Source: source,
				})

				return
			}

			f.events.BlockFiltered.Trigger(&filter.BlockFilteredEvent{
				Block:  block,
				Reason: ErrInvalidSignature,
				Source: source,
			})

			return
		}
	}

	f.events.BlockAllowed.Trigger(block)
}

func (f *Filter) Shutdown() {
	f.TriggerStopped()
}

// WithMinCommittableSlotAge specifies how old a slot has to be for it to be committable.
func WithMinCommittableSlotAge(age iotago.SlotIndex) options.Option[Filter] {
	return func(filter *Filter) {
		filter.optsMinCommittableSlotAge = age
	}
}

// WithMaxAllowedWallClockDrift specifies how far in the future are blocks allowed to be ahead of our own wall clock (defaults to 0 seconds).
func WithMaxAllowedWallClockDrift(d time.Duration) options.Option[Filter] {
	return func(filter *Filter) {
		filter.optsMaxAllowedWallClockDrift = d
	}
}

// WithSignatureValidation specifies if the block signature should be validated (defaults to yes).
func WithSignatureValidation(validation bool) options.Option[Filter] {
	return func(filter *Filter) {
		filter.optsSignatureValidation = validation
	}
}

// WithReferenceManaCost specifies a placeholder for RMC.
func WithReferenceManaCost(rmc uint64) options.Option[Filter] {
	return func(filter *Filter) {
		filter.optsReferenceManaCost = rmc
	}
}
