package sybilprotectionv1

import (
	"fmt"
	"io"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/core/api"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager/poa"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1/performance"
	iotago "github.com/iotaledger/iota.go/v4"
)

type SybilProtection struct {
	events *sybilprotection.Events

	apiProvider api.Provider

	seatManager       seatmanager.SeatManager
	sybilProtection   sybilprotection.SybilProtection // do we need the whole SybilProtection or just a callback to RotateCommittee?
	ledger            ledger.Ledger                   // do we need the whole Ledger or just a callback to retrieve account data?
	lastCommittedSlot iotago.SlotIndex

	performanceTracker *performance.Tracker

	optsInitialCommittee    *account.Accounts
	optsSeatManagerProvider module.Provider[*engine.Engine, seatmanager.SeatManager]

	mutex syncutils.Mutex

	module.Module
}

func NewProvider(opts ...options.Option[SybilProtection]) module.Provider[*engine.Engine, sybilprotection.SybilProtection] {
	return module.Provide(func(e *engine.Engine) sybilprotection.SybilProtection {
		return options.Apply(&SybilProtection{
			events: sybilprotection.NewEvents(),

			apiProvider:             e,
			optsSeatManagerProvider: poa.NewProvider(),
		}, opts,
			func(o *SybilProtection) {
				o.seatManager = o.optsSeatManagerProvider(e)

				e.HookConstructed(func() {
					// TODO: the following fields should be initialized after the engine is constructed,
					//  otherwise we implicitly rely on the order of engine initialization which can change at any time.
					o.ledger = e.Ledger

					o.performanceTracker = performance.NewTracker(e.Storage.Rewards(), e.Storage.PoolStats(), e.Storage.Committee(), e.Storage.PerformanceFactors, e)
					o.lastCommittedSlot = e.Storage.Settings().LatestCommitment().Index()

					// TODO: check if the following value is correctly set to twice eviction age

					if o.optsInitialCommittee != nil {
						if err := o.performanceTracker.RegisterCommittee(1, o.optsInitialCommittee); err != nil {
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

						// TODO: how do we handle changing API here?
						currentEpoch := e.LatestAPI().TimeProvider().EpochFromSlot(e.Storage.Settings().LatestCommitment().Index())

						committee, exists := o.performanceTracker.LoadCommitteeForEpoch(currentEpoch)
						if !exists {
							panic("failed to load committee for last finalized slot to initialize sybil protection")
						}
						o.seatManager.ImportCommittee(currentEpoch, committee)
						if nextCommittee, nextCommitteeExists := o.performanceTracker.LoadCommitteeForEpoch(currentEpoch + 1); nextCommitteeExists {
							o.seatManager.ImportCommittee(currentEpoch+1, nextCommittee)
						}

						o.TriggerInitialized()
					})
				})

				e.Events.SlotGadget.SlotFinalized.Hook(o.slotFinalized)

				e.Events.SybilProtection.LinkTo(o.events)
			},
		)
	})
}

func (o *SybilProtection) Shutdown() {
	o.TriggerStopped()
}

func (o *SybilProtection) BlockAccepted(block *blocks.Block) {
	o.performanceTracker.BlockAccepted(block)
}

func (o *SybilProtection) CommitSlot(slot iotago.SlotIndex) (committeeRoot, rewardsRoot iotago.Identifier) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	apiForSlot := o.apiProvider.APIForSlot(slot)
	timeProvider := apiForSlot.TimeProvider()
	currentEpoch := timeProvider.EpochFromSlot(slot)
	nextEpoch := currentEpoch + 1

	// TODO: check if the following value is correctly set to twice eviction age
	// maxCommittableSlot = 2 * evictionAge
	maxCommittableSlot := apiForSlot.ProtocolParameters().EvictionAge() << 1

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

			committee.SetReused()

			o.seatManager.SetCommittee(nextEpoch, committee)

			o.events.CommitteeSelected.Trigger(committee, nextEpoch)

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
	var targetCommitteeEpoch iotago.EpochIndex
	if o.timeProvider.EpochEnd(currentEpoch) > slot+o.maxCommittableAge {
		targetCommitteeEpoch = currentEpoch
	} else {
		targetCommitteeEpoch = nextEpoch
	}

	committeeRoot = o.committeeRoot(targetCommitteeEpoch)

	var targetRewardsEpoch iotago.EpochIndex
	if o.timeProvider.EpochEnd(currentEpoch) == slot {
		targetRewardsEpoch = nextEpoch
	} else {
		targetRewardsEpoch = currentEpoch
	}

	rewardsRoot = o.performanceTracker.RewardsRoot(targetRewardsEpoch)

	o.lastCommittedSlot = slot

	return
}

