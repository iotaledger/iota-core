package sybilprotectionv1

import (
	"fmt"
	"io"
	"sort"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager/poa"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1/performance"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

type SybilProtection struct {
	events *sybilprotection.Events

	apiProvider api.Provider

	seatManager       seatmanager.SeatManager
	ledger            ledger.Ledger // do we need the whole Ledger or just a callback to retrieve account data?
	lastCommittedSlot iotago.SlotIndex

	performanceTracker *performance.Tracker

	errHandler func(error)

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
					o.ledger = e.Ledger
					o.errHandler = e.ErrorHandler("SybilProtection")

					latestCommittedSlot := e.Storage.Settings().LatestCommitment().Index()
					latestCommittedEpoch := o.apiProvider.APIForSlot(latestCommittedSlot).TimeProvider().EpochFromSlot(latestCommittedSlot)
					o.performanceTracker = performance.NewTracker(e.Storage.RewardsForEpoch, e.Storage.PoolStats(), e.Storage.Committee(), e.Storage.PerformanceFactors, latestCommittedEpoch, e, o.errHandler)
					o.lastCommittedSlot = latestCommittedSlot

					if o.optsInitialCommittee != nil {
						if err := o.performanceTracker.RegisterCommittee(0, o.optsInitialCommittee); err != nil {
							panic(ierrors.Wrap(err, "error while registering initial committee for epoch 0"))
						}
					}
					o.TriggerConstructed()

					// When the engine is triggered initialized, snapshot has been read or database has been initialized properly,
					// so the committee should be available in the performance manager.
					e.HookInitialized(func() {
						// Make sure that the sybil protection knows about the committee of the current epoch
						// (according to the latest committed slot), and potentially the next selected
						// committee if we have one.

						currentEpoch := e.CurrentAPI().TimeProvider().EpochFromSlot(e.Storage.Settings().LatestCommitment().Index())

						committee, exists := o.performanceTracker.LoadCommitteeForEpoch(currentEpoch)
						if !exists {
							panic("failed to load committee for last finalized slot to initialize sybil protection")
						}
						o.seatManager.ImportCommittee(currentEpoch, committee)
						fmt.Println("committee import", committee.TotalStake(), currentEpoch)
						if nextCommittee, nextCommitteeExists := o.performanceTracker.LoadCommitteeForEpoch(currentEpoch + 1); nextCommitteeExists {
							o.seatManager.ImportCommittee(currentEpoch+1, nextCommittee)
							fmt.Println("next committee", nextCommittee.TotalStake(), currentEpoch+1)
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

func (o *SybilProtection) TrackValidationBlock(block *blocks.Block) {
	o.performanceTracker.TrackValidationBlock(block)
}

func (o *SybilProtection) CommitSlot(slot iotago.SlotIndex) (committeeRoot, rewardsRoot iotago.Identifier) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	apiForSlot := o.apiProvider.APIForSlot(slot)
	timeProvider := apiForSlot.TimeProvider()
	currentEpoch := timeProvider.EpochFromSlot(slot)
	nextEpoch := currentEpoch + 1

	maxCommittableAge := apiForSlot.ProtocolParameters().MaxCommittableAge()

	// If the committed slot is `maxCommittableSlot`
	// away from the end of the epoch, then register a committee for the next epoch.
	if timeProvider.EpochEnd(currentEpoch) == slot+maxCommittableAge {
		if _, committeeExists := o.performanceTracker.LoadCommitteeForEpoch(nextEpoch); !committeeExists {
			// If the committee for the epoch wasn't set before due to finalization of a slot,
			// we promote the current committee to also serve in the next epoch.
			committee, exists := o.performanceTracker.LoadCommitteeForEpoch(currentEpoch)
			if !exists {
				panic(fmt.Sprintf("committee for current epoch %d not found", currentEpoch))
			}

			committee.SetReused()
			fmt.Println("reuse committee", currentEpoch, "stake", committee.TotalValidatorStake())
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

	if apiForSlot.TimeProvider().EpochEnd(currentEpoch) > slot+maxCommittableAge {
		targetCommitteeEpoch = currentEpoch
	} else {
		targetCommitteeEpoch = nextEpoch
	}

	committeeRoot, err := o.committeeRoot(targetCommitteeEpoch)
	if err != nil {
		panic(ierrors.Wrapf(err, "failed to calculate committee root for epoch %d", targetCommitteeEpoch))
	}

	var targetRewardsEpoch iotago.EpochIndex
	if apiForSlot.TimeProvider().EpochEnd(currentEpoch) == slot {
		targetRewardsEpoch = nextEpoch
	} else {
		targetRewardsEpoch = currentEpoch
	}

	rewardsRoot, err = o.performanceTracker.RewardsRoot(targetRewardsEpoch)
	if err != nil {
		panic(ierrors.Wrapf(err, "failed to calculate rewards root for epoch %d", targetRewardsEpoch))
	}

	o.lastCommittedSlot = slot

	return
}

func (o *SybilProtection) committeeRoot(targetCommitteeEpoch iotago.EpochIndex) (committeeRoot iotago.Identifier, err error) {
	committee, exists := o.performanceTracker.LoadCommitteeForEpoch(targetCommitteeEpoch)
	if !exists {
		panic(fmt.Sprintf("committee for a finished epoch %d not found", targetCommitteeEpoch))
	}

	comitteeTree := ads.NewSet(
		mapdb.NewMapDB(),
		iotago.AccountID.Bytes,
		iotago.IdentifierFromBytes,
	)

	var innerErr error
	committee.ForEach(func(accountID iotago.AccountID, _ *account.Pool) bool {
		if err := comitteeTree.Add(accountID); err != nil {
			innerErr = ierrors.Wrapf(err, "failed to add account %s to committee tree", accountID)
			return false
		}

		return true
	})
	if innerErr != nil {
		return iotago.Identifier{}, innerErr
	}

	return iotago.Identifier(comitteeTree.Root()), nil
}

func (o *SybilProtection) SeatManager() seatmanager.SeatManager {
	return o.seatManager
}

func (o *SybilProtection) ValidatorReward(validatorID iotago.AccountID, stakeAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (validatorReward iotago.Mana, actualEpochStart, actualEpochEnd iotago.EpochIndex, err error) {
	return o.performanceTracker.ValidatorReward(validatorID, stakeAmount, epochStart, epochEnd)
}

func (o *SybilProtection) DelegatorReward(validatorID iotago.AccountID, delegatedAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (delegatorsReward iotago.Mana, actualEpochStart, actualEpochEnd iotago.EpochIndex, err error) {
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

	// Only select new committee if the finalized slot is epochEndNearingThreshold slots from EpochEnd and the last
	// committed slot is earlier than (the last slot of the epoch - maxCommittableAge).
	// Otherwise, skip committee selection because it's too late and the committee has been reused.
	epochEndSlot := timeProvider.EpochEnd(epoch)
	if slot+apiForSlot.ProtocolParameters().EpochNearingThreshold() == epochEndSlot &&
		epochEndSlot > o.lastCommittedSlot+apiForSlot.ProtocolParameters().MaxCommittableAge() {
		newCommittee := o.selectNewCommittee(slot)
		fmt.Println("new committee selection finalization", epoch, newCommittee.TotalStake(), newCommittee.TotalValidatorStake())
		o.events.CommitteeSelected.Trigger(newCommittee, epoch+1)
	}
}

// IsCandidateActive returns true if the given validator is currently active.
func (o *SybilProtection) IsCandidateActive(validatorID iotago.AccountID, epoch iotago.EpochIndex) bool {
	activeCandidates := o.performanceTracker.EligibleValidatorCandidates(epoch)
	return activeCandidates.Has(validatorID)
}

// EligibleValidators returns the currently known list of recently active validator candidates for the given epoch.
func (o *SybilProtection) EligibleValidators(epoch iotago.EpochIndex) (accounts.AccountsData, error) {
	candidates := o.performanceTracker.EligibleValidatorCandidates(epoch)
	validators := make(accounts.AccountsData, 0)

	if err := candidates.ForEach(func(candidate iotago.AccountID) error {
		accountData, exists, err := o.ledger.Account(candidate, o.lastCommittedSlot)
		if err != nil {
			return ierrors.Wrapf(err, "failed to load account data for candidate %s", candidate)
		}
		if !exists {
			return ierrors.Errorf("account of committee candidate does not exist: %s", candidate)
		}
		// if `End Epoch` is the current one or has passed, validator is no longer considered for validator selection
		if accountData.StakeEndEpoch <= epoch {
			return nil
		}
		validators = append(validators, accountData.Clone())

		return nil
	}); err != nil {
		return nil, ierrors.Wrapf(err, "failed to iterate over eligible validator candidates")
	}

	return validators, nil
}

// OrderedRegisteredCandidateValidatorsList returns the currently known list of registered validator candidates for the given epoch.
func (o *SybilProtection) OrderedRegisteredCandidateValidatorsList(epoch iotago.EpochIndex) ([]*apimodels.ValidatorResponse, error) {
	candidates := o.performanceTracker.ValidatorCandidates(epoch)
	activeCandidates := o.performanceTracker.EligibleValidatorCandidates(epoch)

	validatorResp := make([]*apimodels.ValidatorResponse, 0, candidates.Size())
	if err := candidates.ForEach(func(candidate iotago.AccountID) error {
		accountData, exists, err := o.ledger.Account(candidate, o.lastCommittedSlot)
		if err != nil {
			return ierrors.Wrapf(err, "failed to get account %s", candidate)
		}
		if !exists {
			return ierrors.Errorf("account of committee candidate does not exist: %s", candidate)
		}
		// if `End Epoch` is the current one or has passed, validator is no longer considered for validator selection
		if accountData.StakeEndEpoch <= epoch {
			return nil
		}
		active := activeCandidates.Has(candidate)
		validatorResp = append(validatorResp, &apimodels.ValidatorResponse{
			AccountID:                      accountData.ID,
			StakingEpochEnd:                accountData.StakeEndEpoch,
			PoolStake:                      accountData.ValidatorStake + accountData.DelegationStake,
			ValidatorStake:                 accountData.ValidatorStake,
			FixedCost:                      accountData.FixedCost,
			Active:                         active,
			LatestSupportedProtocolVersion: accountData.LatestSupportedProtocolVersionAndHash.Version,
			LatestSupportedProtocolHash:    accountData.LatestSupportedProtocolVersionAndHash.Hash,
		})

		return nil
	}); err != nil {
		return nil, ierrors.Wrapf(err, "failed to iterate over eligible validator candidates")
	}
	// sort candidates by stake
	sort.Slice(validatorResp, func(i, j int) bool {
		return validatorResp[i].ValidatorStake > validatorResp[j].ValidatorStake
	})

	return validatorResp, nil
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
			o.errHandler(ierrors.Errorf("account of committee candidate does not exist: %s", candidate))
		}

		weightedCandidates.Set(candidate, &account.Pool{
			PoolStake:      a.ValidatorStake + a.DelegationStake,
			ValidatorStake: a.ValidatorStake,
			FixedCost:      a.FixedCost,
		})

		return nil
	}); err != nil {
		o.errHandler(err)
	}

	newCommittee := o.seatManager.RotateCommittee(nextEpoch, weightedCandidates)
	weightedCommittee := newCommittee.Accounts()

	err := o.performanceTracker.RegisterCommittee(nextEpoch, weightedCommittee)
	if err != nil {
		o.errHandler(ierrors.Wrap(err, "failed to register committee for epoch"))
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
