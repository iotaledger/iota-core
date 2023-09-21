package performance

import (
	"math/bits"
	"time"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/epochstore"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Tracker struct {
	rewardsStorePerEpochFunc func(epoch iotago.EpochIndex) (kvstore.KVStore, error)
	poolStatsStore           *epochstore.Store[*model.PoolsStats]
	committeeStore           *epochstore.Store[*account.Accounts]

	validatorPerformancesFunc func(slot iotago.SlotIndex) (*slotstore.Store[iotago.AccountID, *model.ValidatorPerformance], error)
	latestAppliedEpoch        iotago.EpochIndex

	apiProvider iotago.APIProvider

	errHandler func(error)

	performanceFactorsMutex syncutils.RWMutex
	mutex                   syncutils.RWMutex
}

func NewTracker(
	rewardsStorePerEpochFunc func(epoch iotago.EpochIndex) (kvstore.KVStore, error),
	poolStatsStore *epochstore.Store[*model.PoolsStats],
	committeeStore *epochstore.Store[*account.Accounts],
	validatorPerformancesFunc func(slot iotago.SlotIndex) (*slotstore.Store[iotago.AccountID, *model.ValidatorPerformance], error),
	latestAppliedEpoch iotago.EpochIndex,
	apiProvider iotago.APIProvider,
	errHandler func(error),
) *Tracker {
	return &Tracker{
		rewardsStorePerEpochFunc:  rewardsStorePerEpochFunc,
		poolStatsStore:            poolStatsStore,
		committeeStore:            committeeStore,
		validatorPerformancesFunc: validatorPerformancesFunc,
		latestAppliedEpoch:        latestAppliedEpoch,
		apiProvider:               apiProvider,
		errHandler:                errHandler,
	}
}

func (t *Tracker) RegisterCommittee(epoch iotago.EpochIndex, committee *account.Accounts) error {
	return t.committeeStore.Store(epoch, committee)
}

func (t *Tracker) TrackValidationBlock(block *blocks.Block) {
	validatorBlock, isValidationBlock := block.ValidationBlock()
	if !isValidationBlock {
		return
	}

	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.performanceFactorsMutex.Lock()
	defer t.performanceFactorsMutex.Unlock()
	isCommitteeMember, err := t.isCommitteeMember(block.ID().Index(), block.ProtocolBlock().IssuerID)
	if err != nil {
		t.errHandler(ierrors.Errorf("failed to check if account %s is committee member", block.ProtocolBlock().IssuerID))

		return
	}

	if isCommitteeMember {
		t.trackCommitteeMemberPerformance(validatorBlock, block)
	}
}

func (t *Tracker) EligibleValidatorCandidates(epoch iotago.EpochIndex) ds.Set[iotago.AccountID] {
	// TODO: to be implemented for 1.1, for now we just pick previous committee

	eligible := ds.NewSet[iotago.AccountID]()

	lo.PanicOnErr(t.committeeStore.Load(epoch - 1)).ForEach(func(accountID iotago.AccountID, _ *account.Pool) bool {
		eligible.Add(accountID)

		return true
	})

	return eligible

	//epochStart := t.apiProvider.APIForEpoch(epoch).TimeProvider().EpochStart(epoch)
	//registeredStore := t.registeredValidatorsFunc(epochStart)
	//eligible := ds.NewSet[iotago.AccountID]()
	//registeredStore.ForEach(func(accountID iotago.AccountID, a *prunable.RegisteredValidatorActivity) bool {
	//	if a.Active {
	//		eligible.Add(accountID)
	//	}
	//	return true
	//}
}

// ValidatorCandidates returns the registered validator candidates for the given epoch.
func (t *Tracker) ValidatorCandidates(_ iotago.EpochIndex) ds.Set[iotago.AccountID] {
	// TODO: we should choose candidates we tracked performance for no matter if they were active

	return ds.NewSet[iotago.AccountID]()
}

