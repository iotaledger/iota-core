package performance

import (
	"math/bits"
	"time"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type Tracker struct {
	rewardBaseStore kvstore.KVStore
	poolStatsStore  *kvstore.TypedStore[iotago.EpochIndex, *PoolsStats]
	committeeStore  *kvstore.TypedStore[iotago.EpochIndex, *account.Accounts]

	registeredValidatorsFunc     func(index iotago.SlotIndex) *prunable.RegisteredValidatorSlotActivity
	validatorSlotPerformanceFunc func(slot iotago.SlotIndex) *prunable.ValidatorSlotPerformance
	latestAppliedEpoch           iotago.EpochIndex

	apiProvider api.Provider

	errHandler func(error)

	performanceFactorsMutex syncutils.RWMutex
	mutex                   syncutils.RWMutex
}

func NewTracker(
	rewardsBaseStore kvstore.KVStore,
	poolStatsStore kvstore.KVStore,
	committeeStore kvstore.KVStore,
	registeredValidatorsFunc func(index iotago.SlotIndex) *prunable.RegisteredValidatorSlotActivity,
	performanceFactorsFunc func(slot iotago.SlotIndex) *prunable.ValidatorSlotPerformance,
	latestAppliedEpoch iotago.EpochIndex,
	apiProvider api.Provider,
	errHandler func(error),
) *Tracker {
	return &Tracker{
		rewardBaseStore: rewardsBaseStore,
		poolStatsStore: kvstore.NewTypedStore(poolStatsStore,
			iotago.EpochIndex.Bytes,
			iotago.EpochIndexFromBytes,
			(*PoolsStats).Bytes,
			PoolsStatsFromBytes,
		),
		committeeStore: kvstore.NewTypedStore(committeeStore,
			iotago.EpochIndex.Bytes,
			iotago.EpochIndexFromBytes,
			(*account.Accounts).Bytes,
			account.AccountsFromBytes,
		),
		registeredValidatorsFunc:     registeredValidatorsFunc,
		validatorSlotPerformanceFunc: performanceFactorsFunc,
		latestAppliedEpoch:           latestAppliedEpoch,
		apiProvider:                  apiProvider,
		errHandler:                   errHandler,
	}
}

func (t *Tracker) RegisterCommittee(epoch iotago.EpochIndex, committee *account.Accounts) error {
	return t.committeeStore.Set(epoch, committee)
}

// TODO: doesnt need to to be issued by committee member, when you register you send a validation block as activity proof
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

		return
	}
	// not a committee member, just activity proof for registration
	t.trackRegisteredValidatorActivity(validatorBlock, block)
}

func (t *Tracker) trackRegisteredValidatorActivity(validationBlock *iotago.ValidationBlock, block *blocks.Block) {
	epoch := t.apiProvider.APIForSlot(block.ID().Index()).TimeProvider().EpochFromSlot(block.ID().Index())
	epochEnd := t.apiProvider.APIForEpoch(epoch).TimeProvider().EpochEnd(epoch)
	nearingThreshold := t.apiProvider.APIForEpoch(epoch).ProtocolParameters().EpochNearingThreshold()
	activityWindowDuration := t.apiProvider.APIForEpoch(epoch).ProtocolParameters().ActivityWindowDuration()
	// we track activity for [nextEpochEnd-EpochNearingThreshold-ActivityWindowDuration, nextEpochEnd-EpochNearingThreshold]
	// only for registered already validators
	// TODO: shouild I check If committee for nextEpoch was not selected yet?
	if epochEnd <= block.ID().Index()+nearingThreshold+activityWindowDuration && block.ID().Index()+activityWindowDuration < epochEnd {
		// TODO use epochs after merging pruning PR changes
		//nextEpoch := epoch + 1
		//registeredStore := t.registeredValidatorsFunc(t.apiProvider.APIForEpoch(nextEpoch).TimeProvider().EpochStart(epoch))
		// check if validator has registered for the next epoch, if yes: update registeredStore with active: true and
	}
}

