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
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

type SybilProtection struct {
	events *sybilprotection.Events

	apiProvider iotago.APIProvider

	seatManager       seatmanager.SeatManager
	ledger            ledger.Ledger // do we need the whole Ledger or just a callback to retrieve account data?
	lastCommittedSlot iotago.SlotIndex

	performanceTracker *performance.Tracker

	errHandler func(error)

	optsInitialCommittee    accounts.AccountsData
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

					latestCommittedSlot := e.Storage.Settings().LatestCommitment().Slot()
					latestCommittedEpoch := o.apiProvider.APIForSlot(latestCommittedSlot).TimeProvider().EpochFromSlot(latestCommittedSlot)
					o.performanceTracker = performance.NewTracker(e.Storage.RewardsForEpoch, e.Storage.PoolStats(), e.Storage.Committee(), e.Storage.CommitteeCandidates, e.Storage.ValidatorPerformances, latestCommittedEpoch, e, o.errHandler)
					o.lastCommittedSlot = latestCommittedSlot

					if o.optsInitialCommittee != nil {
						if _, err := o.seatManager.RotateCommittee(0, o.optsInitialCommittee); err != nil {
							panic(ierrors.Wrap(err, "error while registering initial committee for epoch 0"))
						}
					}

					o.TriggerConstructed()

					// When the engine is triggered initialized, snapshot has been read or database has been initialized properly,
					// so the committee should be available in the performance manager.
					e.HookInitialized(func() {
						// Mark the committee for the last committed slot as active.
						currentEpoch := e.CommittedAPI().TimeProvider().EpochFromSlot(e.Storage.Settings().LatestCommitment().Slot())
						err := o.seatManager.InitializeCommittee(currentEpoch, e.Clock.Accepted().RelativeTime())
						if err != nil {
							panic(ierrors.Wrap(err, "error while initializing committee"))
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

func (o *SybilProtection) TrackBlock(block *blocks.Block) {
	if _, isValidationBlock := block.ValidationBlock(); isValidationBlock {
		o.performanceTracker.TrackValidationBlock(block)

		return
	}

	accountData, exists, err := o.ledger.Account(block.ProtocolBlock().Header.IssuerID, block.SlotCommitmentID().Slot())
	if err != nil {
		o.errHandler(ierrors.Wrapf(err, "error while retrieving account from account %s in slot %d from accounts ledger", block.ProtocolBlock().Header.IssuerID, block.SlotCommitmentID().Slot()))

		return
	}

	if !exists {
		return
	}

	blockEpoch := o.apiProvider.APIForSlot(block.ID().Slot()).TimeProvider().EpochFromSlot(block.ID().Slot())

	// if the block is issued before the stake end epoch, then it's not a valid validator or candidate block
	if accountData.StakeEndEpoch < blockEpoch {
		return
	}

	// if a candidate block is issued in the stake end epoch,
	// or if block is issued after EpochEndSlot - EpochNearingThreshold, because candidates can register only until that point.
	// then don't consider it because the validator can't be part of the committee in the next epoch
	if accountData.StakeEndEpoch == blockEpoch ||
		block.ID().Slot()+o.apiProvider.APIForSlot(block.ID().Slot()).ProtocolParameters().EpochNearingThreshold() > o.apiProvider.APIForSlot(block.ID().Slot()).TimeProvider().EpochEnd(blockEpoch) {

		return
	}

	if block.Payload().PayloadType() == iotago.PayloadCandidacyAnnouncement {
		o.performanceTracker.TrackCandidateBlock(block)
	}
}

func (o *SybilProtection) CommitSlot(slot iotago.SlotIndex) (committeeRoot, rewardsRoot iotago.Identifier, err error) {
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
		if _, committeeExists := o.seatManager.CommitteeInEpoch(nextEpoch); !committeeExists {
			// If the committee for the epoch wasn't set before due to finalization of a slot,
			// we promote the current committee to also serve in the next epoch.
			committee, exists := o.seatManager.CommitteeInEpoch(currentEpoch)
			if !exists {
				// that should never happen as it is already the fallback strategy
				panic(fmt.Sprintf("committee for current epoch %d not found", currentEpoch))
			}

			committeeAccounts, err := committee.Accounts()
			if err != nil {
				return iotago.Identifier{}, iotago.Identifier{}, ierrors.Wrapf(err, "failed to get accounts from committee for epoch %d", currentEpoch)
			}

			committeeAccounts.SetReused()
			if err = o.seatManager.SetCommittee(nextEpoch, committeeAccounts); err != nil {
				return iotago.Identifier{}, iotago.Identifier{}, ierrors.Wrapf(err, "failed to set committee for epoch %d", nextEpoch)
			}
			o.performanceTracker.ClearCandidates()

			o.events.CommitteeSelected.Trigger(committeeAccounts, nextEpoch)
		}
	}

	if timeProvider.EpochEnd(currentEpoch) == slot {
		committee, exists := o.performanceTracker.LoadCommitteeForEpoch(currentEpoch)
		if !exists {
			return iotago.Identifier{}, iotago.Identifier{}, ierrors.Wrapf(err, "committee for a finished epoch %d not found", currentEpoch)
		}

		err = o.performanceTracker.ApplyEpoch(currentEpoch, committee)
		if err != nil {
			return iotago.Identifier{}, iotago.Identifier{}, ierrors.Wrapf(err, "failed to apply epoch %d", currentEpoch)
		}
	}

	var targetCommitteeEpoch iotago.EpochIndex

	if apiForSlot.TimeProvider().EpochEnd(currentEpoch) > slot+maxCommittableAge {
		targetCommitteeEpoch = currentEpoch
	} else {
		targetCommitteeEpoch = nextEpoch
	}

	committeeRoot, err = o.committeeRoot(targetCommitteeEpoch)
	if err != nil {
		return iotago.Identifier{}, iotago.Identifier{}, ierrors.Wrapf(err, "failed to calculate committee root for epoch %d", targetCommitteeEpoch)
	}

	var targetRewardsEpoch iotago.EpochIndex
	if apiForSlot.TimeProvider().EpochEnd(currentEpoch) == slot {
		targetRewardsEpoch = nextEpoch
	} else {
		targetRewardsEpoch = currentEpoch
	}

	rewardsRoot, err = o.performanceTracker.RewardsRoot(targetRewardsEpoch)
	if err != nil {
		return iotago.Identifier{}, iotago.Identifier{}, ierrors.Wrapf(err, "failed to calculate rewards root for epoch %d", targetRewardsEpoch)
	}

	o.lastCommittedSlot = slot

	return
}

func (o *SybilProtection) committeeRoot(targetCommitteeEpoch iotago.EpochIndex) (committeeRoot iotago.Identifier, err error) {
	committee, exists := o.performanceTracker.LoadCommitteeForEpoch(targetCommitteeEpoch)
	if !exists {
		return iotago.Identifier{}, ierrors.Wrapf(err, "committee for an epoch %d not found", targetCommitteeEpoch)
	}

	committeeTree := ads.NewSet[iotago.Identifier](
		mapdb.NewMapDB(),
		iotago.Identifier.Bytes,
		iotago.IdentifierFromBytes,
		iotago.AccountID.Bytes,
		iotago.AccountIDFromBytes,
	)

	var innerErr error
	committee.ForEach(func(accountID iotago.AccountID, _ *account.Pool) bool {
		if err := committeeTree.Add(accountID); err != nil {
			innerErr = ierrors.Wrapf(err, "failed to add account %s to committee tree", accountID)
			return false
		}

		return true
	})
	if innerErr != nil {
		return iotago.Identifier{}, innerErr
	}

	return committeeTree.Root(), nil
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
		newCommittee, err := o.selectNewCommittee(slot)
		if err != nil {
			panic(ierrors.Wrap(err, "error while selecting new committee"))
		}
		o.events.CommitteeSelected.Trigger(newCommittee, epoch+1)
	}
}

// IsCandidateActive returns true if the given validator is currently active.
func (o *SybilProtection) IsCandidateActive(validatorID iotago.AccountID, epoch iotago.EpochIndex) (bool, error) {
	activeCandidates, err := o.performanceTracker.EligibleValidatorCandidates(epoch)
	if err != nil {
		return false, ierrors.Wrapf(err, "failed to retrieve eligible candidates")
	}

	return activeCandidates.Has(validatorID), nil
}

// EligibleValidators returns the currently known list of recently active validator candidates for the given epoch.
func (o *SybilProtection) EligibleValidators(epoch iotago.EpochIndex) (accounts.AccountsData, error) {
	candidates, err := o.performanceTracker.EligibleValidatorCandidates(epoch)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to retrieve eligible validator candidates for epoch %d", epoch)
	}

	validators := make(accounts.AccountsData, 0)

	if err = candidates.ForEach(func(candidate iotago.AccountID) error {
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
	candidates, err := o.performanceTracker.ValidatorCandidates(epoch)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to retrieve candidates")
	}

	activeCandidates, err := o.performanceTracker.EligibleValidatorCandidates(epoch)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to retrieve eligible candidates")
	}

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

func (o *SybilProtection) selectNewCommittee(slot iotago.SlotIndex) (*account.Accounts, error) {
	timeProvider := o.apiProvider.APIForSlot(slot).TimeProvider()
	currentEpoch := timeProvider.EpochFromSlot(slot)
	nextEpoch := currentEpoch + 1
	candidates, err := o.performanceTracker.EligibleValidatorCandidates(nextEpoch)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to retrieve candidates for epoch %d", nextEpoch)
	}

	candidateAccounts := make(accounts.AccountsData, 0)
	if err := candidates.ForEach(func(candidate iotago.AccountID) error {
		accountData, exists, err := o.ledger.Account(candidate, slot)
		if err != nil {
			return err
		}
		if !exists {
			return ierrors.Errorf("account of committee candidate %s does not exist in slot %d", candidate, slot)
		}

		candidateAccounts = append(candidateAccounts, accountData)

		return nil
	}); err != nil {
		return nil, ierrors.Wrap(err, "failed to iterate through candidates")
	}

	newCommittee, err := o.seatManager.RotateCommittee(nextEpoch, candidateAccounts)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to rotate committee")
	}

	o.performanceTracker.ClearCandidates()

	return newCommittee.Accounts()
}

// WithInitialCommittee registers the passed committee on a given slot.
// This is needed to generate Genesis snapshot with some initial committee.
func WithInitialCommittee(committee accounts.AccountsData) options.Option[SybilProtection] {
	return func(o *SybilProtection) {
		o.optsInitialCommittee = committee
	}
}

func WithSeatManagerProvider(seatManagerProvider module.Provider[*engine.Engine, seatmanager.SeatManager]) options.Option[SybilProtection] {
	return func(o *SybilProtection) {
		o.optsSeatManagerProvider = seatManagerProvider
	}
}