func (t *Tracker) LoadCommitteeForEpoch(epoch iotago.EpochIndex) (committee *account.Accounts, exists bool) {
	c, err := t.committeeStore.Load(epoch)
	if err != nil {
		panic(ierrors.Wrapf(err, "failed to load committee for epoch %d", epoch))
	}
	if c == nil {
		return nil, false
	}

	return c, true
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
		validatorPerformances := make([]*model.ValidatorPerformance, timeProvider.EpochDurationSlots())
		for slot := epochStartSlot; slot <= epochEndSlot; slot++ {
			validatorSlotPerformances, err := t.validatorPerformancesFunc(slot)
			if err != nil {
				validatorPerformances = append(validatorPerformances, nil)

				continue
			}

			validatorPerformance, err := validatorSlotPerformances.Load(accountID)
			if err != nil {
				panic(ierrors.Wrapf(err, "failed to load performance factor for account %s", accountID))
			}

			validatorPerformances = append(validatorPerformances, validatorPerformance)
		}
		pf := t.aggregatePerformanceFactors(validatorPerformances, epoch)
		if pf == 0 {
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
			pf,
		)
		if err != nil {
			panic(ierrors.Wrapf(err, "failed to calculate pool rewards for account %s", accountID))
		}
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

	var epochPerformanceFactor uint64
	for _, pf := range slotActivityVector {
		// no activity in a slot
		if pf == nil {
			continue
		}
		// each one bit represents at least one block issued in that subslot,
		// we reward not only total number of blocks issued, but also regularity based on block timestamp
		slotPerformanceFactor := bits.OnesCount32(pf.SlotActivityVector)

		if pf.BlockIssuedCount > t.apiProvider.APIForEpoch(epoch).ProtocolParameters().RewardsParameters().ValidatorBlocksPerSlot {
			// we harshly punish validators that issue any blocks more than allowed

			return 0
		}

		epochPerformanceFactor += uint64(slotPerformanceFactor)
	}

	return epochPerformanceFactor >> uint64(t.apiProvider.CurrentAPI().ProtocolParameters().TimeProvider().SlotsPerEpochExponent())
}

func (t *Tracker) isCommitteeMember(slot iotago.SlotIndex, accountID iotago.AccountID) (bool, error) {
	epoch := t.apiProvider.APIForEpoch(t.latestAppliedEpoch).TimeProvider().EpochFromSlot(slot)
	committee, exists := t.LoadCommitteeForEpoch(epoch)
	if !exists {
		return false, ierrors.Errorf("committee for epoch %d not found", epoch)
	}

	return committee.Has(accountID), nil
}

func (t *Tracker) trackCommitteeMemberPerformance(validationBlock *iotago.ValidationBlock, block *blocks.Block) {
	validatorPerformances, err := t.validatorPerformancesFunc(block.ID().Index())
	if err != nil {
		t.errHandler(ierrors.Errorf("failed to load performance factor for slot %s", block.ID().Index()))

		return
	}

	validatorPerformance, err := validatorPerformances.Load(block.ProtocolBlock().IssuerID)
	if err != nil {
		t.errHandler(ierrors.Errorf("failed to load performance factor for account %s", block.ProtocolBlock().IssuerID))
	}
	// key not found
	if validatorPerformance == nil {
		validatorPerformance = model.NewValidatorPerformance()
	}

	// set bit at subslotIndex to 1 to indicate activity in that subslot
	validatorPerformance.SlotActivityVector = validatorPerformance.SlotActivityVector | (1 << t.subslotIndex(block.ID().Index(), block.ProtocolBlock().IssuingTime))

	apiForSlot := t.apiProvider.APIForSlot(block.ID().Index())
	// we restrict the number up to ValidatorBlocksPerSlot + 1 to know later if the validator issued more blocks than allowed and be able to punish for it
	// also it can fint into uint8
	if validatorPerformance.BlockIssuedCount < apiForSlot.ProtocolParameters().RewardsParameters().ValidatorBlocksPerSlot+1 {
		validatorPerformance.BlockIssuedCount++
	}
	validatorPerformance.HighestSupportedVersionAndHash = model.VersionAndHash{
		Version: validationBlock.HighestSupportedVersion,
		Hash:    validationBlock.ProtocolParametersHash,
	}
	if err = validatorPerformances.Store(block.ProtocolBlock().IssuerID, validatorPerformance); err != nil {
		t.errHandler(ierrors.Errorf("failed to store performance factor for account %s", block.ProtocolBlock().IssuerID))
	}
}

// subslotIndex returns the index for timestamp corresponding to subslot created dividing slot on validatorBlocksPerSlot equal parts.
func (t *Tracker) subslotIndex(slot iotago.SlotIndex, issuingTime time.Time) int {
	epochAPI := t.apiProvider.APIForEpoch(t.latestAppliedEpoch)
	valBlocksNum := epochAPI.ProtocolParameters().RewardsParameters().ValidatorBlocksPerSlot
	subslotDur := time.Duration(epochAPI.TimeProvider().SlotDurationSeconds()) * time.Second / time.Duration(valBlocksNum)
	slotStart := epochAPI.TimeProvider().SlotStartTime(slot)

	return int(issuingTime.Sub(slotStart) / subslotDur)
}