func (t *Tracker) trackCommitteeMemberPerformance(validationBlock *iotago.ValidationBlock, block *blocks.Block) {
	validatorSlotPerformanceStore := t.validatorSlotPerformanceFunc(block.ID().Index())
	validatorPerformance, err := validatorSlotPerformanceStore.Load(block.ProtocolBlock().IssuerID)
	if err != nil {
		t.errHandler(ierrors.Errorf("failed to load performance factor for account %s", block.ProtocolBlock().IssuerID))
	}
	// key not found
	if validatorPerformance == nil {
		validatorPerformance = &prunable.ValidatorPerformance{}
	}
	updatedPerformance := t.updateSlotPerformanceBitMap(validatorPerformance, block.ID().Index(), block.ProtocolBlock().IssuingTime)

	if updatedPerformance.BlockIssuedCount == t.apiProvider.APIForSlot(block.ID().Index()).ProtocolParameters().RewardsParameters().ValidatorBlocksPerSlot {
		// no need to store larger number and we can fit into uint8
		updatedPerformance.BlockIssuedCount = t.apiProvider.APIForSlot(block.ID().Index()).ProtocolParameters().RewardsParameters().ValidatorBlocksPerSlot + 1
	} else {
		updatedPerformance.BlockIssuedCount++
	}
	updatedPerformance.HighestSupportedVersionAndHash = iotago.VersionAndHash{
		Version: validationBlock.HighestSupportedVersion,
		Hash:    validationBlock.ProtocolParametersHash,
	}

	err = validatorSlotPerformanceStore.Store(block.ProtocolBlock().IssuerID, updatedPerformance)
	if err != nil {
		t.errHandler(ierrors.Errorf("failed to store performance factor for account %s", block.ProtocolBlock().IssuerID))
	}
}

func (t *Tracker) ValidatorPerformance(slot iotago.SlotIndex, accountID iotago.AccountID) (*prunable.RegisteredValidatorActivity, error) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	t.performanceFactorsMutex.RLock()
	defer t.performanceFactorsMutex.RUnlock()

	// TODO finish after refactor of pruning store
	//validatorSlotPerformanceStore := t.registeredValidatorsFunc(slot)
	return nil, nil
}

func (t *Tracker) updateSlotPerformanceBitMap(pf *prunable.ValidatorPerformance, slotIndex iotago.SlotIndex, issuingTime time.Time) *prunable.ValidatorPerformance {
	if pf == nil {
		return pf
	}
	subslotIndex := t.subslotIndex(slotIndex, issuingTime)
	// set bit at subslotIndex to 1 to indicate activity in that subslot
	pf.SlotActivityVector = pf.SlotActivityVector | (1 << subslotIndex)

	return pf
}

// subslotIndex returns the index for timestamp corresponding to subslot created dividing slot on validatorBlocksPerSlot equal parts.
func (t *Tracker) subslotIndex(slot iotago.SlotIndex, issuingTime time.Time) int {
	valBlocksNum := t.apiProvider.APIForEpoch(t.latestAppliedEpoch).ProtocolParameters().RewardsParameters().ValidatorBlocksPerSlot
	subslotDur := time.Duration(t.apiProvider.APIForEpoch(t.latestAppliedEpoch).TimeProvider().SlotDurationSeconds()) * time.Second / time.Duration(valBlocksNum)
	slotStart := t.apiProvider.APIForEpoch(t.latestAppliedEpoch).TimeProvider().SlotStartTime(slot)

	return int(issuingTime.Sub(slotStart) / subslotDur)
}

