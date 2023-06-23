package rewards

import (
	"sync"

	"github.com/cockroachdb/errors"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Manager struct {
	rewardBaseStore kvstore.KVStore
	poolStatsStore  kvstore.KVStore

	performanceFactorsFunc func(slot iotago.SlotIndex) *prunable.PerformanceFactors

	// Cache to allow atomic increase operations protected by a lock
	performanceFactorsCache *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *prunable.PerformanceFactors]
	performanceFactorsMutex sync.RWMutex

	timeProvider *iotago.TimeProvider

	decayProvider *iotago.ManaDecayProvider

	mutex sync.RWMutex
}

func New(
	rewardsBaseStore,
	poolStatsStore kvstore.KVStore,
	performanceFactorsFunc func(slot iotago.SlotIndex) *prunable.PerformanceFactors,
	timeProvider *iotago.TimeProvider,
	decayProvider *iotago.ManaDecayProvider,
) *Manager {
	return &Manager{
		rewardBaseStore:         rewardsBaseStore,
		poolStatsStore:          poolStatsStore,
		performanceFactorsCache: shrinkingmap.New[iotago.SlotIndex, *prunable.PerformanceFactors](),
		performanceFactorsFunc:  performanceFactorsFunc,
		timeProvider:            timeProvider,
		decayProvider:           decayProvider,
	}
}

func (m *Manager) rewardsStorage(epochIndex iotago.EpochIndex) kvstore.KVStore {
	return lo.PanicOnErr(m.rewardBaseStore.WithExtendedRealm(epochIndex.Bytes()))
}

func (m *Manager) BlockAccepted(block *blocks.Block) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.performanceFactorsMutex.Lock()
	defer m.performanceFactorsMutex.Unlock()

	// TODO: check if this block is a validator block

	performanceFactors, _ := m.performanceFactorsCache.GetOrCreate(block.ID().Index(), func() *prunable.PerformanceFactors {
		return m.performanceFactorsFunc(block.ID().Index())
	})
	pf, err := performanceFactors.Load(block.Block().IssuerID)
	if err != nil {
		panic(err)
	}

	err = performanceFactors.Store(block.Block().IssuerID, pf+1)
	if err != nil {
		panic(err)
	}
}

func (m *Manager) evictPerformanceFactors(startSlot, endSlot iotago.SlotIndex) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for index := startSlot; index <= endSlot; index++ {
		m.performanceFactorsCache.Delete(index)
	}
}

func (m *Manager) RewardsRoot(epochIndex iotago.EpochIndex) iotago.Identifier {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return iotago.Identifier(ads.NewMap[iotago.AccountID, RewardsForAccount](m.rewardsStorage(epochIndex)).Root())
}

func (m *Manager) ApplyEpoch(epochIndex iotago.EpochIndex, poolStakes map[iotago.AccountID]*Pool) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	rewardsTree := ads.NewMap[iotago.AccountID, RewardsForAccount](m.rewardsStorage(epochIndex))

	epochSlotStart := m.timeProvider.EpochStart(epochIndex)
	epochSlotEnd := m.timeProvider.EpochEnd(epochIndex)

	var totalStake uint64
	var totalValidatorStake uint64

	for _, pool := range poolStakes {
		totalStake += pool.PoolStake
		totalValidatorStake += pool.ValidatorStake
	}

	profitMargin := calculateProfitMargin(totalValidatorStake, totalStake)
	poolsStats := PoolsStats{
		TotalStake:          totalStake,
		TotalValidatorStake: totalValidatorStake,
		ProfitMargin:        profitMargin,
	}

	err := m.poolStatsStore.Set(epochIndex.Bytes(), lo.PanicOnErr(poolsStats.Bytes()))
	if err != nil {
		return errors.Wrapf(err, "failed to store pool stats for epoch %d", epochIndex)
	}

	for accountID, pool := range poolStakes {
		intermediateFactors := make([]uint64, 0)
		for slot := epochSlotStart; slot <= epochSlotEnd; slot++ {
			performanceFactorStorage, _ := m.performanceFactorsCache.Get(slot)
			if performanceFactorStorage == nil {
				intermediateFactors = append(intermediateFactors, 0)
			}

			pf, err2 := performanceFactorStorage.Load(accountID)
			if err2 != nil {
				return errors.Wrapf(err, "failed to load performance factor for account %s", accountID)
			}

			intermediateFactors = append(intermediateFactors, pf)

		}

		rewardsTree.Set(accountID, &RewardsForAccount{
			PoolStake:   pool.PoolStake,
			PoolRewards: poolReward(totalValidatorStake, totalStake, profitMargin, pool.FixedCost, aggregatePerformanceFactors(intermediateFactors)),
			FixedCost:   pool.FixedCost,
		})
	}
	m.evictPerformanceFactors(epochSlotStart, epochSlotEnd)

	return nil
}

