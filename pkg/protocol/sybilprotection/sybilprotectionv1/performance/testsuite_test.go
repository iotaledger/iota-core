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
	"github.com/iotaledger/iota-core/pkg/storage/prunable/epochstore"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type TestSuite struct {
	T                    *testing.T
	stores               map[iotago.SlotIndex]kvstore.KVStore
	accounts             map[string]iotago.AccountID
	poolRewards          map[iotago.EpochIndex]map[string]*model.PoolRewards
	epochStats           map[iotago.EpochIndex]*model.PoolsStats
	latestCommittedEpoch iotago.EpochIndex

	api iotago.API

	Instance             *Tracker
	perforanceFactorFunc func(iotago.SlotIndex) *model.ValidatorPerformance
}

func NewTestSuite(t *testing.T) *TestSuite {
	ts := &TestSuite{
		T:           t,
		accounts:    make(map[string]iotago.AccountID),
		poolRewards: make(map[iotago.EpochIndex]map[string]*model.PoolRewards),
		epochStats:  make(map[iotago.EpochIndex]*model.PoolsStats),
		api: iotago.V3API(
			iotago.NewV3ProtocolParameters(
				iotago.WithTimeProviderOptions(time.Now().Unix(), 10, 3),
				iotago.WithRewardsOptions(10, 8, 8, 11, 1154, 2, 1),
			),
		),
	}
	ts.InitPerformanceTracker()

	return ts
}

func (t *TestSuite) InitPerformanceTracker() {
	prunableStores := make(map[iotago.SlotIndex]kvstore.KVStore)
	performanceFactorFunc := func(index iotago.SlotIndex) (*slotstore.Store[iotago.AccountID, *model.ValidatorPerformance], error) {
		if _, exists := prunableStores[index]; !exists {
			prunableStores[index] = mapdb.NewMapDB()
		}

		p := slotstore.NewStore(index, prunableStores[index],
			iotago.Identifier.Bytes,
			iotago.IdentifierFromBytes,
			func(s *model.ValidatorPerformance) ([]byte, error) {
				return s.Bytes(t.api)
			},
			model.ValidatorPerformanceFromBytes(t.api),
		)

		return p, nil
	}

	rewardsStore := epochstore.NewEpochKVStore(kvstore.Realm{}, kvstore.Realm{}, mapdb.NewMapDB(), 0)
	poolStatsStore := epochstore.NewStore(kvstore.Realm{}, kvstore.Realm{}, mapdb.NewMapDB(), 0, (*model.PoolsStats).Bytes, model.PoolsStatsFromBytes)
	committeeStore := epochstore.NewStore(kvstore.Realm{}, kvstore.Realm{}, mapdb.NewMapDB(), 0, (*account.Accounts).Bytes, account.AccountsFromBytes)

	t.Instance = NewTracker(rewardsStore.GetEpoch, poolStatsStore, committeeStore, performanceFactorFunc, t.latestCommittedEpoch, api.SingleVersionProvider(t.api), func(err error) {})
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
		action.validate(t.T, t.api)

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
		accID := t.Account(accIDAlias, false)
		t.applyPerformanceFactor(accID, epochIndex, action.ActiveSlotsCount, action.ValidationBlocksSentPerSlot, action.SlotPerformance)
	}

	err = t.Instance.ApplyEpoch(epochIndex, committee)
	require.NoError(t.T, err)

	t.latestCommittedEpoch = epochIndex

	totalStake := iotago.BaseToken(0)
	totalValidatorsStake := iotago.BaseToken(0)
	for _, action := range actions {
		totalStake += action.PoolStake
		totalValidatorsStake += action.ValidatorStake
	}
	profitMarging := t.calculateProfitMargin(totalValidatorsStake, totalStake)
	t.epochStats[epochIndex] = &model.PoolsStats{
		TotalStake:          totalStake,
		TotalValidatorStake: totalValidatorsStake,
		ProfitMargin:        profitMarging,
	}
	t.poolRewards[epochIndex] = make(map[string]*model.PoolRewards)

	for alias, action := range actions {
		epochPerformanceFactor := action.SlotPerformance * action.ActiveSlotsCount >> t.api.ProtocolParameters().TimeProvider().SlotsPerEpochExponent()
		poolRewards := t.calculatePoolReward(epochIndex, totalValidatorsStake, totalStake, action.PoolStake, action.ValidatorStake, epochPerformanceFactor)
		t.poolRewards[epochIndex][alias] = &model.PoolRewards{
			PoolStake:   action.PoolStake,
			PoolRewards: iotago.Mana(poolRewards),
			FixedCost:   action.FixedCost,
		}
	}
}

