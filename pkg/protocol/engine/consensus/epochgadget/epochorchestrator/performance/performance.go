package performance

import (
	"sync"

	"github.com/cockroachdb/errors"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Tracker struct {
	rewardBaseStore kvstore.KVStore
	poolStatsStore  kvstore.KVStore
	committeeStore  kvstore.KVStore

	performanceFactorsFunc func(slot iotago.SlotIndex) *prunable.PerformanceFactors

	timeProvider *iotago.TimeProvider

	decayProvider *iotago.ManaDecayProvider

	performanceFactorsMutex sync.RWMutex
	mutex                   sync.RWMutex
}

func NewTracker(
	rewardsBaseStore kvstore.KVStore,
	poolStatsStore kvstore.KVStore,
	committeeStore kvstore.KVStore,
	performanceFactorsFunc func(slot iotago.SlotIndex) *prunable.PerformanceFactors,
	timeProvider *iotago.TimeProvider,
	decayProvider *iotago.ManaDecayProvider,
) *Tracker {
	return &Tracker{
		rewardBaseStore:        rewardsBaseStore,
		poolStatsStore:         poolStatsStore,
		committeeStore:         committeeStore,
		performanceFactorsFunc: performanceFactorsFunc,
		timeProvider:           timeProvider,
		decayProvider:          decayProvider,
	}
}

func (m *Tracker) RegisterCommittee(epoch iotago.EpochIndex, committee *account.Accounts) error {
	return m.storeCommitteeForEpoch(epoch, committee)
}

func (m *Tracker) BlockAccepted(block *blocks.Block) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.performanceFactorsMutex.Lock()
	defer m.performanceFactorsMutex.Unlock()

	// TODO: check if this block is a validator block

	performanceFactors := m.performanceFactorsFunc(block.ID().Index())
	pf, err := performanceFactors.Load(block.Block().IssuerID)
	if err != nil {
		panic(err)
	}

	err = performanceFactors.Store(block.Block().IssuerID, pf+1)
	if err != nil {
		panic(err)
	}
}

<<<<<<< HEAD:pkg/protocol/engine/rewards/manager.go
func (m *Manager) RewardsRoot(epochIndex iotago.EpochIndex) iotago.Identifier {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return iotago.Identifier(ads.NewMap[iotago.AccountID, AccountRewards](m.rewardsStorage(epochIndex)).Root())
}

func (m *Manager) RegisterCommittee(epochIndex iotago.EpochIndex, committee *account.Accounts) error {
	return m.storeCommitteeForEpoch(epochIndex, committee)
}

func (m *Manager) ApplyEpoch(epochIndex iotago.EpochIndex) {
=======
func (m *Tracker) ApplyEpoch(epochIndex iotago.EpochIndex) {
>>>>>>> 0e8154af (Epoch orchestrator, rewards and performance tracker as epochgadget):pkg/protocol/engine/consensus/epochgadget/epochorchestrator/performance/performance.go
	m.mutex.Lock()
	defer m.mutex.Unlock()

	accounts := m.loadCommitteeForEpoch(epochIndex)
	rewardsTree := ads.NewMap[iotago.AccountID, AccountRewards](m.rewardsStorage(epochIndex))

	epochSlotStart := m.timeProvider.EpochStart(epochIndex)
	epochSlotEnd := m.timeProvider.EpochEnd(epochIndex)

	profitMargin := calculateProfitMargin(accounts.TotalValidatorStake(), accounts.TotalStake())
	poolsStats := PoolsStats{
		TotalStake:          accounts.TotalStake(),
		TotalValidatorStake: accounts.TotalValidatorStake(),
		ProfitMargin:        profitMargin,
	}

	if err := m.poolStatsStore.Set(epochIndex.Bytes(), lo.PanicOnErr(poolsStats.Bytes())); err != nil {
		panic(errors.Wrapf(err, "failed to store pool stats for epoch %d", epochIndex))
	}

	accounts.ForEach(func(id iotago.AccountID, pool *account.Pool) bool {
		intermediateFactors := make([]uint64, 0)
		for slot := epochSlotStart; slot <= epochSlotEnd; slot++ {
			performanceFactorStorage := m.performanceFactorsFunc(slot)
			if performanceFactorStorage == nil {
				intermediateFactors = append(intermediateFactors, 0)
			}

			pf, err := performanceFactorStorage.Load(id)
			if err != nil {
				panic(errors.Wrapf(err, "failed to load performance factor for account %s", id))
				return false
			}

			intermediateFactors = append(intermediateFactors, pf)

		}

		rewardsTree.Set(id, &RewardsForAccount{
			PoolStake:   pool.PoolStake,
			PoolRewards: poolReward(accounts.TotalValidatorStake(), accounts.TotalStake(), profitMargin, pool.FixedCost, aggregatePerformanceFactors(intermediateFactors)),
			FixedCost:   pool.FixedCost,
		})

		return true
	})
}

<<<<<<< HEAD:pkg/protocol/engine/rewards/manager.go
func (m *Manager) ValidatorReward(validatorID iotago.AccountID, stakeAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (validatorReward iotago.Mana, err error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for epochIndex := epochStart; epochIndex <= epochEnd; epochIndex++ {
		rewardsForAccountInEpoch, exists := m.rewardsForAccount(validatorID, epochIndex)
		if !exists {
			continue
		}

		poolStats, err := m.poolStats(epochIndex)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to get pool stats for epoch %d", epochIndex)
		}

		// TODO: check for overflows
		unDecayedEpochRewards := rewardsForAccountInEpoch.FixedCost +
			((iotago.Mana(poolStats.ProfitMargin) * rewardsForAccountInEpoch.PoolRewards) >> 8) +
			(((iotago.Mana(1<<8)-iotago.Mana(poolStats.ProfitMargin))*rewardsForAccountInEpoch.PoolRewards)>>8)*
				iotago.Mana(stakeAmount)/
				iotago.Mana(rewardsForAccountInEpoch.PoolStake)

		decayedEpochRewards, err := m.decayProvider.RewardsWithDecay(unDecayedEpochRewards, epochIndex, epochEnd)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to calculate rewards with decay for epoch %d", epochIndex)
		}
		validatorReward += decayedEpochRewards
	}

	return validatorReward, nil
}

