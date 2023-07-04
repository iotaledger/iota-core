package performance

import (
	"github.com/cockroachdb/errors"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TODO: add later as a protocol params, after its refactor is finished.
var (
	targetRewardFirstPeriod  uint64           = 233373068869021000 // TODO current values are per slot, update with new when provided by Olivia
	targetRewardChangeSlot   iotago.SlotIndex = 9460800
	targetRewardSecondPeriod uint64           = 85853149583786000
	validatorBlocksPerSlot   uint8            = 10
)

func (t *Tracker) RewardsRoot(epochIndex iotago.EpochIndex) iotago.Identifier {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return iotago.Identifier(ads.NewMap[iotago.AccountID, PoolRewards](t.rewardsStorage(epochIndex)).Root())
}

func (t *Tracker) ValidatorReward(validatorID iotago.AccountID, stakeAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (validatorReward iotago.Mana, err error) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	for epochIndex := epochStart; epochIndex <= epochEnd; epochIndex++ {
		rewardsForAccountInEpoch, exists := t.rewardsForAccount(validatorID, epochIndex)
		if !exists {
			continue
		}

		poolStats, err := t.poolStats(epochIndex)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to get pool stats for epoch %d", epochIndex)
		}

		unDecayedEpochRewards := uint64(rewardsForAccountInEpoch.FixedCost) +
			((poolStats.ProfitMargin * uint64(rewardsForAccountInEpoch.PoolRewards)) >> 8) +
			((((1<<8)-poolStats.ProfitMargin)*uint64(rewardsForAccountInEpoch.PoolRewards))>>8)*
				uint64(stakeAmount)/
				uint64(rewardsForAccountInEpoch.PoolStake)

		decayedEpochRewards, err := t.decayProvider.RewardsWithDecay(iotago.Mana(unDecayedEpochRewards), epochIndex, epochEnd)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to calculate rewards with decay for epoch %d", epochIndex)
		}
		validatorReward += decayedEpochRewards
	}

	return validatorReward, nil
}

func (t *Tracker) DelegatorReward(validatorID iotago.AccountID, delegatedAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (delegatorsReward iotago.Mana, err error) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	for epochIndex := epochStart; epochIndex <= epochEnd; epochIndex++ {
		rewardsForAccountInEpoch, exists := t.rewardsForAccount(validatorID, epochIndex)
		if !exists {
			continue
		}

		poolStats, err := t.poolStats(epochIndex)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to get pool stats for epoch %d", epochIndex)
		}

		unDecayedEpochRewards := ((((1 << 8) - poolStats.ProfitMargin) * uint64(rewardsForAccountInEpoch.PoolRewards)) >> 8) *
			uint64(delegatedAmount) /
			uint64(rewardsForAccountInEpoch.PoolStake)

		decayedEpochRewards, err := t.decayProvider.RewardsWithDecay(iotago.Mana(unDecayedEpochRewards), epochIndex, epochEnd)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to calculate rewards with decay for epoch %d", epochIndex)
		}

		delegatorsReward += decayedEpochRewards
	}

	return delegatorsReward, nil
}

func (t *Tracker) rewardsStorage(epochIndex iotago.EpochIndex) kvstore.KVStore {
	return lo.PanicOnErr(t.rewardBaseStore.WithExtendedRealm(epochIndex.Bytes()))
}

func (t *Tracker) rewardsForAccount(accountID iotago.AccountID, epochIndex iotago.EpochIndex) (rewardsForAccount *PoolRewards, exists bool) {
	return ads.NewMap[iotago.AccountID, PoolRewards](t.rewardsStorage(epochIndex)).Get(accountID)
}

func (t *Tracker) poolReward(slotIndex iotago.SlotIndex, totalValidatorsStake, totalStake, poolStake, validatorStake iotago.BaseToken, fixedCost iotago.Mana, performanceFactor uint64) iotago.Mana {
	initialReward := targetRewardFirstPeriod
	if slotIndex > targetRewardChangeSlot {
		initialReward = targetRewardSecondPeriod
	}

	// TODO: this calculation will overflow with ~4Gi poolstake already.
	// maybe we can reuse the functions from the mana decay provider?
	// should we move the mana decay functions to the safemath package?
	aux := (((1 << 31) * poolStake) / totalStake) + ((1 << 31) * validatorStake / totalValidatorsStake)
	aux2 := iotago.Mana(uint64(aux) * initialReward * performanceFactor)
	if (aux2 >> 40) < fixedCost {
		return 0
	}

	return (aux2 >> 40) - fixedCost
}

func calculateProfitMargin(totalValidatorsStake, totalPoolStake iotago.BaseToken) uint64 {
	return (1 << 8) * uint64(totalValidatorsStake) / (uint64(totalValidatorsStake) + uint64(totalPoolStake))
}