func (t *TestSuite) AssertEpochRewards(epochIndex iotago.EpochIndex, actions map[string]*EpochActions) {
	for alias, action := range actions {
		poolRewards := t.poolRewards[epochIndex][alias].PoolRewards
		expectedValidatorReward := t.validatorReward(alias, epochIndex, t.epochStats[epochIndex].ProfitMargin, uint64(poolRewards), uint64(action.ValidatorStake), uint64(action.PoolStake), uint64(action.FixedCost), action)

		accountID := t.Account(alias, true)
		actualValidatorReward, _, _, err := t.Instance.ValidatorReward(accountID, actions[alias].ValidatorStake, epochIndex, epochIndex)
		require.NoError(t.T, err)
		require.Equal(t.T, expectedValidatorReward, actualValidatorReward)

		for delegatedAmount := range action.Delegators {
			expectedDelegatorReward := t.delegatorReward(epochIndex, t.epochStats[epochIndex].ProfitMargin, uint64(poolRewards), uint64(delegatedAmount), uint64(action.PoolStake), uint64(action.FixedCost), action)
			actualDelegatorReward, _, _, err := t.Instance.DelegatorReward(accountID, iotago.BaseToken(delegatedAmount), epochIndex, epochIndex)
			require.NoError(t.T, err)
			require.Equal(t.T, expectedDelegatorReward, actualDelegatorReward)
		}

	}
}

func (t *TestSuite) AssertNoReward(alias string, epoch iotago.EpochIndex, actions map[string]*EpochActions) {
	accID := t.Account(alias, false)
	actualValidatorReward, _, _, err := t.Instance.ValidatorReward(accID, actions[alias].ValidatorStake, epoch, epoch)
	require.NoError(t.T, err)
	require.Equal(t.T, iotago.Mana(0), actualValidatorReward)
	action, exists := actions[alias]
	require.True(t.T, exists)
	for delegatedAmount := range action.Delegators {
		actualDelegatorReward, _, _, err := t.Instance.DelegatorReward(accID, iotago.BaseToken(delegatedAmount), epoch, epoch)
		require.NoError(t.T, err)
		require.Equal(t.T, iotago.Mana(0), actualDelegatorReward)
	}
}

func (t *TestSuite) AssertRewardForDelegatorsOnly(alias string, epoch iotago.EpochIndex, actions map[string]*EpochActions) {
	accID := t.Account(alias, false)
	actualValidatorReward, _, _, err := t.Instance.ValidatorReward(accID, actions[alias].ValidatorStake, epoch, epoch)
	require.NoError(t.T, err)
	require.Equal(t.T, iotago.Mana(0), actualValidatorReward)
	action, exists := actions[alias]
	require.True(t.T, exists)

	for delegatedAmount := range action.Delegators {
		actualDelegatorReward, _, _, err := t.Instance.DelegatorReward(accID, iotago.BaseToken(delegatedAmount), epoch, epoch)
		expectedDelegatorReward := t.delegatorReward(epoch, t.epochStats[epoch].ProfitMargin, uint64(t.poolRewards[epoch][alias].PoolRewards), uint64(delegatedAmount), uint64(action.PoolStake), uint64(action.FixedCost), action)

		require.NoError(t.T, err)
		require.Equal(t.T, expectedDelegatorReward, actualDelegatorReward)
	}
}

