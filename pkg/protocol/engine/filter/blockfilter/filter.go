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
	ErrCommitmentNotCommittable                  = errors.New("a block cannot commit to a slot that cannot objectively be committable yet")
	ErrBlockTimeTooFarAheadInFuture              = errors.New("a block cannot be too far ahead in the future")
	ErrInvalidSignature                          = errors.New("block has invalid signature")
	ErrInvalidProofOfWork                        = errors.New("error validating PoW")
	ErrTransactionCommitmentInputTooFarInThePast = errors.New("transaction in a block references too old CommitmentInput")
)

// Filter filters blocks.
type Filter struct {
	events *filter.Events

	protocolParamsFunc func() *iotago.ProtocolParameters

	optsMaxAllowedWallClockDrift time.Duration
	optsSignatureValidation      bool

	module.Module
}

func NewProvider(opts ...options.Option[Filter]) module.Provider[*engine.Engine, filter.Filter] {
	return module.Provide(func(e *engine.Engine) filter.Filter {

		f := New(e.Storage.Settings().ProtocolParameters, opts...)
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
func New(protocolParamsFunc func() *iotago.ProtocolParameters, opts ...options.Option[Filter]) *Filter {
	return options.Apply(&Filter{
		events:                  filter.NewEvents(),
		optsSignatureValidation: true,
		protocolParamsFunc:      protocolParamsFunc,
	}, opts,
		(*Filter).TriggerConstructed,
		(*Filter).TriggerInitialized,
	)
}

// ProcessReceivedBlock processes block from the given source.
func (f *Filter) ProcessReceivedBlock(block *model.Block, source network.PeerID) {
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

	// Check if the block is trying to commit to a slot that is not yet committable.
	// This check, together with the optsMaxAllowedWallClockDrift makes sure that no one can issue blocks with commitments in the future.
	if block.SlotCommitment().Index() > 0 && block.SlotCommitment().Index()+protocolParams.EvictionAge > block.ID().Index() {
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

	// Verify that if CommitmentInput referenced by a transaction is not too old.
	// This check provides that Time-lock and other time-based checks in the VM don't have to rely on what is the relation of transaction to
	// current wall time, because it will only receive transactions that are created in a recent and limited past.
	if block.Block().Payload != nil && block.Block().Payload.PayloadType() == iotago.PayloadTransaction {
		transaction, _ := block.Block().Payload.(*iotago.Transaction)
		if commitmentInput := transaction.CommitmentInput(); commitmentInput != nil {
			// commitmentInput must reference a commitment
			// that is between the latest possible, non-evicted committable slot in relation to the block time
			// (block.ID().Index() - protocolParams.LivenessThreshold - protocolParams.EvictionAge)
			// and the slot that block is committing to.

			// Parameters moved to the other side of inequality to avoid underflow errors with subtraction from an uint64 type.
			if commitmentInput.CommitmentID.Index()+protocolParams.LivenessThreshold+protocolParams.EvictionAge < block.ID().Index() {
				f.events.BlockFiltered.Trigger(&filter.BlockFilteredEvent{
					Block:  block,
					Reason: errors.WithMessagef(ErrTransactionCommitmentInputTooFarInThePast, "transaction in a block contains CommitmentInput to slot %d while min allowed is %d", commitmentInput.CommitmentID.Index(), block.ID().Index()-protocolParams.LivenessThreshold-protocolParams.EvictionAge),
					Source: source,
				})

				return
			}
			if commitmentInput.CommitmentID.Index() > block.SlotCommitment().Index() {
				f.events.BlockFiltered.Trigger(&filter.BlockFilteredEvent{
					Block:  block,
					Reason: errors.WithMessagef(ErrTransactionCommitmentInputTooFarInThePast, "transaction in a block contains CommitmentInput to slot %d while max allowed is %d", commitmentInput.CommitmentID.Index(), block.SlotCommitment().Index()),
					Source: source,
				})

				return
			}
		}
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
