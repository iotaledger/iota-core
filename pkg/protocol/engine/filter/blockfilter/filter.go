package blockfilter

import (
	"time"

	"github.com/pkg/errors"

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
	ErrCommitmentNotCommittable                  = errors.New("a block cannot commit to a slot that cannot objectively be committable yet")
	ErrBlockTimeTooFarAheadInFuture              = errors.New("a block cannot be too far ahead in the future")
	ErrInvalidSignature                          = errors.New("block has invalid signature")
	ErrTransactionCommitmentInputTooFarInThePast = errors.New("transaction in a block references too old CommitmentInput")
	ErrTransactionCommitmentInputInTheFuture     = errors.New("transaction in a block references a CommitmentInput in the future")
)

// Filter filters blocks.
type Filter struct {
	events *filter.Events

	apiProvider api.Provider

	optsMaxAllowedWallClockDrift time.Duration
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
	api := f.apiProvider.APIForSlot(block.ID().Index())
	protocolParams := api.ProtocolParameters()

	// Check if the block is trying to commit to a slot that is not yet committable.
	// This check, together with the optsMaxAllowedWallClockDrift makes sure that no one can issue blocks with commitments in the future.
	if block.ProtocolBlock().SlotCommitmentID.Index() > 0 && block.ProtocolBlock().SlotCommitmentID.Index()+protocolParams.EvictionAge() > block.ID().Index() {
		f.events.BlockFiltered.Trigger(&filter.BlockFilteredEvent{
			Block:  block,
			Reason: errors.WithMessagef(ErrCommitmentNotCommittable, "block at slot %d committing to slot %d", block.ID().Index(), block.ProtocolBlock().SlotCommitmentID.Index()),
			Source: source,
		})

		return
	}

	// Verify the timestamp is not too far in the future.
	timeDelta := time.Since(block.ProtocolBlock().IssuingTime)
	if timeDelta < -f.optsMaxAllowedWallClockDrift {
		f.events.BlockFiltered.Trigger(&filter.BlockFilteredEvent{
			Block:  block,
			Reason: errors.WithMessagef(ErrBlockTimeTooFarAheadInFuture, "issuing time ahead %s vs %s allowed", -timeDelta, f.optsMaxAllowedWallClockDrift),
			Source: source,
		})

		return
	}

	if basicBlock, isBasic := block.BasicBlock(); isBasic {

		// Verify that if CommitmentInput referenced by a transaction is not too old.
		// This check provides that Time-lock and other time-based checks in the VM don't have to rely on what is the relation of transaction to
		// current wall time, because it will only receive transactions that are created in a recent and limited past.
		if basicBlock.Payload != nil && basicBlock.Payload.PayloadType() == iotago.PayloadTransaction {
			transaction, _ := basicBlock.Payload.(*iotago.Transaction)
			if commitmentInput := transaction.CommitmentInput(); commitmentInput != nil {
				// commitmentInput must reference a commitment
				// that is between the latest possible, non-evicted committable slot in relation to the block time
				// block.ID().Index() - (2 * protocolParams.EvictionAge)
				// and the slot that block is committing to.

				// Parameters moved to the other side of inequality to avoid underflow errors with subtraction from an uint64 type.
				if commitmentInput.CommitmentID.Index()+(protocolParams.EvictionAge()<<1) < block.ID().Index() {
					f.events.BlockFiltered.Trigger(&filter.BlockFilteredEvent{
						Block:  block,
						Reason: errors.WithMessagef(ErrTransactionCommitmentInputTooFarInThePast, "transaction in a block contains CommitmentInput to slot %d while min allowed is %d", commitmentInput.CommitmentID.Index(), block.ID().Index()-(protocolParams.EvictionAge()<<1)),
						Source: source,
					})

					return
				}
				if commitmentInput.CommitmentID.Index() > block.ProtocolBlock().SlotCommitmentID.Index() {
					f.events.BlockFiltered.Trigger(&filter.BlockFilteredEvent{
						Block:  block,
						Reason: errors.WithMessagef(ErrTransactionCommitmentInputInTheFuture, "transaction in a block contains CommitmentInput to slot %d while max allowed is %d", commitmentInput.CommitmentID.Index(), block.ProtocolBlock().SlotCommitmentID.Index()),
						Source: source,
					})

					return
				}
			}
		}
	}

	if f.optsSignatureValidation {
		// Verify the block signature.
		if valid, err := block.ProtocolBlock().VerifySignature(api); !valid {
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
