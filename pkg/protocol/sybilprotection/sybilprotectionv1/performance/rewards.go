package performance

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/core/safemath"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *Tracker) RewardsRoot(epochIndex iotago.EpochIndex) (iotago.Identifier, error) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	m, err := t.rewardsMap(epochIndex)
	if err != nil {
		return iotago.Identifier{}, err
	}

	return iotago.Identifier(m.Root()), nil
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

		poolStats, err := t.poolStatsStore.Load(epochIndex)
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to get pool stats for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		if poolStats == nil {
			return 0, 0, 0, ierrors.Errorf("pool stats for epoch %d and validator accountID %s are nil", epochIndex, validatorID)
		}

		// if validator's fixed cost is greater than earned reward, all reward goes for delegators
		if rewardsForAccountInEpoch.PoolRewards < rewardsForAccountInEpoch.FixedCost {
			continue
		}
		poolRewardsNoFixedCost := rewardsForAccountInEpoch.PoolRewards - rewardsForAccountInEpoch.FixedCost

		profitMarginExponent := t.apiProvider.APIForEpoch(epochIndex).ProtocolParameters().RewardsParameters().ProfitMarginExponent
		profitMarginComplement, err := scaleUpComplement(poolStats.ProfitMargin, profitMarginExponent)
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to calculate profit margin factor due to overflow for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		result, err := safemath.SafeMul(poolStats.ProfitMargin, uint64(poolRewardsNoFixedCost))
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to calculate profit margin factor due to overflow for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		profitMarginFactor := result >> profitMarginExponent

		result, err = safemath.SafeMul(profitMarginComplement, uint64(poolRewardsNoFixedCost))
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to calculate profit margin factor due to overflow for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		result, err = safemath.SafeMul(result>>profitMarginExponent, uint64(stakeAmount))
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to calculate profit margin factor due to overflow for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		residualValidatorFactor, err := safemath.SafeDiv(result, uint64(rewardsForAccountInEpoch.PoolStake))
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to calculate residual validator factor due to overflow for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		result, err = safemath.SafeAdd(uint64(rewardsForAccountInEpoch.FixedCost), profitMarginFactor)
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to calculate un-decayed epoch reward due to overflow for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		unDecayedEpochRewards, err := safemath.SafeAdd(result, residualValidatorFactor)
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to calculate un-decayed epoch rewards due to overflow for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		decayProvider := t.apiProvider.APIForEpoch(epochIndex).ManaDecayProvider()
		decayedEpochRewards, err := decayProvider.RewardsWithDecay(iotago.Mana(unDecayedEpochRewards), epochIndex, epochEnd)
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to calculate rewards with decay for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		validatorReward, err = safemath.SafeAdd(validatorReward, decayedEpochRewards)
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to calculate validator reward due to overflow for epoch %d and validator accountID %s", epochIndex, validatorID)
		}
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

		poolStats, err := t.poolStatsStore.Load(epochIndex)
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to get pool stats for epoch %d and validator account ID %s", epochIndex, validatorID)
		}
		if poolStats == nil {
			return 0, 0, 0, ierrors.Errorf("pool stats for epoch %d and validator accountID %s are nil", epochIndex, validatorID)
		}

		profitMarginExponent := t.apiProvider.APIForEpoch(epochIndex).ProtocolParameters().RewardsParameters().ProfitMarginExponent
		profitMarginComplement, err := scaleUpComplement(poolStats.ProfitMargin, profitMarginExponent)
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to calculate profit margin factor due to overflow for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		// if pool reward was lower than fixed cost, the whole reward goes to delegators
		poolReward := rewardsForAccountInEpoch.PoolRewards
		// fixed cost was not too high, no punishment for the validator
		if rewardsForAccountInEpoch.PoolRewards >= rewardsForAccountInEpoch.FixedCost {
			poolReward = rewardsForAccountInEpoch.PoolRewards - rewardsForAccountInEpoch.FixedCost
		}

		result, err := safemath.SafeMul(profitMarginComplement, uint64(poolReward))
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to calculate unDecayedEpochRewards due to overflow for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		result, err = safemath.SafeMul(result>>profitMarginExponent, uint64(delegatedAmount))
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to calculate unDecayedEpochRewards due to overflow for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		unDecayedEpochRewards, err := safemath.SafeDiv(result, uint64(rewardsForAccountInEpoch.PoolStake))
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to calculate unDecayedEpochRewards due to overflow for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		decayProvider := t.apiProvider.APIForEpoch(epochIndex).ManaDecayProvider()
		decayedEpochRewards, err := decayProvider.RewardsWithDecay(iotago.Mana(unDecayedEpochRewards), epochIndex, epochEnd)
		if err != nil {
			return 0, 0, 0, ierrors.Wrapf(err, "failed to calculate rewards with decay for epoch %d and validator accountID %s", epochIndex, validatorID)
		}

		delegatorsReward += decayedEpochRewards
	}

	return delegatorsReward, epochStart, epochEnd, nil
}

func (t *Tracker) rewardsMap(epochIndex iotago.EpochIndex) (ads.Map[iotago.AccountID, *model.PoolRewards], error) {
	kv, err := t.rewardsStorePerEpochFunc(epochIndex)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get rewards store for epoch %d", epochIndex)
	}

	return ads.NewMap(kv,
		iotago.Identifier.Bytes,
		iotago.IdentifierFromBytes,
		(*model.PoolRewards).Bytes,
		model.PoolRewardsFromBytes,
	), nil
}