func (m *Manager) ValidatorReward(validatorID iotago.AccountID, stakeAmount uint64, epochStart, epochEnd iotago.EpochIndex) (validatorReward uint64, err error) {
	for epochIndex := epochStart; epochIndex <= epochEnd; epochIndex++ {
		rewardsForAccountInEpoch, exists := m.rewardsForAccount(validatorID, epochIndex)
		if !exists {
			continue
		}

		poolStats, err := m.poolStats(epochIndex)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to get pool stats for epoch %d", epochIndex)
		}

		unDecayedEpochRewards := rewardsForAccountInEpoch.FixedCost +
			((poolStats.ProfitMargin * rewardsForAccountInEpoch.PoolRewards) >> 8) +
			((((1<<8)-poolStats.ProfitMargin)*rewardsForAccountInEpoch.PoolRewards)>>8)*
				stakeAmount/
				rewardsForAccountInEpoch.PoolStake

		decayedEpochRewards := m.decayProvider.RewardsWithDecay(unDecayedEpochRewards, epochIndex, epochEnd)
		validatorReward += decayedEpochRewards
	}

	return validatorReward, nil
}

func (m *Manager) DelegatorReward(validatorID iotago.AccountID, delegatedAmount uint64, epochStart, epochEnd iotago.EpochIndex) (delegatorsReward uint64, err error) {
	for epochIndex := epochStart; epochIndex <= epochEnd; epochIndex++ {
		rewardsForAccountInEpoch, exists := m.rewardsForAccount(validatorID, epochIndex)
		if !exists {
			continue
		}

		poolStats, err := m.poolStats(epochIndex)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to get pool stats for epoch %d", epochIndex)
		}

		unDecayedEpochRewards := ((((1 << 8) - poolStats.ProfitMargin) * rewardsForAccountInEpoch.PoolRewards) >> 8) *
			delegatedAmount /
			rewardsForAccountInEpoch.PoolStake

		decayedEpochRewards := m.decayProvider.RewardsWithDecay(unDecayedEpochRewards, epochIndex, epochEnd)
		delegatorsReward += decayedEpochRewards
	}

	return delegatorsReward, nil
}

func (m *Manager) rewardsForAccount(accountID iotago.AccountID, epochIndex iotago.EpochIndex) (rewardsForAccount *RewardsForAccount, exists bool) {
	return ads.NewMap[iotago.AccountID, RewardsForAccount](m.rewardsStorage(epochIndex)).Get(accountID)
}

func (m *Manager) poolStats(epochIndex iotago.EpochIndex) (poolStats *PoolsStats, err error) {
	poolStats = new(PoolsStats)
	poolStatsBytes, err := m.poolStatsStore.Get(epochIndex.Bytes())
	if err != nil {
		return poolStats, errors.Wrapf(err, "failed to get pool stats for epoch %d", epochIndex)
	}

	if _, err := poolStats.FromBytes(poolStatsBytes); err != nil {
		return poolStats, errors.Wrapf(err, "failed to parse pool stats for epoch %d", epochIndex)
	}

	return poolStats, nil
}

func aggregatePerformanceFactors(pfs []uint64) uint64 {
	var sum uint64
	for _, pf := range pfs {
		sum += pf
	}

	return sum / uint64(len(pfs))
}

func calculateProfitMargin(totalValidatorsStake, totalPoolStake uint64) uint64 {
	return (1 << 8) * totalValidatorsStake / (totalValidatorsStake + totalPoolStake)
}

func poolReward(totalValidatorsStake, totalStake, profitMargin, fixedCosts, performanceFactor uint64) uint64 {
	// TODO: decay is calculated per epoch now, so do we need to calculate the rewards for each slot of the epoch?
	return totalValidatorsStake * performanceFactor
}
