package sybilprotectionv1

import (
	"fmt"
	"io"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1/performance"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Orchestrator struct {
	events            *sybilprotection.Events
	seatManager       seatmanager.SeatManager
	ledger            ledger.Ledger // do we need the whole Ledger or just a callback to retrieve account data?
	lastCommittedSlot iotago.SlotIndex
	timeProvider      *iotago.TimeProvider

	performanceManager *performance.Tracker

	epochEndNearingThreshold iotago.SlotIndex
	maxCommittableAge        iotago.SlotIndex

	optsInitialCommittee    *account.Accounts
	optsSeatManagerProvider module.Provider[*engine.Engine, seatmanager.SeatManager]

	mutex syncutils.Mutex

	module.Module
}

func NewProvider(opts ...options.Option[Orchestrator]) module.Provider[*engine.Engine, sybilprotection.SybilProtection] {
	return module.Provide(func(e *engine.Engine) sybilprotection.SybilProtection {
		return options.Apply(&Orchestrator{
			events: sybilprotection.NewEvents(),

			// TODO: the following fields should be initialized after the engine is constructed,
			//  otherwise we implicitly rely on the order of engine initialization which can change at any time.
		}, opts,
			func(o *Orchestrator) {
				o.seatManager = o.optsSeatManagerProvider(e)

				e.HookConstructed(func() {
					o.ledger = e.Ledger

					e.Storage.Settings().HookInitialized(func() {
						o.timeProvider = e.API().TimeProvider()
						o.maxCommittableAge = e.Storage.Settings().ProtocolParameters().EvictionAge + e.Storage.Settings().ProtocolParameters().EvictionAge

						o.epochEndNearingThreshold = e.Storage.Settings().ProtocolParameters().EpochNearingThreshold

						o.performanceManager = performance.NewTracker(o.maxCommittableAge, e.Storage.Rewards(), e.Storage.PoolStats(), e.Storage.Committee(), e.Storage.PerformanceFactors, e.API().TimeProvider(), e.API().ManaDecayProvider())
						o.lastCommittedSlot = e.Storage.Settings().LatestCommitment().Index()

						// TODO: check if the following value is correctly set to twice eviction age

						if o.optsInitialCommittee != nil {
							if err := o.performanceManager.RegisterCommittee(1, o.optsInitialCommittee); err != nil {
								panic(ierrors.Wrap(err, "error while registering initial committee for epoch 1"))
							}
						}
						o.TriggerConstructed()

						// When the engine is triggered initialized, snapshot has been read or database has been initialized properly,
						// so the committee should be available in the performance manager.
						e.HookInitialized(func() {
							// Make sure that the sybil protection knows about the committee of the current epoch
							// (according to the latest committed slot), and potentially the next selected
							// committee if we have one.
							currentEpoch := o.timeProvider.EpochFromSlot(e.Storage.Settings().LatestCommitment().Index())

							committee, exists := o.performanceManager.LoadCommitteeForEpoch(currentEpoch)
							if !exists {
								panic("failed to load committee for last finalized slot to initialize sybil protection")
							}
							o.seatManager.ImportCommittee(currentEpoch, committee)
							if nextCommittee, nextCommitteeExists := o.performanceManager.LoadCommitteeForEpoch(currentEpoch + 1); nextCommitteeExists {
								o.seatManager.ImportCommittee(currentEpoch+1, nextCommittee)
							}

							o.TriggerInitialized()
						})
					})
				})

				// TODO: CommitSlot should be called directly from NotarizationManager and should return some trees to be put into actual commitment
				e.Events.Notarization.SlotCommitted.Hook(func(scd *notarization.SlotCommittedDetails) { o.CommitSlot(scd.Commitment.Index()) })

				e.Events.SlotGadget.SlotFinalized.Hook(o.slotFinalized)

				e.Events.SybilProtection.LinkTo(o.events)
			},
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
	o.mutex.Lock()
	defer o.mutex.Unlock()

	currentEpoch := o.timeProvider.EpochFromSlot(slot)
	nextEpoch := currentEpoch + 1

	o.lastCommittedSlot = slot

	// If the committed slot is `maxCommittableAge`
	// away from the end of the epoch, then register a committee for the next epoch.
	if o.timeProvider.EpochEnd(currentEpoch) == slot+o.maxCommittableAge {
		if _, committeeExists := o.performanceManager.LoadCommitteeForEpoch(nextEpoch); !committeeExists {
			// If the committee for the epoch wasn't set before due to finalization of a slot,
			// we promote the current committee to also serve in the next epoch.
			committee, exists := o.performanceManager.LoadCommitteeForEpoch(currentEpoch)
			if !exists {
				panic(fmt.Sprintf("committee for current epoch %d not found", currentEpoch))
			}

			committee.SetReused()

			o.seatManager.SetCommittee(nextEpoch, committee)
			if err := o.performanceManager.RegisterCommittee(nextEpoch, committee); err != nil {
				panic(ierrors.Wrapf(err, "failed to register committee for epoch %d", nextEpoch))
			}
		}
	}

	if o.timeProvider.EpochEnd(currentEpoch) == slot {
		committee, exists := o.performanceManager.LoadCommitteeForEpoch(currentEpoch)
		if !exists {
			panic(fmt.Sprintf("committee for a finished epoch %d not found", currentEpoch))
		}

		o.performanceManager.ApplyEpoch(currentEpoch, committee)
	}
}

func (o *Orchestrator) SeatManager() seatmanager.SeatManager {
	return o.seatManager
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
	o.mutex.Lock()
	defer o.mutex.Unlock()

	epoch := o.timeProvider.EpochFromSlot(slot)

	// Only select new committee if the finalized slot is epochEndNearingThreshold slots from EpochEnd and the last
	// committed slot is earlier than (the last slot of the epoch - maxCommittableAge).
	// Otherwise, skip committee selection because it's too late and the committee has been reused.
	if slot+o.epochEndNearingThreshold == o.timeProvider.EpochEnd(epoch) && o.timeProvider.EpochEnd(epoch) > o.lastCommittedSlot+o.maxCommittableAge {
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

	newCommittee := o.seatManager.RotateCommittee(nextEpoch, weightedCandidates)
	weightedCommittee := newCommittee.Accounts()

	// FIXME: weightedCommittee returned by the PoA sybil protection does not have stake specified, which will cause problems during rewards calculation.
	err := o.performanceManager.RegisterCommittee(nextEpoch, weightedCommittee)
	if err != nil {
		panic("failed to register committee for epoch")
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

func WithSeatManagerProvider(seatManagerProvider module.Provider[*engine.Engine, seatmanager.SeatManager]) options.Option[Orchestrator] {
	return func(o *Orchestrator) {
		o.optsSeatManagerProvider = seatManagerProvider
	}
}