func (t *TestSuite) validatorReward(alias string, epochIndex iotago.EpochIndex, profitMargin, poolRewards, stakeAmount, poolStake, fixedCost uint64, action *EpochActions) iotago.Mana {
	if action.ValidationBlocksSentPerSlot > uint64(t.api.ProtocolParameters().RewardsParameters().ValidatorBlocksPerSlot) {
		return iotago.Mana(0)
	}
	if action.FixedCost > t.poolRewards[epochIndex][alias].PoolRewards {
		fmt.Println("No rewards for validator: ", alias, " epoch: ", epochIndex)
		return iotago.Mana(0)
	}
	// punishment for too high fixed cost
	if poolRewards < fixedCost {
		return 0
	}

	profitMarginExponent := t.api.ProtocolParameters().RewardsParameters().ProfitMarginExponent
	profitMarginComplement := (1 << profitMarginExponent) - profitMargin
	poolRewardsNoFixedCost := poolRewards - fixedCost
	profitMarginFactor := (profitMargin * poolRewardsNoFixedCost) >> profitMarginExponent
	residualValidatorFactor := ((profitMarginComplement * poolRewardsNoFixedCost) >> profitMarginExponent) * stakeAmount / poolStake
	unDecayedEpochRewards := fixedCost + profitMarginFactor + residualValidatorFactor
	decayProvider := t.api.ManaDecayProvider()
	decayedEpochRewards, err := decayProvider.RewardsWithDecay(iotago.Mana(unDecayedEpochRewards), epochIndex, epochIndex)
	require.NoError(t.T, err)

	return decayedEpochRewards
}

func (t *TestSuite) delegatorReward(epochIndex iotago.EpochIndex, profitMargin, poolRewardWithFixedCost, delegatedAmount, poolStake, fixedCost uint64, action *EpochActions) iotago.Mana {
	if action.ValidationBlocksSentPerSlot > uint64(t.api.ProtocolParameters().RewardsParameters().ValidatorBlocksPerSlot) {
		return iotago.Mana(0)
	}

	profitMarginExponent := t.api.ProtocolParameters().RewardsParameters().ProfitMarginExponent
	profitMarginComplement := (1 << profitMarginExponent) - profitMargin

	poolRewards := poolRewardWithFixedCost
	if poolRewardWithFixedCost >= fixedCost {
		poolRewards = poolRewardWithFixedCost - fixedCost
	}
	unDecayedEpochRewards := (((profitMarginComplement * poolRewards) >> profitMarginExponent) * delegatedAmount) / poolStake

	decayProvider := t.api.ManaDecayProvider()
	decayedEpochRewards, err := decayProvider.RewardsWithDecay(iotago.Mana(unDecayedEpochRewards), epochIndex, epochIndex)
	require.NoError(t.T, err)

	return decayedEpochRewards
}

func (t *TestSuite) calculatePoolReward(epoch iotago.EpochIndex, totalValidatorsStake, totalStake, poolStake, validatorStake iotago.BaseToken, performanceFactor uint64) uint64 {
	params := t.api.ProtocolParameters()
	targetReward, err := params.RewardsParameters().TargetReward(epoch, t.api)
	require.NoError(t.T, err)

	poolCoefficient := t.calculatePoolCoefficient(poolStake, totalStake, validatorStake, totalValidatorsStake)
	scaledPoolReward := poolCoefficient * uint64(targetReward) * performanceFactor
	poolRewardNoFixedCost := scaledPoolReward / uint64(params.RewardsParameters().ValidatorBlocksPerSlot) >> (params.RewardsParameters().PoolCoefficientExponent + 1)

	return poolRewardNoFixedCost
}

func (t *TestSuite) calculatePoolCoefficient(poolStake, totalStake, validatorStake, totalValidatorStake iotago.BaseToken) uint64 {
	poolCoeffExponent := t.api.ProtocolParameters().RewardsParameters().PoolCoefficientExponent
	poolCoeff := (poolStake<<poolCoeffExponent)/totalStake +
		(validatorStake<<poolCoeffExponent)/totalValidatorStake

	return uint64(poolCoeff)
}