func (t *Tracker) rewardsForAccount(accountID iotago.AccountID, epochIndex iotago.EpochIndex) (rewardsForAccount *model.PoolRewards, exists bool, err error) {
	m, err := t.rewardsMap(epochIndex)
	if err != nil {
		return nil, false, err
	}

	return m.Get(accountID)
}
func (t *Tracker) poolReward(slotIndex iotago.SlotIndex, totalValidatorsStake, totalStake, poolStake, validatorStake iotago.BaseToken, performanceFactor uint64) (iotago.Mana, error) {
	apiForSlot := t.apiProvider.APIForSlot(slotIndex)
	epoch := apiForSlot.TimeProvider().EpochFromSlot(slotIndex)
	params := apiForSlot.ProtocolParameters()

	targetReward, err := params.RewardsParameters().TargetReward(epoch, apiForSlot)
	if err != nil {
		return 0, ierrors.Wrapf(err, "failed to calculate target reward for slot %d", slotIndex)
	}

	// Notice that, since both pool stake  and validator stake use at most 53 bits of the variable,
	// to not overflow the calculation, PoolCoefficientExponent must be at most 11. Pool Coefficient will then use at most PoolCoefficientExponent + 1 bits.
	poolCoefficient, err := t.calculatePoolCoefficient(poolStake, totalStake, validatorStake, totalValidatorsStake, slotIndex)
	if err != nil {
		return 0, ierrors.Wrapf(err, "failed to calculate pool coefficient for slot %d", slotIndex)
	}

	// Since `Pool Coefficient` uses at most 12 bits, `Target Reward(n)` uses at most 41 bits, and `Performance Factor` uses at most 8 bits,
	// this multiplication will not overflow using uint64 variables.
	result, err := safemath.SafeMul(poolCoefficient, uint64(targetReward))
	if err != nil {
		return 0, ierrors.Wrapf(err, "failed to calculate pool scaled reward due to overflow for slot %d", slotIndex)
	}

	scaledPoolReward, err := safemath.SafeMul(result, performanceFactor)
	if err != nil {
		return 0, ierrors.Wrapf(err, "failed to calculate pool reward without fixed costs due to overflow for slot %d", slotIndex)
	}

	result, err = safemath.SafeDiv(scaledPoolReward, uint64(params.RewardsParameters().ValidatorBlocksPerSlot))
	if err != nil {
		return 0, ierrors.Wrapf(err, "failed to calculate result reward due division by zero for slot %d", slotIndex)
	}
	poolRewardFixedCost := iotago.Mana(result >> (params.RewardsParameters().PoolCoefficientExponent + 1))

	return poolRewardFixedCost, nil
}

func (t *Tracker) calculatePoolCoefficient(poolStake, totalStake, validatorStake, totalValidatorStake iotago.BaseToken, slot iotago.SlotIndex) (uint64, error) {
	poolCoeffExponent := t.apiProvider.APIForSlot(slot).ProtocolParameters().RewardsParameters().PoolCoefficientExponent
	scaledUpPoolStake, err := safemath.SafeLeftShift(poolStake, poolCoeffExponent)
	if err != nil {
		return 0, ierrors.Wrapf(err, "failed to calculate pool coefficient due to overflow for slot %d", slot)
	}

	result1, err := safemath.SafeDiv(scaledUpPoolStake, totalStake)
	if err != nil {
		return 0, ierrors.Wrapf(err, "failed to calculate pool coefficient due to overflow for slot %d", slot)
	}

	scaledUpValidatorStake, err := safemath.SafeLeftShift(validatorStake, poolCoeffExponent)
	if err != nil {
		return 0, ierrors.Wrapf(err, "failed to calculate pool coefficient due to overflow for slot %d", slot)
	}

	result2, err := safemath.SafeDiv(scaledUpValidatorStake, totalValidatorStake)
	if err != nil {
		return 0, ierrors.Wrapf(err, "failed to calculate pool coefficient due to overflow for slot %d", slot)
	}

	poolCoeff, err := safemath.SafeAdd(result1, result2)
	if err != nil {
		return 0, ierrors.Wrapf(err, "failed to calculate pool coefficient due to overflow for slot %d", slot)
	}

	return uint64(poolCoeff), nil
}

// calculateProfitMargin calculates a common profit margin for all validators by firstly increasing the accuracy of the given value, so the profit margin is moved to the power of 2^accuracyShift.
func (t *Tracker) calculateProfitMargin(totalValidatorsStake, totalPoolStake iotago.BaseToken, epoch iotago.EpochIndex) (uint64, error) {
	scaledUpTotalValidatorStake, err := safemath.SafeLeftShift(totalValidatorsStake, t.apiProvider.APIForEpoch(epoch).ProtocolParameters().RewardsParameters().ProfitMarginExponent)
	if err != nil {
		return 0, ierrors.Wrapf(err, "failed to calculate profit margin due to overflow for epoch %d", epoch)
	}

	return uint64(scaledUpTotalValidatorStake / (totalValidatorsStake + totalPoolStake)), nil
}

// scaleUpComplement returns the 'shifted' completition to "one" for the shifted value where one is the 2^accuracyShift.
func scaleUpComplement[V iotago.BaseToken | iotago.Mana | uint64](val V, shift uint8) (V, error) {
	shiftedOne, err := safemath.SafeLeftShift(V(1), shift)
	if err != nil {
		return 0, err
	}

	return safemath.SafeSub(shiftedOne, val)
}
