package performance

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type TestSuite struct {
	T *testing.T

	accounts map[string]iotago.AccountID

	ProtocolParameters *iotago.ProtocolParameters

	Instance *Tracker
}

func NewTestSuite(t *testing.T) *TestSuite {
	ts := &TestSuite{
		T:        t,
		accounts: make(map[string]iotago.AccountID),

		ProtocolParameters: &iotago.ProtocolParameters{
			Version:     3,
			NetworkName: utils.RandString(255),
			Bech32HRP:   iotago.NetworkPrefix(utils.RandString(3)),
			MinPoWScore: utils.RandUint32(50000),
			RentStructure: iotago.RentStructure{
				VByteCost:    100,
				VBFactorData: 1,
				VBFactorKey:  10,
			},
			TokenSupply:                      tpkg.RandBaseToken(math.MaxUint64),
			GenesisUnixTimestamp:             time.Now().Unix(),
			SlotDurationInSeconds:            10,
			SlotsPerEpochExponent:            1,
			ManaDecayFactors:                 []uint32{},
			ManaDecayFactorEpochsSum:         0,
			ManaDecayFactorEpochsSumExponent: 0,
			ManaDecayFactorsExponent:         0,
		},
	}
	ts.InitRewardManager()

	return ts
}

func (t *TestSuite) API() iotago.API {
	return iotago.LatestAPI(t.ProtocolParameters)
}

func (t *TestSuite) InitRewardManager() {
	prunableStores := make(map[iotago.SlotIndex]kvstore.KVStore)
	perforanceFactorFunc := func(index iotago.SlotIndex) *prunable.PerformanceFactors {
		if _, exists := prunableStores[index]; !exists {
			prunableStores[index] = mapdb.NewMapDB()
		}

		p := prunable.NewPerformanceFactors(index, prunableStores[index])

		return p
	}

	rewardsStore := mapdb.NewMapDB()
	poolStatsStore := mapdb.NewMapDB()
	committeeStore := mapdb.NewMapDB()
	t.Instance = NewTracker(rewardsStore, poolStatsStore, committeeStore, perforanceFactorFunc, t.API().TimeProvider(), t.API().ManaDecayProvider())
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
}

func (t *TestSuite) AssertEpochRewards(epochIndex iotago.EpochIndex, actions map[string]*EpochActions) {
	totalStake := iotago.BaseToken(0)
	totalValidatorsStake := iotago.BaseToken(0)
	for _, action := range actions {
		totalStake += action.PoolStake
		totalValidatorsStake += action.ValidatorStake
	}
	// TODO: finish and assert profit margin
	//profitMarging := (1 << 8) * totalValidatorsStake / (totalValidatorsStake + totalStake)
	//require.Equal(t.T, profitMarging, expectedStats.ProfitMargin)
	for alias, action := range actions {
		accountID := t.Account(alias, false)
		performanceFactor := action.ValidationBlocksSent

		epochsInSlot := 1 << 13
		targetRewardPerEpoch := uint64(233373068869021000 * epochsInSlot)
		aux := (((1 << 31) * action.PoolStake) / totalStake) + ((2 << 31) * action.ValidatorStake / totalValidatorsStake)
		aux2 := uint64(aux) * targetRewardPerEpoch * performanceFactor
		expectedRewardWithFixedCost := iotago.Mana(aux2 >> 40)
		actualValidatorReward, err := t.Instance.ValidatorReward(accountID, actions[alias].ValidatorStake, epochIndex, epochIndex)
		require.NoError(t.T, err)
		delegatorStake := actions[alias].PoolStake - actions[alias].ValidatorStake
		actualDelegatorReward, err := t.Instance.DelegatorReward(accountID, delegatorStake, epochIndex, epochIndex)
		require.NoError(t.T, err)
		fmt.Printf("expected: %d, actual: %d\n", expectedRewardWithFixedCost, actualValidatorReward+actualDelegatorReward)
		//TODO: require.EqualValues(t.T, expectedRewardWithFixedCost, actualValidatorReward+actualDelegatorReward)
	}

}

func (t *TestSuite) applyPerformanceFactor(accountID iotago.AccountID, epochIndex iotago.EpochIndex, performanceFactor uint64) {
	startSlot := t.API().TimeProvider().EpochStart(epochIndex)
	endSlot := t.API().TimeProvider().EpochEnd(epochIndex)
	for slot := startSlot; slot <= endSlot; slot++ {
		for i := uint64(0); i < performanceFactor; i++ {
			block := tpkg.RandBlockWithIssuerAndBurnedMana(accountID, 10)
			block.IssuingTime = t.API().TimeProvider().SlotStartTime(slot)
			modelBlock, err := model.BlockFromBlock(block, t.API())
			t.Instance.BlockAccepted(blocks.NewBlock(modelBlock))

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
			reward, err := t.Instance.DelegatorReward(t.Account(aliasAccount, false), 1, epochIndex, epochIndex)
			require.NoError(t.T, err)
			delegatorRewardPerAccount[epochIndex][aliasAccount] = reward
		}
		for aliasAccount := range epochActions {
			reward, err := t.Instance.ValidatorReward(t.Account(aliasAccount, true), 1, epochIndex, epochIndex)
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
