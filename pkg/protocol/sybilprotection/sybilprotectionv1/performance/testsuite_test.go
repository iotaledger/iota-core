package performance

import (
	"fmt"
	"testing"

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

	Instance *Tracker
}

func NewTestSuite(t *testing.T) *TestSuite {
	apiProvider := api.NewEpochBasedProvider()
	apiProvider.AddProtocolParametersAtEpoch(iotago.NewV3ProtocolParameters(), 0)
	apiProvider.AddProtocolParametersAtEpoch(iotago.NewV3ProtocolParameters(), 1)
	apiProvider.AddProtocolParametersAtEpoch(iotago.NewV3ProtocolParameters(), 2)
	apiProvider.AddProtocolParametersAtEpoch(iotago.NewV3ProtocolParameters(), 3)

	ts := &TestSuite{
		T:           t,
		accounts:    make(map[string]iotago.AccountID),
		apiProvider: apiProvider,
	}
	ts.InitRewardManager()

	return ts
}

func (t *TestSuite) InitRewardManager() {
	prunableStores := make(map[iotago.SlotIndex]kvstore.KVStore)
	perforanceFactorFunc := func(index iotago.SlotIndex) *prunable.VlidatorSlotPerformance {
		if _, exists := prunableStores[index]; !exists {
			prunableStores[index] = mapdb.NewMapDB()
		}

		p := prunable.NewPerformanceFactors(index, prunableStores[index], t.apiProvider)

		return p
	}

	rewardsStore := mapdb.NewMapDB()
	poolStatsStore := mapdb.NewMapDB()
	committeeStore := mapdb.NewMapDB()
	t.Instance = NewTracker(rewardsStore, poolStatsStore, committeeStore, perforanceFactorFunc, t.latestCommittedEpoch, t.apiProvider, func(err error) {})
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
		t.applyPerformanceFactor(accID, epochIndex, action.ValidationBlocksSent)
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
	// TODO: finish and assert profit margin
	// profitMarging := (1 << 8) * totalValidatorsStake / (totalValidatorsStake + totalStake)
	// require.Equal(t.T, profitMarging, expectedStats.ProfitMargin)
	for alias, action := range actions {
		accountID := t.Account(alias, false)
		performanceFactor := action.ValidationBlocksSent

		epochsInSlot := 1 << 13
		targetRewardPerEpoch := uint64(233373068869021000 * epochsInSlot)
		aux := (((1 << 31) * action.PoolStake) / totalStake) + ((2 << 31) * action.ValidatorStake / totalValidatorsStake)
		aux2 := uint64(aux) * targetRewardPerEpoch * performanceFactor
		expectedRewardWithFixedCost := iotago.Mana(aux2 >> 40)
		actualValidatorReward, _, _, err := t.Instance.ValidatorReward(accountID, actions[alias].ValidatorStake, epochIndex, epochIndex)
		require.NoError(t.T, err)
		delegatorStake := actions[alias].PoolStake - actions[alias].ValidatorStake
		actualDelegatorReward, _, _, err := t.Instance.DelegatorReward(accountID, delegatorStake, epochIndex, epochIndex)
		require.NoError(t.T, err)
		fmt.Printf("expected: %d, actual: %d\n", expectedRewardWithFixedCost, actualValidatorReward+actualDelegatorReward)
		// TODO: require.EqualValues(t.T, expectedRewardWithFixedCost, actualValidatorReward+actualDelegatorReward)
	}
}

func (t *TestSuite) applyPerformanceFactor(accountID iotago.AccountID, epochIndex iotago.EpochIndex, performanceFactor uint64) {
	startSlot := t.apiProvider.APIForEpoch(epochIndex).TimeProvider().EpochStart(epochIndex)
	// TODO change params instad of hand typing here, also how to make it faster without shortening the epoch length?
	//endSlot := t.apiProvider.APIForEpoch(epochIndex).TimeProvider().EpochEnd(epochIndex)
	endSlot := startSlot + 10
	for slot := startSlot; slot <= endSlot; slot++ {
		for i := uint64(0); i < performanceFactor; i++ {
			block := tpkg.RandBasicBlockWithIssuerAndRMC(accountID, 10)
			block.IssuingTime = t.apiProvider.APIForEpoch(epochIndex).TimeProvider().SlotStartTime(slot)
			modelBlock, err := model.BlockFromBlock(block, t.apiProvider.APIForEpoch(epochIndex))
			t.Instance.TrackValidationBlock(blocks.NewBlock(modelBlock))

			require.NoError(t.T, err)
		}
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
	PoolStake            iotago.BaseToken
	ValidatorStake       iotago.BaseToken
	FixedCost            iotago.Mana
	ValidationBlocksSent uint64
}
