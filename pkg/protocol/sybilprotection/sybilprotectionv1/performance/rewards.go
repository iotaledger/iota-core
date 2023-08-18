package performance

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *Tracker) RewardsRoot(epochIndex iotago.EpochIndex) iotago.Identifier {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return iotago.Identifier(t.rewardsMap(epochIndex).Root())
}

func (t *Tracker) ValidatorReward(validatorID iotago.AccountID, stakeAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (iotago.Mana, iotago.EpochIndex, iotago.EpochIndex, error) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	var validatorReward iotago.Mana

	// limit looping to committed epochs
	if epochEnd > t.latestAppliedEpoch {
		epochEnd = t.latestAppliedEpoch
	}

	for epochIndex := epochStart; epochIndex <= epochEnd; epochIndex++ {
		rewardsForAccountInEpoch, exists, err := t.rewardsForAccount(validatorID, epochIndex)
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to get rewards for account %s in epoch %d", validatorID, epochIndex)
		}

		if !exists || rewardsForAccountInEpoch.PoolStake == 0 {
			// updating epoch start for beginning epochs without the reward
			if epochIndex < epochEnd && epochStart == epochIndex {
				epochStart = epochIndex + 1
			}

			continue
		}

		poolStats, err := t.poolStatsStore.Get(epochIndex)
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to get pool stats for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		profitMarginExponent := t.apiProvider.APIForEpoch(epochIndex).ProtocolParameters().RewardsParameters().ProfitMarginExponent
		profitMarginComplement := scaleUpComplement(poolStats.ProfitMargin, profitMarginExponent)
		profitMarginFactor := scaleDownWithExponent(poolStats.ProfitMargin*uint64(rewardsForAccountInEpoch.PoolRewards), profitMarginExponent)
		residualValidatorFactor := scaleDownWithExponent(iotago.Mana(profitMarginComplement)*rewardsForAccountInEpoch.PoolRewards, profitMarginExponent) * iotago.Mana(stakeAmount) / iotago.Mana(rewardsForAccountInEpoch.PoolStake)

		unDecayedEpochRewards := rewardsForAccountInEpoch.FixedCost + iotago.Mana(profitMarginFactor) + residualValidatorFactor

		decayProvider := t.apiProvider.APIForEpoch(epochIndex).ManaDecayProvider()
		decayedEpochRewards, err2 := decayProvider.RewardsWithDecay(unDecayedEpochRewards, epochIndex, epochEnd)
		if err2 != nil {
			return 0, 0, 0, ierrors.Wrapf(err2, "failed to calculate rewards with decay for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		validatorReward += decayedEpochRewards
	}

	return validatorReward, epochStart, epochEnd, nil
}

func (t *Tracker) DelegatorReward(validatorID iotago.AccountID, delegatedAmount iotago.BaseToken, epochStart, epochEnd iotago.EpochIndex) (iotago.Mana, iotago.EpochIndex, iotago.EpochIndex, error) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	var delegatorsReward iotago.Mana

	// limit looping to committed epochs
	if epochEnd > t.latestAppliedEpoch {
		epochEnd = t.latestAppliedEpoch
	}

	for epochIndex := epochStart; epochIndex <= epochEnd; epochIndex++ {
		rewardsForAccountInEpoch, exists, err := t.rewardsForAccount(validatorID, epochIndex)
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to get rewards for account %s in epoch %d", validatorID, epochIndex)
		}

		if !exists || rewardsForAccountInEpoch.PoolStake == 0 {
			// updating epoch start for beginning epochs without the reward
			if epochStart == epochIndex {
				epochStart = epochIndex + 1
			}

			continue
		}

		poolStats, err := t.poolStatsStore.Get(epochIndex)
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to get pool stats for epoch %d and validator account ID %s", epochIndex, validatorID)
		}

		profitMarginExponent := t.apiProvider.APIForEpoch(epochIndex).ProtocolParameters().RewardsParameters().ProfitMarginExponent
		profitMarginComplement := scaleUpComplement(poolStats.ProfitMargin, profitMarginExponent)

		unDecayedEpochRewards := scaleDownWithExponent(iotago.Mana(profitMarginComplement)*rewardsForAccountInEpoch.PoolRewards, profitMarginExponent) * iotago.Mana(delegatedAmount) / iotago.Mana(rewardsForAccountInEpoch.PoolStake)

		decayProvider := t.apiProvider.APIForEpoch(epochIndex).ManaDecayProvider()
		decayedEpochRewards, err := decayProvider.RewardsWithDecay(unDecayedEpochRewards, epochIndex, epochEnd)
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to calculate rewards with decay for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		delegatorsReward += decayedEpochRewards
	}

	return delegatorsReward, epochStart, epochEnd, nil
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
	epoch := t.apiProvider.APIForSlot(slotIndex).TimeProvider().EpochFromSlot(slotIndex)
	params := t.apiProvider.APIForSlot(slotIndex).ProtocolParameters()
	targetReward := params.RewardsParameters().TargetReward(epoch, uint64(params.TokenSupply()), params.ManaParameters().ManaGenerationRate, params.ManaParameters().ManaGenerationRateExponent, params.SlotsPerEpochExponent())

	// Notice that, since both pool stake  and validator stake use at most 53 bits of the variable,
	// to not overflow the calculation, PoolCoefficientExponent must be at most 11. Pool Coefficient will then use at most PoolCoefficientExponent + 1 bits.
	poolCoefficient := t.calculatePoolCoefficient(poolStake, totalStake, validatorStake, totalValidatorsStake, slotIndex)
	// Since `Pool Coefficient` uses at most 12 bits, `Target Reward(n)` uses at most 41 bits, and `Performance Factor` uses at most 8 bits,
	// this multiplication will not overflow using uint64 variables.
	scaledPoolReward := poolCoefficient * targetReward * performanceFactor
	poolRewardNoFixedCost := iotago.Mana(scaleDownWithExponent(scaledPoolReward/uint64(params.RewardsParameters().ValidatorBlocksPerSlot), params.RewardsParameters().PoolCoefficientExponent+1))
	// if validator's fixed cost is greater than earned reward, all reward goes for delegators
	if poolRewardNoFixedCost < fixedCost {
		return 0
	}
	return poolRewardNoFixedCost - fixedCost
}