func (o *SybilProtection) committeeRoot(targetCommitteeEpoch iotago.EpochIndex) iotago.Identifier {
	committee, exists := o.performanceTracker.LoadCommitteeForEpoch(targetCommitteeEpoch)
	if !exists {
		panic(fmt.Sprintf("committee for a finished epoch %d not found", targetCommitteeEpoch))
	}

	comitteeTree := ads.NewSet[iotago.AccountID, *iotago.AccountID](mapdb.NewMapDB())
	committee.ForEach(func(accountID iotago.AccountID, _ *account.Pool) bool {
		comitteeTree.Add(accountID)

		return true
	})

	return iotago.Identifier(comitteeTree.Root())
}

func (o *SybilProtection) SeatManager() seatmanager.SeatManager {
	return o.seatManager
}

func (o *SybilProtection) ValidatorReward(validatorID iotago.AccountID, stakeAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (validatorReward iotago.Mana, err error) {
	return o.performanceTracker.ValidatorReward(validatorID, stakeAmount, epochStart, epochEnd)
}

func (o *SybilProtection) DelegatorReward(validatorID iotago.AccountID, delegatedAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (delegatorsReward iotago.Mana, err error) {
	return o.performanceTracker.DelegatorReward(validatorID, delegatedAmount, epochStart, epochEnd)
}

func (o *SybilProtection) Import(reader io.ReadSeeker) error {
	return o.performanceTracker.Import(reader)
}

func (o *SybilProtection) Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) error {
	return o.performanceTracker.Export(writer, targetSlot)
}

func (o *SybilProtection) slotFinalized(slot iotago.SlotIndex) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	apiForSlot := o.apiProvider.APIForSlot(slot)
	timeProvider := apiForSlot.TimeProvider()
	epoch := timeProvider.EpochFromSlot(slot)

	// TODO: check if the following value is correctly set to twice eviction age
	// maxCommittableSlot = 2 * evictionAge
	maxCommittableSlot := apiForSlot.ProtocolParameters().EvictionAge() << 1

	// Only select new committee if the finalized slot is epochEndNearingThreshold slots from EpochEnd and the last
	// committed slot is earlier than (the last slot of the epoch - maxCommittableAge).
	// Otherwise, skip committee selection because it's too late and the committee has been reused.
	epochEndSlot := timeProvider.EpochEnd(epoch)
	if slot+apiForSlot.ProtocolParameters().EpochNearingThreshold() == epochEndSlot && epochEndSlot > o.lastCommittedSlot+maxCommittableSlot {
		newCommittee := o.selectNewCommittee(slot)
		o.events.CommitteeSelected.Trigger(newCommittee, epoch+1)
	}
}

func (o *SybilProtection) selectNewCommittee(slot iotago.SlotIndex) *account.Accounts {
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

	newCommittee := o.seatManager.RotateCommittee(nextEpoch, weightedCandidates)
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
func WithInitialCommittee(committee *account.Accounts) options.Option[SybilProtection] {
	return func(o *SybilProtection) {
		o.optsInitialCommittee = committee
	}
}

func WithSeatManagerProvider(seatManagerProvider module.Provider[*engine.Engine, seatmanager.SeatManager]) options.Option[SybilProtection] {
	return func(o *SybilProtection) {
		o.optsSeatManagerProvider = seatManagerProvider
	}
}
