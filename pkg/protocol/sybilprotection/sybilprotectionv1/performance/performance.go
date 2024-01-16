package performance

import (
	"math/bits"
	"time"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/epochstore"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Tracker struct {
	rewardsStorePerEpochFunc       func(epoch iotago.EpochIndex) (kvstore.KVStore, error)
	poolStatsStore                 *epochstore.Store[*model.PoolsStats]
	committeeStore                 *epochstore.Store[*account.SeatedAccounts]
	committeeCandidatesInEpochFunc func(epoch iotago.EpochIndex) (*kvstore.TypedStore[iotago.AccountID, iotago.SlotIndex], error)
	nextEpochCommitteeCandidates   *shrinkingmap.ShrinkingMap[iotago.AccountID, iotago.SlotIndex]
	validatorPerformancesFunc      func(slot iotago.SlotIndex) (*slotstore.Store[iotago.AccountID, *model.ValidatorPerformance], error)
	latestAppliedEpoch             iotago.EpochIndex

	apiProvider iotago.APIProvider

	errHandler func(error)

	performanceFactorsMutex syncutils.RWMutex
	mutex                   syncutils.RWMutex

	log.Logger
}

func NewTracker(
	rewardsStorePerEpochFunc func(epoch iotago.EpochIndex) (kvstore.KVStore, error),
	poolStatsStore *epochstore.Store[*model.PoolsStats],
	committeeStore *epochstore.Store[*account.SeatedAccounts],
	committeeCandidatesInEpochFunc func(epoch iotago.EpochIndex) (*kvstore.TypedStore[iotago.AccountID, iotago.SlotIndex], error),
	validatorPerformancesFunc func(slot iotago.SlotIndex) (*slotstore.Store[iotago.AccountID, *model.ValidatorPerformance], error),
	latestAppliedEpoch iotago.EpochIndex,
	apiProvider iotago.APIProvider,
	errHandler func(error),
	logger log.Logger,
) *Tracker {
	return &Tracker{
		nextEpochCommitteeCandidates:   shrinkingmap.New[iotago.AccountID, iotago.SlotIndex](),
		rewardsStorePerEpochFunc:       rewardsStorePerEpochFunc,
		poolStatsStore:                 poolStatsStore,
		committeeStore:                 committeeStore,
		committeeCandidatesInEpochFunc: committeeCandidatesInEpochFunc,
		validatorPerformancesFunc:      validatorPerformancesFunc,
		latestAppliedEpoch:             latestAppliedEpoch,
		apiProvider:                    apiProvider,
		errHandler:                     errHandler,
		Logger:                         logger,
	}
}

func (t *Tracker) ClearCandidates() {
	// clean the candidate cache stored in memory to make room for candidates in the next epoch
	t.nextEpochCommitteeCandidates.Clear()
}

func (t *Tracker) TrackValidationBlock(block *blocks.Block) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	validationBlock, isValidationBlock := block.ValidationBlock()
	if !isValidationBlock {
		return
	}

	t.performanceFactorsMutex.Lock()
	defer t.performanceFactorsMutex.Unlock()
	isCommitteeMember, err := t.isCommitteeMember(block.ID().Slot(), block.ProtocolBlock().Header.IssuerID)
	if err != nil {
		t.errHandler(ierrors.Wrapf(err, "error while checking if account %s is a committee member in slot %d", block.ProtocolBlock().Header.IssuerID, block.ID().Slot()))

		return
	}

	if isCommitteeMember {
		t.trackCommitteeMemberPerformance(validationBlock, block)
	}
}

func (t *Tracker) TrackCandidateBlock(block *blocks.Block) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if block.Payload().PayloadType() != iotago.PayloadCandidacyAnnouncement {
		return
	}

	blockEpoch := t.apiProvider.APIForSlot(block.ID().Slot()).TimeProvider().EpochFromSlot(block.ID().Slot())

	var rollback bool
	t.nextEpochCommitteeCandidates.Compute(block.ProtocolBlock().Header.IssuerID, func(currentValue iotago.SlotIndex, exists bool) iotago.SlotIndex {
		if !exists || currentValue > block.ID().Slot() {
			committeeCandidatesStore, err := t.committeeCandidatesInEpochFunc(blockEpoch)
			if err != nil {
				// if there is an error, and we don't register a candidate, then we might eventually create a different commitment
				t.errHandler(ierrors.Wrapf(err, "error while retrieving candidate storage for epoch %d", blockEpoch))

				// rollback on error if entry did not exist before
				rollback = !exists

				return currentValue
			}

			err = committeeCandidatesStore.Set(block.ProtocolBlock().Header.IssuerID, block.ID().Slot())
			if err != nil {
				// if there is an error, and we don't register a candidate, then we might eventually create a different commitment
				t.errHandler(ierrors.Wrapf(err, "error while updating candidate activity for epoch %d", blockEpoch))

				// rollback on error if entry did not exist before
				rollback = !exists

				return currentValue
			}

			return block.ID().Slot()
		}

		return currentValue
	})

	// if there was an error when computing the value,
	// and it was the first entry for the given issuer, then remove the entry
	if rollback {
		t.nextEpochCommitteeCandidates.Delete(block.ProtocolBlock().Header.IssuerID)
	}

}