func (t *Tracker) calculatePoolCoefficient(poolStake, totalStake, validatorStake, totalValidatorStake iotago.BaseToken, slot iotago.SlotIndex) uint64 {
	poolCoeffExponent := t.apiProvider.APIForSlot(slot).ProtocolParameters().RewardsParameters().PoolCoefficientExponent
	poolCoeff := scaleUpWithExponent(poolStake, poolCoeffExponent)/totalStake +
		scaleUpWithExponent(validatorStake, poolCoeffExponent)/totalValidatorStake

	return uint64(poolCoeff)
}

// calculateProfitMargin calculates the profit margin of the pool by firstly increasing the accuracy of the given value, so the profit margin is moved to the power of 2^accuracyShift.
func (t *Tracker) calculateProfitMargin(totalValidatorsStake, totalPoolStake iotago.BaseToken, epoch iotago.EpochIndex) uint64 {
	return uint64(scaleUpWithExponent(totalValidatorsStake, t.apiProvider.APIForEpoch(epoch).ProtocolParameters().RewardsParameters().ProfitMarginExponent) / (totalValidatorsStake + totalPoolStake))
}

// scaleUpWithExponent shifts the bits of the given value to the left by the given amount, so that the value is moved to the power of 2^accuracyShift.
// TODO make sure that we handle overflow here correctly if the inserted value is > 2^(64-accuracyShift).
func scaleUpWithExponent[V iotago.BaseToken | iotago.Mana | uint64](val V, shift uint8) V {
	return val << shift
}

// scaleDownWithExponent reversts the accuracy operation of scaleUpWithExponent by shifting the bits of the given value to the right by the profitMarginExponent.
// This is a lossy operation. All values less than 2^accuracyShift will be rounded to 0.
func scaleDownWithExponent[V iotago.BaseToken | iotago.Mana | uint64](val V, shift uint8) V {
	return val >> shift
}

// scaleUpComplement returns the 'shifted' completition to "one" for the shifted value where one is the 2^accuracyShift.
func scaleUpComplement[V iotago.BaseToken | iotago.Mana | uint64](val V, shift uint8) V {
	// it should never overflow for val=profit margin, if profit margin was previously scaled with scaleUpWithExponent.
	return (1 << shift) - val
}
