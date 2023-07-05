package epochorchestrator

import (
	"fmt"
	"io"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/core/api"
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
	events *epochgadget.Events

	apiProvider api.Provider

	sybilProtection   sybilprotection.SybilProtection // do we need the whole SybilProtection or just a callback to RotateCommittee?
	ledger            ledger.Ledger                   // do we need the whole Ledger or just a callback to retrieve account data?
	lastCommittedSlot iotago.SlotIndex

	performanceTracker *performance.Tracker

	optsInitialCommittee *account.Accounts

	mutex syncutils.Mutex

	module.Module
}

func NewProvider(opts ...options.Option[Orchestrator]) module.Provider[*engine.Engine, epochgadget.Gadget] {
	return module.Provide(func(e *engine.Engine) epochgadget.Gadget {
		return options.Apply(&Orchestrator{
			events: epochgadget.NewEvents(),

			// TODO: the following fields should be initialized after the engine is constructed,
			//  otherwise we implicitly rely on the order of engine initialization which can change at any time.
			apiProvider:     e,
			sybilProtection: e.SybilProtection,
			ledger:          e.Ledger,
		}, opts,
			func(o *Orchestrator) {
				e.HookConstructed(func() {
					o.performanceTracker = performance.NewTracker(e.Storage.Rewards(), e.Storage.PoolStats(), e.Storage.Committee(), e.Storage.PerformanceFactors, e)
					o.lastCommittedSlot = e.Storage.Settings().LatestCommitment().Index()

					if o.optsInitialCommittee != nil {
						if err := o.performanceTracker.RegisterCommittee(1, o.optsInitialCommittee); err != nil {
							panic(ierrors.Wrap(err, "error while registering initial committee for epoch 0"))
						}
					}

					o.TriggerConstructed()
					o.TriggerInitialized()
				})

				// TODO: does this potentially cause a data race due to fanning-in parallel events?
				e.Events.BlockGadget.BlockAccepted.Hook(o.BlockAccepted)

				// TODO: CommitSlot should be called directly from NotarizationManager and should return some trees to be put into actual commitment
				e.Events.Notarization.SlotCommitted.Hook(func(scd *notarization.SlotCommittedDetails) { o.CommitSlot(scd.Commitment.Index()) })

				e.Events.SlotGadget.SlotFinalized.Hook(o.slotFinalized)

				e.Events.EpochGadget.LinkTo(o.events)
			},
		)
	})
}

func (o *Orchestrator) Shutdown() {
	o.TriggerStopped()
}

func (o *Orchestrator) BlockAccepted(block *blocks.Block) {
	o.performanceTracker.BlockAccepted(block)
}

func (o *Orchestrator) CommitSlot(slot iotago.SlotIndex) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	api := o.apiProvider.APIForSlot(slot)
	timeProvider := api.TimeProvider()
	currentEpoch := timeProvider.EpochFromSlot(slot)
	nextEpoch := currentEpoch + 1

	// TODO: check if the following value is correctly set to twice eviction age
	// maxCommittableSlot = 2 * evictionAge
	maxCommittableSlot := api.ProtocolParameters().EvictionAge() + api.ProtocolParameters().EvictionAge()<<1

	// If the committed slot is `maxCommittableSlot`
	// away from the end of the epoch, then register a committee for the next epoch.
	if timeProvider.EpochEnd(currentEpoch) == slot+maxCommittableSlot {
		if _, committeeExists := o.performanceTracker.LoadCommitteeForEpoch(nextEpoch); !committeeExists {
			// If the committee for the epoch wasn't set before due to finalization of a slot,
			// we promote the current committee to also serve in the next epoch.
			committee, exists := o.performanceTracker.LoadCommitteeForEpoch(currentEpoch)
			if !exists {
				panic(fmt.Sprintf("committee for current epoch %d not found", currentEpoch))
			}

			_ = o.sybilProtection.RotateCommittee(nextEpoch, committee)
			if err := o.performanceTracker.RegisterCommittee(nextEpoch, committee); err != nil {
				panic(ierrors.Wrapf(err, "failed to register committee for epoch %d", nextEpoch))
			}
		}
	}

	if timeProvider.EpochEnd(currentEpoch) == slot {
		committee, exists := o.performanceTracker.LoadCommitteeForEpoch(currentEpoch)
		if !exists {
			panic(fmt.Sprintf("committee for a finished epoch %d not found", currentEpoch))
		}

		o.performanceTracker.ApplyEpoch(currentEpoch, committee)
	}

	o.lastCommittedSlot = slot
}