// EligibleValidatorCandidates returns the eligible validator candidates registered in the given epoch for the next epoch.
func (t *Tracker) EligibleValidatorCandidates(epoch iotago.EpochIndex) (ds.Set[iotago.AccountID], error) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return t.getValidatorCandidates(epoch)
}

// ValidatorCandidates returns the eligible validator candidates registered in the given epoch for the next epoch.
func (t *Tracker) ValidatorCandidates(epoch iotago.EpochIndex) (ds.Set[iotago.AccountID], error) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return t.getValidatorCandidates(epoch)
}

func (t *Tracker) getValidatorCandidates(epoch iotago.EpochIndex) (ds.Set[iotago.AccountID], error) {
	candidates := ds.NewSet[iotago.AccountID]()

	candidateStore, err := t.committeeCandidatesInEpochFunc(epoch)
	if err != nil {
		return nil, ierrors.Wrapf(err, "error while retrieving candidates for epoch %d", epoch)
	}

	if err = candidateStore.IterateKeys(kvstore.EmptyPrefix, func(accountID iotago.AccountID) bool {
		candidates.Add(accountID)

		return true
	}); err != nil {
		return nil, ierrors.Wrapf(err, "error while retrieving candidates for epoch %d", epoch)
	}

	return candidates, nil
}

func (t *Tracker) LoadCommitteeForEpoch(epoch iotago.EpochIndex) (committee *account.Accounts, exists bool) {
	c, err := t.committeeStore.Load(epoch)
	if err != nil {
		panic(ierrors.Wrapf(err, "failed to load committee for epoch %d", epoch))
	}

	if c == nil {
		return nil, false
	}

	committeeAccounts, err := c.Accounts()
	if err != nil {
		panic(ierrors.Wrapf(err, "failed to extract committee accounts for epoch %d", epoch))
	}

	return committeeAccounts, true
}

// ApplyEpoch calculates and stores pool stats and rewards for the given epoch.
func (t *Tracker) ApplyEpoch(epoch iotago.EpochIndex, committee *account.Accounts) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	timeProvider := t.apiProvider.APIForEpoch(epoch).TimeProvider()
	epochStartSlot := timeProvider.EpochStart(epoch)
	epochEndSlot := timeProvider.EpochEnd(epoch)

	profitMargin, err := t.calculateProfitMargin(committee.TotalValidatorStake(), committee.TotalStake(), epoch)
	if err != nil {
		return ierrors.Wrapf(err, "failed to calculate profit margin for epoch %d", epoch)
	}

	poolsStats := &model.PoolsStats{
		TotalStake:          committee.TotalStake(),
		TotalValidatorStake: committee.TotalValidatorStake(),
		ProfitMargin:        profitMargin,
	}

	if err = t.poolStatsStore.Store(epoch, poolsStats); err != nil {
		panic(ierrors.Wrapf(err, "failed to store pool stats for epoch %d", epoch))
	}

	rewardsMap, err := t.rewardsMap(epoch)
	if err != nil {
		panic(ierrors.Wrapf(err, "failed to create rewards tree for epoch %d", epoch))
	}

	committee.ForEach(func(accountID iotago.AccountID, pool *account.Pool) bool {
		validatorPerformances := make([]*model.ValidatorPerformance, 0, timeProvider.EpochDurationSlots())

		for slot := epochStartSlot; slot <= epochEndSlot; slot++ {
			validatorSlotPerformances, err := t.validatorPerformancesFunc(slot)
			if err != nil {
				validatorPerformances = append(validatorPerformances, nil)
				continue
			}

			validatorPerformance, exists, err := validatorSlotPerformances.Load(accountID)
			if err != nil {
				panic(ierrors.Wrapf(err, "failed to load performance factor for account %s", accountID))
			}

			// key not found
			if !exists {
				validatorPerformance = model.NewValidatorPerformance()
			}

			validatorPerformances = append(validatorPerformances, validatorPerformance)
		}

		// Aggregate the performance factor of the epoch which approximates the average of the slot's performance factor.
		epochPerformanceFactor := t.aggregatePerformanceFactors(validatorPerformances, epoch)

		if epochPerformanceFactor == 0 {
			// no rewards for this pool, we do not set pool rewards at all,
			// to differientiate between situation when poolReward == fixedCost (no reward for delegators)

			return true
		}

		poolReward, err := t.poolReward(
			epochEndSlot,
			committee.TotalValidatorStake(),
			committee.TotalStake(),
			pool.PoolStake,
			pool.ValidatorStake,
			epochPerformanceFactor,
		)
		if err != nil {
			panic(ierrors.Wrapf(err, "failed to calculate pool rewards for account %s", accountID))
		}

		t.LogInfo("PerformanceApplyEpoch", "accountID", accountID, "epochPerformanceFactor", epochPerformanceFactor, "poolReward", poolReward)

		if err = rewardsMap.Set(accountID, &model.PoolRewards{
			PoolStake:   pool.PoolStake,
			PoolRewards: poolReward,
			FixedCost:   pool.FixedCost,
		}); err != nil {
			panic(ierrors.Wrapf(err, "failed to set rewards for account %s", accountID))
		}

		return true
	})

	if err = rewardsMap.Commit(); err != nil {
		panic(ierrors.Wrapf(err, "failed to commit rewards for epoch %d", epoch))
	}

	t.latestAppliedEpoch = epoch

	return nil
}

