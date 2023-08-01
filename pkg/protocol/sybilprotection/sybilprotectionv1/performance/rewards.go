package performance

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TODO: add later as a protocol params, after its refactor is finished.
var (
	targetRewardFirstPeriod   uint64           = 233373068869021000 // TODO current values are per slot, update with new when provided by Olivia
	targetRewardChangeSlot    iotago.SlotIndex = 9460800
	targetRewardSecondPeriod  uint64           = 85853149583786000
	validatorBlocksPerSlot    uint8            = 10
	profitMarginExponent      uint64           = 8
	rewardCalculationExponent uint64           = 31
	// TODO why do we choose 40 here, why dont we use ^31 again?
	finalRewardScalingExponent uint64 = 40
)

func (t *Tracker) RewardsRoot(epochIndex iotago.EpochIndex) iotago.Identifier {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return iotago.Identifier(t.rewardsMap(epochIndex).Root())
}

func (t *Tracker) ValidatorReward(validatorID iotago.AccountID, stakeAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (iotago.Mana, error) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	var validatorReward iotago.Mana

	// TODO: the epoch should be returned by the reward calculations and we should only loop until the current epoch, not epochEnd
	for epochIndex := epochStart; epochIndex <= epochEnd; epochIndex++ {
		rewardsForAccountInEpoch, exists, err := t.rewardsForAccount(validatorID, epochIndex)
		if err != nil {
			return 0, ierrors.Wrapf(err, "failed to get rewards for account %s in epoch %d", validatorID, epochIndex)
		}

		if !exists {
			continue
		}

		if rewardsForAccountInEpoch.PoolStake == 0 {
			continue
		}

		poolStats, err := t.poolStatsStore.Get(epochIndex)
		if err != nil {
			return 0, ierrors.Wrapf(err, "failed to get pool stats for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		unDecayedEpochRewards := uint64(rewardsForAccountInEpoch.FixedCost) +
			decreaseAccuracy(poolStats.ProfitMargin*uint64(rewardsForAccountInEpoch.PoolRewards), profitMarginExponent) +
			decreaseAccuracy(increasedAccuracyComplement(poolStats.ProfitMargin, profitMarginExponent)*uint64(rewardsForAccountInEpoch.PoolRewards), profitMarginExponent)*
				uint64(stakeAmount)/
				uint64(rewardsForAccountInEpoch.PoolStake)

		decayProvider := t.apiProvider.APIForEpoch(epochIndex).ManaDecayProvider()
		decayedEpochRewards, err2 := decayProvider.RewardsWithDecay(iotago.Mana(unDecayedEpochRewards), epochIndex, epochEnd)
		if err2 != nil {
			return 0, ierrors.Wrapf(err2, "failed to calculate rewards with decay for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		validatorReward += decayedEpochRewards
	}

	return validatorReward, nil
}

func (t *Tracker) DelegatorReward(validatorID iotago.AccountID, delegatedAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (iotago.Mana, error) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	var delegatorsReward iotago.Mana

	// TODO: the epoch should be returned by the reward calculations and we should only loop until the current epoch, not epochEnd
	for epochIndex := epochStart; epochIndex <= epochEnd; epochIndex++ {
		rewardsForAccountInEpoch, exists, err := t.rewardsForAccount(validatorID, epochIndex)
		if err != nil {
			return 0, ierrors.Wrapf(err, "failed to get rewards for account %s in epoch %d", validatorID, epochIndex)
		}

		if !exists {
			continue
		}

		if rewardsForAccountInEpoch.PoolStake == 0 {
			continue
		}

		poolStats, err := t.poolStatsStore.Get(epochIndex)
		if err != nil {
			return 0, ierrors.Wrapf(err, "failed to get pool stats for epoch %d and validator account ID %s", epochIndex, validatorID)
		}

		unDecayedEpochRewards := decreaseAccuracy(increasedAccuracyComplement(poolStats.ProfitMargin, profitMarginExponent)*uint64(rewardsForAccountInEpoch.PoolRewards), profitMarginExponent) *
			uint64(delegatedAmount) /
			uint64(rewardsForAccountInEpoch.PoolStake)

		decayProvider := t.apiProvider.APIForEpoch(epochIndex).ManaDecayProvider()
		decayedEpochRewards, err := decayProvider.RewardsWithDecay(iotago.Mana(unDecayedEpochRewards), epochIndex, epochEnd)
		if err != nil {
			return 0, ierrors.Wrapf(err, "failed to calculate rewards with decay for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		delegatorsReward += decayedEpochRewards
	}

	return delegatorsReward, nil
}

func (t *Tracker) rewardsStorage(epochIndex iotago.EpochIndex) kvstore.KVStore {
	return lo.PanicOnErr(t.rewardBaseStore.WithExtendedRealm(epochIndex.MustBytes()))
}

func (t *Tracker) rewardsMap(epochIndex iotago.EpochIndex) ads.Map[iotago.AccountID, *PoolRewards] {
	return ads.NewMap(t.rewardsStorage(epochIndex),
		iotago.Identifier.Bytes,
		iotago.IdentifierFromBytes,
		(*PoolRewards).Bytes,
		PoolRewardsFromBytes,
	)
}

func (t *Tracker) rewardsForAccount(accountID iotago.AccountID, epochIndex iotago.EpochIndex) (rewardsForAccount *PoolRewards, exists bool, err error) {
	return t.rewardsMap(epochIndex).Get(accountID)
}

func (t *Tracker) poolReward(slotIndex iotago.SlotIndex, totalValidatorsStake, totalStake, poolStake, validatorStake iotago.BaseToken, fixedCost iotago.Mana, performanceFactor uint64) iotago.Mana {
	initialReward := targetRewardFirstPeriod
	if slotIndex > targetRewardChangeSlot {
		initialReward = targetRewardSecondPeriod
	}

	// TODO: this calculation will overflow with ~4Gi poolstake already.
	// maybe we can reuse the functions from the mana decay provider?
	// should we move the mana decay functions to the safemath package?
	aux := (increaseAccuracy(poolStake, rewardCalculationExponent) / totalStake) + (increaseAccuracy(validatorStake, rewardCalculationExponent) / totalValidatorsStake)
	aux2 := iotago.Mana(uint64(aux) * initialReward * performanceFactor)
	if decreaseAccuracy(aux2, finalRewardScalingExponent) < fixedCost {
		return 0
	}

	return (aux2 >> finalRewardScalingExponent) - fixedCost
}

// calculateProfitMargin calculates the profit margin of the pool by firstly increasing the accuracy of the given value, so the profit margin is moved to the power of 2^accuracyShift.
func calculateProfitMargin(totalValidatorsStake, totalPoolStake iotago.BaseToken) uint64 {
	return uint64(increaseAccuracy(totalValidatorsStake, profitMarginExponent) / (totalValidatorsStake + totalPoolStake))
}

// increaseAccuracy shifts the bits of the given value to the left by the given amount, so that the value is moved to the power of 2^accuracyShift.
// TODO make sure that we handle overflow here correctly if the inserted value is > 2^(64-accuracyShift).
func increaseAccuracy[V iotago.BaseToken | iotago.Mana | uint64](val V, shift uint64) V {
	return val << shift
}

// decreaseAccuracy reversts the accuracy operation of increaseAccuracy by shifting the bits of the given value to the right by the profitMarginExponent.
// This is a lossy operation. All values less than 2^accuracyShift will be rounded to 0.
func decreaseAccuracy[V iotago.BaseToken | iotago.Mana | uint64](val V, shift uint64) V {
	return val >> shift
}

// increasedAccuracyComplement returns the 'shifted' completition to "one" for the shifted value where one is the 2^accuracyShift.
func increasedAccuracyComplement[V iotago.BaseToken | iotago.Mana | uint64](val V, shift uint64) V {
	// it should never overflow for val=profit margin, if profit margin was previously scaled with increaseAccuracy.
	return (1 << shift) - val
}
