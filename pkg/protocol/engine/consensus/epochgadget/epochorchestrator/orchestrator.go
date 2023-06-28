package epochorchestrator

import (
	"io"

	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/epochgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/epochgadget/epochorchestrator/performance"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Orchestrator struct {
	events          *epochgadget.Events
	sybilProtection sybilprotection.SybilProtection
	ledger          ledger.Ledger
	timeProvider    *iotago.TimeProvider

	performanceManager *performance.Tracker

	optsEpochEndNearingThreshold iotago.SlotIndex

	module.Module
}

func NewProvider(opts ...options.Option[Orchestrator]) module.Provider[*engine.Engine, epochgadget.Gadget] {
	return module.Provide(func(e *engine.Engine) epochgadget.Gadget {
		return options.Apply(&Orchestrator{
			events:             epochgadget.NewEvents(),
			sybilProtection:    e.SybilProtection,
			ledger:             e.Ledger,
			timeProvider:       e.API().TimeProvider(),
			performanceManager: performance.NewTracker(e.Storage.Rewards(), e.Storage.PoolStats(), e.Storage.Committee(), e.Storage.PerformanceFactors, e.API().TimeProvider(), e.API().ManaDecayProvider()),
		}, opts,
			func(o *Orchestrator) {
				e.Events.BlockGadget.BlockAccepted.Hook(o.BlockAccepted)
				e.Events.Notarization.SlotCommitted.Hook(func(scd *notarization.SlotCommittedDetails) { o.CommitSlot(scd.Commitment.Index()) })
				e.Events.SlotGadget.SlotFinalized.Hook(o.slotFinalized)

				e.Events.EpochGadget.LinkTo(o.events)
			},
			(*Orchestrator).TriggerConstructed,
		)
	})
}

func (o *Orchestrator) Shutdown() {
	o.TriggerStopped()
}

func (o *Orchestrator) BlockAccepted(block *blocks.Block) {
	o.performanceManager.BlockAccepted(block)
}

func (o *Orchestrator) CommitSlot(slot iotago.SlotIndex) {
	currentEpoch := o.timeProvider.EpochFromSlot(slot)

	if o.timeProvider.EpochEnd(currentEpoch) == slot {
		committee, exists := o.performanceManager.LoadCommitteeForEpoch(currentEpoch)
		if !exists {
			// If the committee for the epoch wasn't set before we promote the current one.
			committee, exists = o.performanceManager.LoadCommitteeForEpoch(currentEpoch - 1)
			if !exists {
				panic("committee for epoch not found")
			}
			err := o.performanceManager.RegisterCommittee(currentEpoch, committee)
			if err != nil {
				panic("failed to register committee for epoch")
			}
		}

		o.performanceManager.ApplyEpoch(currentEpoch, committee)
	}
}

func (o *Orchestrator) ValidatorReward(validatorID iotago.AccountID, stakeAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (validatorReward iotago.Mana, err error) {
	return o.performanceManager.ValidatorReward(validatorID, stakeAmount, epochStart, epochEnd)
}

func (o *Orchestrator) DelegatorReward(validatorID iotago.AccountID, delegatedAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (delegatorsReward iotago.Mana, err error) {
	return o.performanceManager.DelegatorReward(validatorID, delegatedAmount, epochStart, epochEnd)
}

func (o *Orchestrator) Import(reader io.ReadSeeker) error {
	return o.performanceManager.Import(reader)
}

func (o *Orchestrator) Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) error {
	return o.performanceManager.Export(writer, targetSlot)
}

func (o *Orchestrator) slotFinalized(slot iotago.SlotIndex) {
	epoch := o.timeProvider.EpochFromSlot(slot)
	if o.timeProvider.EpochEnd(epoch)-o.optsEpochEndNearingThreshold == slot {
		newCommittee := o.selectNewCommittee(slot)
		o.events.CommitteeSelected.Trigger(newCommittee)
	}
}

func (o *Orchestrator) selectNewCommittee(slot iotago.SlotIndex) *account.Accounts {
	currentEpoch := o.timeProvider.EpochFromSlot(slot)
	nextEpoch := currentEpoch + 1
	candidates := o.performanceManager.EligibleValidatorCandidates(nextEpoch)

	weightedCandidates := account.NewAccounts()
	if err := candidates.ForEach(func(candidate iotago.AccountID) error {
		a, exists, err := o.ledger.Account(candidate, slot)
		if err != nil {
			return err
		}
		if !exists {
			panic("account does not exist")
		}

		weightedCandidates.Set(candidate, &account.Pool{
			PoolStake:      a.ValidatorStake + a.DelegationStake,
			ValidatorStake: a.ValidatorStake,
			FixedCost:      a.FixedCost,
		})

		return nil
	}); err != nil {
		panic(err)
	}

	newCommittee := o.sybilProtection.RotateCommittee(nextEpoch, weightedCandidates)
	weightedCommittee := newCommittee.Accounts()
	err := o.performanceManager.RegisterCommittee(nextEpoch, weightedCommittee)
	if err != nil {
		panic("failed to register committee for epoch")
	}
	return weightedCommittee
}

func WithEpochEndNearingThreshold(threshold iotago.SlotIndex) options.Option[Orchestrator] {
	return func(o *Orchestrator) {
		o.optsEpochEndNearingThreshold = threshold
	}
}