// aggregatePerformanceFactors calculates epoch performance factor of a validator based on its performance in each slot by summing up all active subslots.
func (t *Tracker) aggregatePerformanceFactors(slotActivityVector []*model.ValidatorPerformance, epoch iotago.EpochIndex) uint64 {
	if len(slotActivityVector) == 0 {
		return 0
	}

	protoParamsForEpoch := t.apiProvider.APIForEpoch(epoch).ProtocolParameters()

	var epochPerformanceFactor uint64
	for _, pf := range slotActivityVector {
		// no activity in a slot
		if pf == nil {
			continue
		}

		// each one bit represents at least one block issued in that subslot,
		// we reward not only total number of blocks issued, but also regularity based on block timestamp
		slotPerformanceFactor := bits.OnesCount32(pf.SlotActivityVector)

		if pf.BlocksIssuedCount > protoParamsForEpoch.ValidationBlocksPerSlot() {
			// we harshly punish validators that issue any blocks more than allowed

			return 0
		}

		epochPerformanceFactor += uint64(slotPerformanceFactor)
	}

	return epochPerformanceFactor >> uint64(protoParamsForEpoch.SlotsPerEpochExponent())
}

func (t *Tracker) isCommitteeMember(slot iotago.SlotIndex, accountID iotago.AccountID) (bool, error) {
	epoch := t.apiProvider.APIForEpoch(t.latestAppliedEpoch).TimeProvider().EpochFromSlot(slot)
	committee, exists := t.LoadCommitteeForEpoch(epoch)
	if !exists {
		return false, ierrors.Errorf("committee for epoch %d not found", epoch)
	}

	return committee.Has(accountID), nil
}

func (t *Tracker) trackCommitteeMemberPerformance(validationBlock *iotago.ValidationBlockBody, block *blocks.Block) {
	validatorPerformances, err := t.validatorPerformancesFunc(block.ID().Slot())
	if err != nil {
		t.errHandler(ierrors.Errorf("failed to load performance factor for slot %s", block.ID().Slot()))

		return
	}

	validatorPerformance, exists, err := validatorPerformances.Load(block.ProtocolBlock().Header.IssuerID)
	if err != nil {
		t.errHandler(ierrors.Errorf("failed to load performance factor for account %s", block.ProtocolBlock().Header.IssuerID))

		return
	}

	// No validator performance for this slot yet.
	if !exists {
		validatorPerformance = model.NewValidatorPerformance()
	}

	// Set a bit at subslotIndex to 1 to indicate activity in that subslot.
	validatorPerformance.SlotActivityVector = validatorPerformance.SlotActivityVector | (1 << t.subslotIndex(block.ID().Slot(), block.ProtocolBlock().Header.IssuingTime))

	apiForSlot := t.apiProvider.APIForSlot(block.ID().Slot())

	// We restrict the number up to ValidationBlocksPerSlot + 1 to know later if the validator issued
	// more blocks than allowed and be able to punish for it.
	// Also it can fit into uint8.
	if validatorPerformance.BlocksIssuedCount < apiForSlot.ProtocolParameters().ValidationBlocksPerSlot()+1 {
		validatorPerformance.BlocksIssuedCount++
	}

	validatorPerformance.HighestSupportedVersionAndHash = model.VersionAndHash{
		Version: validationBlock.HighestSupportedVersion,
		Hash:    validationBlock.ProtocolParametersHash,
	}

	if err = validatorPerformances.Store(block.ProtocolBlock().Header.IssuerID, validatorPerformance); err != nil {
		t.errHandler(ierrors.Errorf("failed to store performance factor for account %s", block.ProtocolBlock().Header.IssuerID))
	}
}

// subslotIndex returns the index for timestamp corresponding to subslot created dividing slot on validationBlocksPerSlot equal parts.
func (t *Tracker) subslotIndex(slot iotago.SlotIndex, issuingTime time.Time) int {
	epochAPI := t.apiProvider.APIForEpoch(t.latestAppliedEpoch)
	valBlocksNum := epochAPI.ProtocolParameters().ValidationBlocksPerSlot()
	subslotDur := time.Duration(epochAPI.TimeProvider().SlotDurationSeconds()) * time.Second / time.Duration(valBlocksNum)
	slotStart := epochAPI.TimeProvider().SlotStartTime(slot)

	return int(issuingTime.Sub(slotStart) / subslotDur)
}
