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

	validatorSlotPerformanceFunc func(slot iotago.SlotIndex) *prunable.VlidatorSlotPerformance
	latestAppliedEpoch           iotago.EpochIndex

	apiProvider api.Provider

	performanceFactorsMutex syncutils.RWMutex
	mutex                   syncutils.RWMutex
}

func NewTracker(
	rewardsBaseStore kvstore.KVStore,
	poolStatsStore kvstore.KVStore,
	committeeStore kvstore.KVStore,
	performanceFactorsFunc func(slot iotago.SlotIndex) *prunable.VlidatorSlotPerformance,
	latestAppliedEpoch iotago.EpochIndex,
	apiProvider api.Provider,
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
		validatorSlotPerformanceFunc: performanceFactorsFunc,
		latestAppliedEpoch:           latestAppliedEpoch,
		apiProvider:                  apiProvider,
	}
}

func (t *Tracker) RegisterCommittee(epoch iotago.EpochIndex, committee *account.Accounts) error {
	return t.committeeStore.Set(epoch, committee)
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

	validatorSlotPerformanceStore := t.validatorSlotPerformanceFunc(block.ID().Index())
	validatorPerformance, err := validatorSlotPerformanceStore.Load(block.ProtocolBlock().IssuerID)
	if err != nil {
		// TODO replace panic with errors in the future, like triggering an error event
		panic(ierrors.Errorf("failed to load performance factor for account %s", block.ProtocolBlock().IssuerID))
	}
	updatedPerformance := t.updateSlotPerformanceBitMap(validatorPerformance, block.ID().Index(), block.ProtocolBlock().IssuingTime)
	if updatedPerformance.BlockIssuedCount == validatorBlocksPerSlot {
		// no need to store larger number and we can fit into uint8
		updatedPerformance.BlockIssuedCount = validatorBlocksPerSlot + 1
	} else {
		updatedPerformance.BlockIssuedCount++
	}
	updatedPerformance.HighestSupportedVersion = validatorBlock.HighestSupportedVersion

	err = validatorSlotPerformanceStore.Store(block.ProtocolBlock().IssuerID, updatedPerformance)
	if err != nil {
		// TODO replace panic with errors in the future, like triggering an error event
		panic(ierrors.Errorf("failed to store performance factor for account %s", block.ProtocolBlock().IssuerID))
	}
}

func (t *Tracker) updateSlotPerformanceBitMap(pf *prunable.ValidatorPerformance, slotIndex iotago.SlotIndex, issuingTime time.Time) *prunable.ValidatorPerformance {
	subslotIndex := t.subslotIndex(slotIndex, issuingTime)
	// set bit at subslotIndex to 1 to indicate activity in that subslot
	pf.SlotActivityVector = pf.SlotActivityVector | (1 << subslotIndex)

	return pf
}

// subslotIndex returns the index for timestamp corresponding to subslot created dividing slot on validatorBlocksPerSlot equal parts.
func (t *Tracker) subslotIndex(slot iotago.SlotIndex, issuingTime time.Time) int {
	valBlocksNum := t.apiProvider.APIForEpoch(t.latestAppliedEpoch).ProtocolParameters().ValidatorBlocksPerSlot()
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

	profitMargin := calculateProfitMargin(committee.TotalValidatorStake(), committee.TotalStake())
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

		if err := rewardsTree.Set(accountID, &PoolRewards{
			PoolStake:   pool.PoolStake,
			PoolRewards: t.poolReward(epochEndSlot, committee.TotalValidatorStake(), committee.TotalStake(), pool.PoolStake, pool.ValidatorStake, pool.FixedCost, t.aggregatePerformanceFactors(validatorPerformances)),
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

func (t *Tracker) EligibleValidatorCandidates(_ iotago.EpochIndex) ds.Set[iotago.AccountID] {
	// TODO: we should choose candidates we tracked performance for, only active

	return ds.NewSet[iotago.AccountID]()
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

func (t *Tracker) aggregatePerformanceFactors(slotActivityVector []*prunable.ValidatorPerformance) uint64 {
	if len(slotActivityVector) == 0 {
		return 0
	}

	var epochPerformanceFactor uint64
	for _, pf := range slotActivityVector {
		// each one bit represents at least one block issued in that subslot,
		// we reward not only total number of blocks issued, but also regularity based on block timestamp
		slotPerformanceFactor := bits.OnesCount32(pf.SlotActivityVector)

		if pf.BlockIssuedCount > validatorBlocksPerSlot {
			// we harshly punish validators that issue any blocks more than allowed
			return 0
		}
		epochPerformanceFactor += uint64(slotPerformanceFactor)
	}

	return epochPerformanceFactor
}