func (o *Orchestrator) ValidatorReward(validatorID iotago.AccountID, stakeAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (validatorReward iotago.Mana, err error) {
	return o.performanceTracker.ValidatorReward(validatorID, stakeAmount, epochStart, epochEnd)
}

func (o *Orchestrator) DelegatorReward(validatorID iotago.AccountID, delegatedAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (delegatorsReward iotago.Mana, err error) {
	return o.performanceTracker.DelegatorReward(validatorID, delegatedAmount, epochStart, epochEnd)
}

func (o *Orchestrator) Import(reader io.ReadSeeker) error {
	return o.performanceTracker.Import(reader)
}

func (o *Orchestrator) Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) error {
	return o.performanceTracker.Export(writer, targetSlot)
}

func (o *Orchestrator) slotFinalized(slot iotago.SlotIndex) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	api := o.apiProvider.APIForSlot(slot)
	timeProvider := api.TimeProvider()
	epoch := timeProvider.EpochFromSlot(slot)

	// TODO: check if the following value is correctly set to twice eviction age
	// maxCommittableSlot = 2 * evictionAge
	maxCommittableSlot := api.ProtocolParameters().EvictionAge() + api.ProtocolParameters().EvictionAge()<<1

	// Only select new committee if the finalized slot is epochEndNearingThreshold slots from EpochEnd and the last
	// committed slot is earlier than (the last slot of the epoch - maxCommittableSlot).
	// Otherwise, skip committee selection because it's too late and the committee has been reused.
	epochEndSlot := timeProvider.EpochEnd(epoch)
	if slot+api.ProtocolParameters().EpochNearingThreshold() == epochEndSlot && epochEndSlot > o.lastCommittedSlot+maxCommittableSlot {
		newCommittee := o.selectNewCommittee(slot)
		o.events.CommitteeSelected.Trigger(newCommittee)
	}
}

func (o *Orchestrator) selectNewCommittee(slot iotago.SlotIndex) *account.Accounts {
	timeProvider := o.apiProvider.APIForSlot(slot).TimeProvider()
	currentEpoch := timeProvider.EpochFromSlot(slot)
	nextEpoch := currentEpoch + 1
	candidates := o.performanceTracker.EligibleValidatorCandidates(nextEpoch)

	weightedCandidates := account.NewAccounts()
	if err := candidates.ForEach(func(candidate iotago.AccountID) error {
		a, exists, err := o.ledger.Account(candidate, slot)
		if err != nil {
			return err
		}
		if !exists {
			// TODO: instead of panic, we should return an error here
			panic(ierrors.Errorf("account of committee candidate does not exist: %s", candidate))
		}

		weightedCandidates.Set(candidate, &account.Pool{
			PoolStake:      a.ValidatorStake + a.DelegationStake,
			ValidatorStake: a.ValidatorStake,
			FixedCost:      a.FixedCost,
		})

		return nil
	}); err != nil {
		// TODO: instead of panic, we should return an error here
		panic(err)
	}

	newCommittee := o.sybilProtection.RotateCommittee(nextEpoch, weightedCandidates)
	weightedCommittee := newCommittee.Accounts()

	// FIXME: weightedCommittee returned by the PoA sybil protection does not have stake specified, which will cause problems during rewards calculation.
	err := o.performanceTracker.RegisterCommittee(nextEpoch, weightedCommittee)
	if err != nil {
		// TODO: instead of panic, we should return an error here
		panic(ierrors.Wrap(err, "failed to register committee for epoch"))
	}

	return weightedCommittee
}

// WithInitialCommittee registers the passed committee on a given slot.
// This is needed to generate Genesis snapshot with some initial committee.
func WithInitialCommittee(committee *account.Accounts) options.Option[Orchestrator] {
	return func(o *Orchestrator) {
		o.optsInitialCommittee = committee
	}
}