// calculateProfitMargin calculates the profit margin of the pool by firstly increasing the accuracy of the given value, so the profit margin is moved to the power of 2^accuracyShift.
func (t *TestSuite) calculateProfitMargin(totalValidatorsStake, totalPoolStake iotago.BaseToken) uint64 {
	return uint64((totalValidatorsStake << t.api.ProtocolParameters().RewardsParameters().ProfitMarginExponent) / (totalValidatorsStake + totalPoolStake))
}

func (t *TestSuite) applyPerformanceFactor(accountID iotago.AccountID, epochIndex iotago.EpochIndex, activeSlotsCount, validationBlocksSentPerSlot, slotPerformanceFactor uint64) {
	startSlot := t.api.TimeProvider().EpochStart(epochIndex)
	endSlot := t.api.TimeProvider().EpochEnd(epochIndex)
	valBlocksNum := t.api.ProtocolParameters().RewardsParameters().ValidatorBlocksPerSlot
	subslotDur := time.Duration(t.api.TimeProvider().SlotDurationSeconds()) * time.Second / time.Duration(valBlocksNum)

	slotCount := uint64(0)
	for slot := startSlot; slot <= endSlot; slot++ {
		if slotCount >= activeSlotsCount {
			// no more blocks issued by this validator in this epoch
			return
		}

		for i := uint64(0); i < validationBlocksSentPerSlot; i++ {
			valBlock := tpkg.ValidationBlock()
			block := tpkg.RandProtocolBlock(valBlock, t.api, 10)
			block.IssuerID = accountID
			subslotIndex := i
			// issued more than one block in the same slot to reduce performance factor
			if i >= slotPerformanceFactor {
				subslotIndex = 0
			}
			block.IssuingTime = t.api.TimeProvider().SlotStartTime(slot).Add(time.Duration(subslotIndex)*subslotDur + 1*time.Nanosecond)
			modelBlock, err := model.BlockFromBlock(block, t.api)
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

func (t *TestSuite) AssertValidatorRewardGreaterThan(alias1 string, alias2 string, epoch iotago.EpochIndex, actions map[string]*EpochActions) {
	accID1 := t.Account(alias1, false)
	actualValidatorReward1, _, _, err := t.Instance.ValidatorReward(accID1, actions[alias1].ValidatorStake, epoch, epoch)
	require.NoError(t.T, err)

	accID2 := t.Account(alias2, false)
	actualValidatorReward2, _, _, err := t.Instance.ValidatorReward(accID2, actions[alias2].ValidatorStake, epoch, epoch)
	require.NoError(t.T, err)

	require.Greater(t.T, actualValidatorReward1, actualValidatorReward2)
}

type EpochActions struct {
	PoolStake      iotago.BaseToken
	ValidatorStake iotago.BaseToken
	Delegators     []iotago.BaseToken
	FixedCost      iotago.Mana
	// ActiveSlotsCount is the number of firsts slots the validator was active in the epoch. If lower than slotsPerEpoch then validator went offline after ActiveSlotsCount.
	ActiveSlotsCount uint64
	// ValidationBlocksSentPerSlot is the number of validation blocks validator sent per slot.
	ValidationBlocksSentPerSlot uint64
	// SlotPerformance is the target slot performance factor, how many subslots were covered by validator blocks.
	SlotPerformance uint64
}

func (e *EpochActions) validate(t *testing.T, api iotago.API) {
	delegatorsTotal := iotago.BaseToken(0)
	for _, delegatorStake := range e.Delegators {
		delegatorsTotal += delegatorStake
	}
	require.Equal(t, e.PoolStake, delegatorsTotal+e.ValidatorStake, "pool stake must be equal to the sum of delegators stakes plus validator")

	sumOfSlots := 1 << api.ProtocolParameters().TimeProvider().SlotsPerEpochExponent()
	require.LessOrEqual(t, e.ActiveSlotsCount, uint64(sumOfSlots), "active slots count must be less or equal to the number of slots in the epoch")

	require.LessOrEqual(t, e.SlotPerformance, e.ValidationBlocksSentPerSlot, "number of subslots covered cannot be greated than number of blocks sent in a slot")
}
