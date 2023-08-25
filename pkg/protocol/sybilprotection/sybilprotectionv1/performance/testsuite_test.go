package performance

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type TestSuite struct {
	T *testing.T

	accounts             map[string]iotago.AccountID
	latestCommittedEpoch iotago.EpochIndex

	apiProvider api.Provider

	Instance             *Tracker
	perforanceFactorFunc func(iotago.SlotIndex) *prunable.ValidatorSlotPerformance
}

func NewTestSuite(t *testing.T) *TestSuite {
	apiProvider := api.NewEpochBasedProvider()
	// setup params for 8 epochs
	for i := 0; i <= 8; i++ {
		apiProvider.AddProtocolParametersAtEpoch(iotago.NewV3ProtocolParameters(iotago.WithTimeProviderOptions(time.Now().Unix(), 10, 3)), iotago.EpochIndex(i))
	}

	ts := &TestSuite{
		T:           t,
		accounts:    make(map[string]iotago.AccountID),
		apiProvider: apiProvider,
	}
	ts.InitPerformanceTracker()

	return ts
}

func (t *TestSuite) InitPerformanceTracker() {
	prunableStores := make(map[iotago.SlotIndex]kvstore.KVStore)
	perforanceFactorFunc := func(index iotago.SlotIndex) *prunable.ValidatorSlotPerformance {
		if _, exists := prunableStores[index]; !exists {
			prunableStores[index] = mapdb.NewMapDB()
		}

		p := prunable.NewPerformanceFactors(index, prunableStores[index], t.apiProvider)

		return p
	}
	t.perforanceFactorFunc = perforanceFactorFunc

	RegistrationActivityFunc := func(index iotago.SlotIndex) *prunable.RegisteredValidatorSlotActivity {
		if _, exists := prunableStores[index]; !exists {
			prunableStores[index] = mapdb.NewMapDB()
		}

		p := prunable.NewRegisteredValidatorActivity(index, prunableStores[index], t.apiProvider)

		return p
	}

	rewardsStore := mapdb.NewMapDB()
	poolStatsStore := mapdb.NewMapDB()
	committeeStore := mapdb.NewMapDB()
	t.Instance = NewTracker(rewardsStore, poolStatsStore, committeeStore, RegistrationActivityFunc, perforanceFactorFunc, t.latestCommittedEpoch, t.apiProvider, func(err error) {})
}

func (t *TestSuite) Account(alias string, createIfNotExists bool) iotago.AccountID {
	if accID, exists := t.accounts[alias]; exists {
		return accID
	} else if !createIfNotExists {
		panic(fmt.Sprintf("account with alias '%s' does not exist", alias))
	}

	t.accounts[alias] = tpkg.RandAccountID()
	t.accounts[alias].RegisterAlias(alias)

	return t.accounts[alias]
}

func (t *TestSuite) ApplyEpochActions(epochIndex iotago.EpochIndex, actions map[string]*EpochActions) {
	committee := account.NewAccounts()
	for alias, action := range actions {
		accountID := t.Account(alias, true)
		committee.Set(accountID, &account.Pool{
			PoolStake:      action.PoolStake,
			ValidatorStake: action.ValidatorStake,
			FixedCost:      action.FixedCost,
		})
	}

	err := t.Instance.RegisterCommittee(epochIndex, committee)
	require.NoError(t.T, err)
	for accIDAlias, action := range actions {
		accID := t.Account(accIDAlias, true)
		t.applyPerformanceFactor(accID, epochIndex, action.ActiveSlotsCount, action.ValidationBlocksSentPerSlot, action.SlotPerformance)
	}

	t.Instance.ApplyEpoch(epochIndex, committee)
	t.latestCommittedEpoch = epochIndex
}