func (t *Tracker) ApplyEpoch(epoch iotago.EpochIndex, committee *account.Accounts) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	timeProvider := t.apiProvider.APIForEpoch(epoch).TimeProvider()
	epochStartSlot := timeProvider.EpochStart(epoch)
	epochEndSlot := timeProvider.EpochEnd(epoch)

	profitMargin := t.calculateProfitMargin(committee.TotalValidatorStake(), committee.TotalStake(), epoch)
	poolsStats := &PoolsStats{
		TotalStake:          committee.TotalStake(),
		TotalValidatorStake: committee.TotalValidatorStake(),
		ProfitMargin:        profitMargin,
	}

	if err := t.poolStatsStore.Set(epoch, poolsStats); err != nil {
		panic(ierrors.Wrapf(err, "failed to store pool stats for epoch %d", epoch))
	}

	rewardsTree := ads.NewMap(t.rewardsStorage(epoch),
		iotago.Identifier.Bytes,
		iotago.IdentifierFromBytes,
		(*PoolRewards).Bytes,
		PoolRewardsFromBytes,
	)

	committee.ForEach(func(accountID iotago.AccountID, pool *account.Pool) bool {
		validatorPerformances := make([]*prunable.ValidatorPerformance, 0, epochEndSlot+1-epochStartSlot)
		for slot := epochStartSlot; slot <= epochEndSlot; slot++ {
			performanceFactorStorage := t.validatorSlotPerformanceFunc(slot)
			if performanceFactorStorage == nil {
				validatorPerformances = append(validatorPerformances, &prunable.ValidatorPerformance{})

				continue
			}

			validatorPerformance, err := performanceFactorStorage.Load(accountID)
			if err != nil {
				panic(ierrors.Wrapf(err, "failed to load performance factor for account %s", accountID))
			}

			validatorPerformances = append(validatorPerformances, validatorPerformance)
		}

		poolReward, err := t.poolReward(epochEndSlot, committee.TotalValidatorStake(), committee.TotalStake(), pool.PoolStake, pool.ValidatorStake, pool.FixedCost, t.aggregatePerformanceFactors(validatorPerformances, epoch))
		if err != nil {
			panic(ierrors.Wrapf(err, "failed to calculate pool rewards for account %s", accountID))
		}
		if err = rewardsTree.Set(accountID, &PoolRewards{
			PoolStake:   pool.PoolStake,
			PoolRewards: poolReward,
			FixedCost:   pool.FixedCost,
		}); err != nil {
			panic(ierrors.Wrapf(err, "failed to set rewards for account %s", accountID))
		}

		return true
	})

	if err := rewardsTree.Commit(); err != nil {
		panic(ierrors.Wrapf(err, "failed to commit rewards for epoch %d", epoch))
	}

	t.latestAppliedEpoch = epoch
}

func (t *Tracker) EligibleValidatorCandidates(epoch iotago.EpochIndex) ds.Set[iotago.AccountID] {
	//epochStart := t.apiProvider.APIForEpoch(epoch).TimeProvider().EpochStart(epoch)
	//registeredStore := t.registeredValidatorsFunc(epochStart)
	//eligible := ds.NewSet[iotago.AccountID]()
	//registeredStore.ForEach(func(accountID iotago.AccountID, a *prunable.RegisteredValidatorActivity) bool {
	//	if a.Active {
	//		eligible.Add(accountID)
	//	}
	//	return true
	//}

	return nil
}

// ValidatorCandidates returns the registered validator candidates for the given epoch.
func (t *Tracker) ValidatorCandidates(_ iotago.EpochIndex) ds.Set[iotago.AccountID] {
	// TODO: we should choose candidates we tracked performance for no matter if they were active

	return ds.NewSet[iotago.AccountID]()
}

func (t *Tracker) LoadCommitteeForEpoch(epoch iotago.EpochIndex) (committee *account.Accounts, exists bool) {
	c, err := t.committeeStore.Get(epoch)
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, false
		}
		panic(ierrors.Wrapf(err, "failed to load committee for epoch %d", epoch))
	}

	return c, true
}

// aggregatePerformanceFactors calculates epoch performance factor of a validator based on its performance in each slot by summing up all active subslots.
func (t *Tracker) aggregatePerformanceFactors(slotActivityVector []*prunable.ValidatorPerformance, epoch iotago.EpochIndex) uint64 {
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

	return epochPerformanceFactor
}

func (t *Tracker) isCommitteeMember(slot iotago.SlotIndex, accountID iotago.AccountID) (bool, error) {
	epoch := t.apiProvider.APIForEpoch(t.latestAppliedEpoch).TimeProvider().EpochFromSlot(slot)
	committee, exists := t.LoadCommitteeForEpoch(epoch)
	if !exists {
		return false, ierrors.Errorf("committee for epoch %d not found", epoch)
	}
	return committee.Has(accountID), nil
}
