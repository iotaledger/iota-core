package performance

import (
	"github.com/cockroachdb/errors"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (m *Tracker) RewardsRoot(epochIndex iotago.EpochIndex) iotago.Identifier {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return iotago.Identifier(ads.NewMap[iotago.AccountID, RewardsForAccount](m.rewardsStorage(epochIndex)).Root())
}

func (m *Tracker) ValidatorReward(validatorID iotago.AccountID, stakeAmount uint64, epochStart, epochEnd iotago.EpochIndex) (validatorReward uint64, err error) {
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

		unDecayedEpochRewards := rewardsForAccountInEpoch.FixedCost +
			((poolStats.ProfitMargin * rewardsForAccountInEpoch.PoolRewards) >> 8) +
			((((1<<8)-poolStats.ProfitMargin)*rewardsForAccountInEpoch.PoolRewards)>>8)*
				stakeAmount/
				rewardsForAccountInEpoch.PoolStake

		decayedEpochRewards, err := m.decayProvider.RewardsWithDecay(iotago.Mana(unDecayedEpochRewards), epochIndex, epochEnd)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to calculate rewards with decay for epoch %d", epochIndex)
		}
		validatorReward += uint64(decayedEpochRewards)
	}

	return validatorReward, nil
}

func (m *Tracker) DelegatorReward(validatorID iotago.AccountID, delegatedAmount uint64, epochStart, epochEnd iotago.EpochIndex) (delegatorsReward uint64, err error) {
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

		unDecayedEpochRewards := ((((1 << 8) - poolStats.ProfitMargin) * rewardsForAccountInEpoch.PoolRewards) >> 8) *
			delegatedAmount /
			rewardsForAccountInEpoch.PoolStake

		decayedEpochRewards, err := m.decayProvider.RewardsWithDecay(iotago.Mana(unDecayedEpochRewards), epochIndex, epochEnd)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to calculate rewards with decay for epoch %d", epochIndex)
		}

		delegatorsReward += uint64(decayedEpochRewards)
	}

	return delegatorsReward, nil
}

func (m *Tracker) rewardsStorage(epochIndex iotago.EpochIndex) kvstore.KVStore {
	return lo.PanicOnErr(m.rewardBaseStore.WithExtendedRealm(epochIndex.Bytes()))
}

func (m *Tracker) rewardsForAccount(accountID iotago.AccountID, epochIndex iotago.EpochIndex) (rewardsForAccount *RewardsForAccount, exists bool) {
	return ads.NewMap[iotago.AccountID, RewardsForAccount](m.rewardsStorage(epochIndex)).Get(accountID)
}

func calculateProfitMargin(totalValidatorsStake, totalPoolStake uint64) uint64 {
	return (1 << 8) * totalValidatorsStake / (totalValidatorsStake + totalPoolStake)
}

func poolReward(totalValidatorsStake, totalStake, profitMargin, fixedCosts, performanceFactor uint64) uint64 {
	// TODO: decay is calculated per epoch now, so do we need to calculate the rewards for each slot of the epoch?
	return totalValidatorsStake * performanceFactor
}