func (t *TestSuite) AssertEpochRewards(epochIndex iotago.EpochIndex, actions map[string]*EpochActions) {
	totalStake := iotago.BaseToken(0)
	totalValidatorsStake := iotago.BaseToken(0)
	for _, action := range actions {
		totalStake += action.PoolStake
		totalValidatorsStake += action.ValidatorStake
	}
	profitMarging := t.calculateProfitMargin(totalValidatorsStake, totalStake, epochIndex)

	for alias, action := range actions {
		epochPerformanceFactor := action.SlotPerformance * action.ActiveSlotsCount
		poolRewards := t.calculatePoolReward(epochIndex, totalValidatorsStake, totalStake, action.PoolStake, action.ValidatorStake, uint64(action.FixedCost), epochPerformanceFactor)
		fmt.Println("pool: ", alias, "rewards: ", poolRewards)
		expectedValidatorReward := t.validatorReward(epochIndex, profitMarging, poolRewards, uint64(action.ValidatorStake), uint64(action.PoolStake), uint64(action.FixedCost))
		accountID := t.Account(alias, true)
		actualValidatorReward, _, _, err := t.Instance.ValidatorReward(accountID, actions[alias].ValidatorStake, epochIndex, epochIndex)
		fmt.Println("actual validator reward: ", actualValidatorReward, "expected validator reward: ", expectedValidatorReward)
		require.NoError(t.T, err)
		require.Equal(t.T, expectedValidatorReward, actualValidatorReward)

		for delegatedAmount := range action.Delegators {
			expectedDelegatorReward := t.delegatorReward(epochIndex, profitMarging, poolRewards, uint64(delegatedAmount), uint64(action.PoolStake), uint64(action.FixedCost))
			actualDelegatorReward, _, _, err := t.Instance.DelegatorReward(accountID, iotago.BaseToken(delegatedAmount), epochIndex, epochIndex)
			fmt.Println("actual delegator reward: ", actualDelegatorReward, "expected delegator reward: ", expectedDelegatorReward)
			require.NoError(t.T, err)
			require.Equal(t.T, expectedDelegatorReward, actualDelegatorReward)
		}

	}
}

func (t *TestSuite) validatorReward(epochIndex iotago.EpochIndex, profitMargin, poolRewards, stakeAmount, poolStake, fixedCost uint64) iotago.Mana {
	profitMarginExponent := t.apiProvider.APIForEpoch(epochIndex).ProtocolParameters().RewardsParameters().ProfitMarginExponent
	profitMarginComplement := (1 << profitMarginExponent) - profitMargin
	profitMarginFactor := (profitMargin * poolRewards) >> profitMarginExponent
	residualValidatorFactor := ((profitMarginComplement * poolRewards) >> profitMarginExponent) * stakeAmount / poolStake

	unDecayedEpochRewards := fixedCost + profitMarginFactor + residualValidatorFactor
	decayProvider := t.apiProvider.APIForEpoch(epochIndex).ManaDecayProvider()
	decayedEpochRewards, err := decayProvider.RewardsWithDecay(iotago.Mana(unDecayedEpochRewards), epochIndex, epochIndex)
	require.NoError(t.T, err)

	return decayedEpochRewards
}

func (t *TestSuite) delegatorReward(epochIndex iotago.EpochIndex, profitMargin, poolRewards, delegatedAmount, poolStake, fixedCost uint64) iotago.Mana {
	profitMarginExponent := t.apiProvider.APIForEpoch(epochIndex).ProtocolParameters().RewardsParameters().ProfitMarginExponent
	profitMarginComplement := (1 << profitMarginExponent) - profitMargin

	unDecayedEpochRewards := (((profitMarginComplement * poolRewards) >> profitMarginExponent) * delegatedAmount) / poolStake

	decayProvider := t.apiProvider.APIForEpoch(epochIndex).ManaDecayProvider()
	decayedEpochRewards, err := decayProvider.RewardsWithDecay(iotago.Mana(unDecayedEpochRewards), epochIndex, epochIndex)
	require.NoError(t.T, err)

	return decayedEpochRewards
}

func (t *TestSuite) calculatePoolReward(epoch iotago.EpochIndex, totalValidatorsStake, totalStake, poolStake, validatorStake iotago.BaseToken, fixedCost, performanceFactor uint64) uint64 {
	params := t.apiProvider.APIForEpoch(epoch).ProtocolParameters()
	targetReward := params.RewardsParameters().TargetReward(epoch, uint64(params.TokenSupply()), params.ManaParameters().ManaGenerationRate, params.ManaParameters().ManaGenerationRateExponent, params.SlotsPerEpochExponent())

	poolCoefficient := t.calculatePoolCoefficient(poolStake, totalStake, validatorStake, totalValidatorsStake, epoch)
	scaledPoolReward := poolCoefficient * targetReward * performanceFactor
	poolRewardNoFixedCost := scaledPoolReward / uint64(params.RewardsParameters().ValidatorBlocksPerSlot) >> (params.RewardsParameters().PoolCoefficientExponent + 1)
	if poolRewardNoFixedCost < fixedCost {
		fmt.Println("less than fixed cost ", poolRewardNoFixedCost, fixedCost)
		return 0
	}

	return poolRewardNoFixedCost - fixedCost
}

func (t *TestSuite) calculatePoolCoefficient(poolStake, totalStake, validatorStake, totalValidatorStake iotago.BaseToken, epoch iotago.EpochIndex) uint64 {
	poolCoeffExponent := t.apiProvider.APIForEpoch(epoch).ProtocolParameters().RewardsParameters().PoolCoefficientExponent
	poolCoeff := (poolStake<<poolCoeffExponent)/totalStake +
		(validatorStake<<poolCoeffExponent)/totalValidatorStake

	return uint64(poolCoeff)
}