func (m *Manager) DelegatorReward(validatorID iotago.AccountID, delegatedAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (delegatorsReward iotago.Mana, err error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for epochIndex := epochStart; epochIndex <= epochEnd; epochIndex++ {
		rewardsForAccountInEpoch, exists := m.rewardsForAccount(validatorID, epochIndex)
		if !exists {
			continue
		}

		poolStats, err := m.poolStats(epochIndex)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to get pool stats for epoch %d", epochIndex)
		}

		unDecayedEpochRewards := ((iotago.Mana((1<<8)-poolStats.ProfitMargin) * rewardsForAccountInEpoch.PoolRewards) >> 8) *
			iotago.Mana(delegatedAmount) /
			iotago.Mana(rewardsForAccountInEpoch.PoolStake)

		decayedEpochRewards, err := m.decayProvider.RewardsWithDecay(unDecayedEpochRewards, epochIndex, epochEnd)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to calculate rewards with decay for epoch %d", epochIndex)
		}

		delegatorsReward += decayedEpochRewards
	}

	return delegatorsReward, nil
}

func (m *Manager) rewardsForAccount(accountID iotago.AccountID, epochIndex iotago.EpochIndex) (rewardsForAccount *AccountRewards, exists bool) {
	return ads.NewMap[iotago.AccountID, AccountRewards](m.rewardsStorage(epochIndex)).Get(accountID)
}

func (m *Manager) poolStats(epochIndex iotago.EpochIndex) (poolStats *PoolsStats, err error) {
=======
func (m *Tracker) poolStats(epochIndex iotago.EpochIndex) (poolStats *PoolsStats, err error) {
>>>>>>> 0e8154af (Epoch orchestrator, rewards and performance tracker as epochgadget):pkg/protocol/engine/consensus/epochgadget/epochorchestrator/performance/performance.go
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

<<<<<<< HEAD:pkg/protocol/engine/rewards/manager.go
func aggregatePerformanceFactors(pfs []uint64) uint64 {
	var sum uint64
	for _, pf := range pfs {
		sum += pf
	}

	return sum / uint64(len(pfs))
}

func calculateProfitMargin(totalValidatorsStake iotago.BaseToken, totalPoolStake iotago.BaseToken) uint64 {
	// TODO: take care of overflows here.
	return (1 << 8) * uint64(totalValidatorsStake) / uint64(totalValidatorsStake+totalPoolStake)
}

func poolReward(slotIndex iotago.SlotIndex, totalValidatorsStake, totalStake, poolStake, validatorStake iotago.BaseToken, fixedCost iotago.Mana, performanceFactor uint64) iotago.Mana {
	initialReward := 233373068869021000
	if slotIndex > 9460800 {
		initialReward = 85853149583786000
	}
	epochsInSlot := 1 << 13
	targetRewardPerEpoch := uint64(initialReward * epochsInSlot)
	aux := (((1 << 31) * poolStake) / totalStake) + ((2 << 31) * validatorStake / totalValidatorsStake)
	aux2 := iotago.Mana(uint64(aux) * targetRewardPerEpoch * performanceFactor)
	if aux2 < fixedCost {
		return 0
	}
	reward := (aux2 >> 40) - fixedCost

	return reward
}

func (m *Manager) loadCommitteeForEpoch(epochIndex iotago.EpochIndex) *account.Accounts {
=======
func (m *Tracker) loadCommitteeForEpoch(epochIndex iotago.EpochIndex) *account.Accounts {
>>>>>>> 0e8154af (Epoch orchestrator, rewards and performance tracker as epochgadget):pkg/protocol/engine/consensus/epochgadget/epochorchestrator/performance/performance.go
	accountsBytes, err := m.committeeStore.Get(epochIndex.Bytes())
	if err != nil {
		panic(errors.Wrapf(err, "failed to load committee for epoch %d", epochIndex))
	}

	accounts, err := account.AccountsFromBytes(accountsBytes)
	if err != nil {
		panic(errors.Wrapf(err, "failed to parse committee for epoch %d", epochIndex))
	}
	return accounts
}

func (m *Tracker) storeCommitteeForEpoch(epochIndex iotago.EpochIndex, committee *account.Accounts) error {
	committeeBytes, err := committee.Bytes()
	if err != nil {
		return err
	}

	if err := m.committeeStore.Set(epochIndex.Bytes(), committeeBytes); err != nil {
		return err
	}

	return nil
}

func (m *Tracker) EligibleValidatorCandidates(epoch iotago.EpochIndex) *advancedset.AdvancedSet[iotago.AccountID] {
	// TODO: we should choose candidates we tracked performance for

	return &advancedset.AdvancedSet[iotago.AccountID]{}
}

func aggregatePerformanceFactors(pfs []uint64) uint64 {
	var sum uint64
	for _, pf := range pfs {
		sum += pf
	}

	return sum / uint64(len(pfs))
}