// calculateProfitMargin calculates the profit margin of the pool by firstly increasing the accuracy of the given value, so the profit margin is moved to the power of 2^accuracyShift.
func (t *TestSuite) calculateProfitMargin(totalValidatorsStake, totalPoolStake iotago.BaseToken, epoch iotago.EpochIndex) uint64 {
	return uint64((totalValidatorsStake << t.apiProvider.APIForEpoch(epoch).ProtocolParameters().RewardsParameters().ProfitMarginExponent) / (totalValidatorsStake + totalPoolStake))
}

func (t *TestSuite) applyPerformanceFactor(accountID iotago.AccountID, epochIndex iotago.EpochIndex, activeSlotsCount, validationBlocksSentPerSlot, slotPerformanceFactor uint64) {
	startSlot := t.apiProvider.APIForEpoch(epochIndex).TimeProvider().EpochStart(epochIndex)
	endSlot := t.apiProvider.APIForEpoch(epochIndex).TimeProvider().EpochEnd(epochIndex)
	valBlocksNum := t.apiProvider.APIForEpoch(epochIndex).ProtocolParameters().RewardsParameters().ValidatorBlocksPerSlot
	subslotDur := time.Duration(t.apiProvider.APIForEpoch(epochIndex).TimeProvider().SlotDurationSeconds()) * time.Second / time.Duration(valBlocksNum)

	fmt.Println("Start slot: ", startSlot, "\nEnd slot: ", endSlot, "\nSubslot duration: ", subslotDur, "\nTarget validation blocks: ", validationBlocksSentPerSlot)

	slotCount := uint64(0)
	for slot := startSlot; slot <= endSlot; slot++ {
		if slotCount > activeSlotsCount {
			// no more blocks issued by this validator in this epoch
			return
		}
		fmt.Println("New slot: ", slot)
		for i := uint64(0); i < validationBlocksSentPerSlot; i++ {
			block := tpkg.RandBasicBlockWithIssuerAndRMC(accountID, 10)
			subslotIndex := i
			// issued more than one block in the same slot to reduce performance factor
			if i >= slotPerformanceFactor {
				subslotIndex = 0
			}
			block.IssuingTime = t.apiProvider.APIForEpoch(epochIndex).TimeProvider().SlotStartTime(slot).Add(time.Duration(subslotIndex)*subslotDur + 1*time.Nanosecond)
			fmt.Println("Issue block at: ", block.IssuingTime)
			modelBlock, err := model.BlockFromBlock(block, t.apiProvider.APIForEpoch(epochIndex))
			t.Instance.TrackValidationBlock(blocks.NewBlock(modelBlock))

			require.NoError(t.T, err)
		}
		slotCount++
	}
}

func (t *TestSuite) calculateExpectedRewards(epochsCount int, epochActions map[string]*EpochActions) (map[iotago.EpochIndex]map[string]iotago.Mana, map[iotago.EpochIndex]map[string]iotago.Mana) {
	delegatorRewardPerAccount := make(map[iotago.EpochIndex]map[string]iotago.Mana)
	validatorRewardPerAccount := make(map[iotago.EpochIndex]map[string]iotago.Mana)
	for epochIndex := iotago.EpochIndex(1); epochIndex <= iotago.EpochIndex(epochsCount); epochIndex++ {
		delegatorRewardPerAccount[epochIndex] = make(map[string]iotago.Mana)
		validatorRewardPerAccount[epochIndex] = make(map[string]iotago.Mana)
		for aliasAccount := range epochActions {
			reward, _, _, err := t.Instance.DelegatorReward(t.Account(aliasAccount, false), 1, epochIndex, epochIndex)
			require.NoError(t.T, err)
			delegatorRewardPerAccount[epochIndex][aliasAccount] = reward
		}
		for aliasAccount := range epochActions {
			reward, _, _, err := t.Instance.ValidatorReward(t.Account(aliasAccount, true), 1, epochIndex, epochIndex)
			require.NoError(t.T, err)
			validatorRewardPerAccount[epochIndex][aliasAccount] = reward
		}
	}
	return delegatorRewardPerAccount, validatorRewardPerAccount
}

type EpochActions struct {
	PoolStake      iotago.BaseToken
	ValidatorStake iotago.BaseToken
	Delegators     []iotago.BaseToken
	FixedCost      iotago.Mana
	// ActiveSlotsCount is the number of firsts slots the validator was active in the epoch.
	ActiveSlotsCount uint64
	// ValidationBlocksSentPerSlot is the number of validation blocks validator sent per slot.
	ValidationBlocksSentPerSlot uint64
	// SlotPerformance is the target slot performance factor, how many subslots were covered by validator blocks.
	SlotPerformance uint64
}
