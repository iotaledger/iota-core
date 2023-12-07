package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/iotaledger/hive.go/core/safemath"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/stretchr/testify/require"

	iotago "github.com/iotaledger/iota.go/v4"
)

func setupValidatorTestsuite(t *testing.T) *testsuite.TestSuite {
	var slotDuration uint8 = 5
	var slotsPerEpochExponent uint8 = 5
	var validationBlocksPerSlot uint8 = 5

	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithStakingOptions(1, validationBlocksPerSlot, 1),
			// Pick larger values for ManaShareCoefficient and DecayBalancingConstant for more precision in the calculations.
			iotago.WithRewardsOptions(8, 8, 11, 1154, 200, 200),
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(1000, slotDuration),
				slotDuration,
				slotsPerEpochExponent,
			),
			iotago.WithLivenessOptions(
				testsuite.DefaultLivenessThresholdLowerBoundInSeconds,
				testsuite.DefaultLivenessThresholdUpperBoundInSeconds,
				5,
				10,
				15,
			),
		),
	)

	// Add validator nodes to the network. This will add validator accounts to the snapshot.
	vnode1 := ts.AddValidatorNode("node1", testsuite.WithWalletAmount(20_000_000))
	vnode2 := ts.AddValidatorNode("node2", testsuite.WithWalletAmount(25_000_000))
	// Add a non-validator node to the network. This will not add any accounts to the snapshot.
	node3 := ts.AddNode("node3")
	// Add a default block issuer to the network. This will add another block issuer account to the snapshot.
	ts.AddDefaultWallet(vnode1)

	ts.Run(true)

	// For debugging set the log level appropriately.
	vnode1.Protocol.SetLogLevel(log.LevelInfo)
	vnode2.Protocol.SetLogLevel(log.LevelError)
	node3.Protocol.SetLogLevel(log.LevelError)

	ts.SetCurrentSlot(1)

	return ts
}

type EpochPerformanceMap = map[iotago.EpochIndex]uint64
type ValidatorTest struct {
	ts *testsuite.TestSuite

	// How many validation blocks per slot the validator test nodes are supposed to issue.
	// Can be set to a different value than the protocol parameter to model under- or overissuance.
	validationBlocksPerSlot uint8
	epochPerformanceFactors EpochPerformanceMap
}

func Test_Validator_PerfectIssuance(t *testing.T) {
	ts := setupValidatorTestsuite(t)
	defer ts.Shutdown()

	validationBlocksPerSlot := uint64(ts.API.ProtocolParameters().ValidationBlocksPerSlot())
	epochDurationSlots := uint64(ts.API.TimeProvider().EpochDurationSlots())

	test := ValidatorTest{
		ts:                      ts,
		validationBlocksPerSlot: ts.API.ProtocolParameters().ValidationBlocksPerSlot(),
		epochPerformanceFactors: EpochPerformanceMap{
			// A validator cannot issue blocks in the genesis slot, so we deduct one slot worth of blocks.
			0: (validationBlocksPerSlot * (epochDurationSlots - 1)) >> uint64(ts.API.ProtocolParameters().SlotsPerEpochExponent()),
			1: (validationBlocksPerSlot * (epochDurationSlots)) >> uint64(ts.API.ProtocolParameters().SlotsPerEpochExponent()),
			2: (validationBlocksPerSlot * (epochDurationSlots)) >> uint64(ts.API.ProtocolParameters().SlotsPerEpochExponent()),
		},
	}

	validatorTest(t, test)
}

func Test_Validator_OverIssuance(t *testing.T) {
	ts := setupValidatorTestsuite(t)
	defer ts.Shutdown()

	test := ValidatorTest{
		ts: ts,
		// Issue one more block than supposed to.
		validationBlocksPerSlot: ts.API.ProtocolParameters().ValidationBlocksPerSlot() + 1,
		epochPerformanceFactors: EpochPerformanceMap{
			// We expect 0 rewards for overissuance.
			// We model that in this test by setting performance factor to 0.
			0: 0,
			1: 0,
			2: 0,
		},
	}

	validatorTest(t, test)
}

func Test_Validator_UnderIssuance(t *testing.T) {
	ts := setupValidatorTestsuite(t)
	defer ts.Shutdown()

	// Issue less than supposed to.
	validationBlocksPerSlot := uint64(ts.API.ProtocolParameters().ValidationBlocksPerSlot() - 2)
	epochDurationSlots := uint64(ts.API.TimeProvider().EpochDurationSlots())

	test := ValidatorTest{
		ts:                      ts,
		validationBlocksPerSlot: uint8(validationBlocksPerSlot),
		epochPerformanceFactors: EpochPerformanceMap{
			// A validator cannot issue blocks in the genesis slot, so we deduct one slot worth of blocks.
			0: (validationBlocksPerSlot * (epochDurationSlots - 1)) >> uint64(ts.API.ProtocolParameters().SlotsPerEpochExponent()),
			1: (validationBlocksPerSlot * (epochDurationSlots)) >> uint64(ts.API.ProtocolParameters().SlotsPerEpochExponent()),
			2: (validationBlocksPerSlot * (epochDurationSlots)) >> uint64(ts.API.ProtocolParameters().SlotsPerEpochExponent()),
		},
	}

	validatorTest(t, test)
}

func validatorTest(t *testing.T, test ValidatorTest) {
	ts := test.ts

	tip := ts.DefaultWallet().Node.Protocol.Engines.Main.Get().TipSelection.SelectTips(1)[iotago.StrongParentType][0]
	startEpoch := iotago.EpochIndex(0)
	endEpoch := iotago.EpochIndex(len(test.epochPerformanceFactors) - 1)
	fmt.Printf("ValidatorTest: startEpoch=%d, endEpoch=%d\n", startEpoch, endEpoch)

	subslotDuration := time.Duration(ts.API.ProtocolParameters().SlotDurationInSeconds()/test.validationBlocksPerSlot) * time.Second
	// Validate until the last slot of the epoch is definitely committed.
	var endEpochSlot iotago.SlotIndex = ts.API.TimeProvider().EpochEnd(endEpoch) + ts.API.ProtocolParameters().MaxCommittableAge()
	// Needed to increase the block timestamp monotonically relative to the parent.
	subSlotBlockCounter := time.Duration(0)

	for slot := ts.CurrentSlot(); slot <= endEpochSlot; slot++ {
		ts.SetCurrentSlot(slot)

		slotStartTime := ts.API.TimeProvider().SlotStartTime(ts.CurrentSlot()).UTC()
		for subslotIdx := uint8(0); subslotIdx < test.validationBlocksPerSlot; subslotIdx++ {
			for _, node := range ts.Validators() {
				blockName := fmt.Sprintf("block-%s-%d/%d", node.Name, ts.CurrentSlot(), subslotIdx)
				latestCommitment := node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment()

				issuingTime := slotStartTime.
					Add(subslotDuration * time.Duration(subslotIdx)).
					Add(time.Duration(subSlotBlockCounter))

				validationBlock := ts.IssueValidationBlockWithHeaderOptions(blockName, node,
					mock.WithSlotCommitment(latestCommitment),
					mock.WithIssuingTime(issuingTime),
					mock.WithStrongParents(tip),
				)

				tip = validationBlock.ID()
				subSlotBlockCounter += 1
			}
		}
	}

	ts.Wait(ts.Nodes()...)

	// Determine total stakes required for rewards calculation.
	var totalStake iotago.BaseToken = 0
	var totalValidatorStake iotago.BaseToken = 0
	lo.ForEach(ts.Validators(), func(n *mock.Node) {
		latestCommittedSlot := ts.DefaultWallet().Node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Slot()
		accountData, exists, err := ts.DefaultWallet().Node.Protocol.Engines.Main.Get().Ledger.Account(n.Validator.AccountID, latestCommittedSlot)
		if err != nil || !exists {
			t.Fatal(exists, err)
		}

		totalStake += accountData.ValidatorStake + accountData.DelegationStake
		totalValidatorStake += accountData.ValidatorStake
	})

	// Determine the rewards the validators actually got.
	actualRewards := make(map[iotago.AccountID]iotago.Mana, len(ts.Validators()))
	claimingEpoch := ts.API.TimeProvider().EpochFromSlot(ts.CurrentSlot())

	for _, validatorAccount := range []string{"Genesis:1", "Genesis:2"} {
		output := ts.DefaultWallet().Output(validatorAccount)
		accountID := output.Output().(*iotago.AccountOutput).AccountID

		rewardMana, _, _, err := ts.DefaultWallet().Node.Protocol.Engines.Main.Get().SybilProtection.ValidatorReward(
			accountID,
			output.Output().FeatureSet().Staking().StakedAmount,
			output.Output().FeatureSet().Staking().StartEpoch,
			claimingEpoch,
		)
		if err != nil {
			panic(err)
		}

		actualRewards[accountID] = rewardMana
	}

	for accountID, actualReward := range actualRewards {
		lastRewardEpoch := iotago.EpochIndex(len(test.epochPerformanceFactors))
		rewards := make([]reward, 0, lastRewardEpoch)
		for epoch := iotago.EpochIndex(0); epoch < lastRewardEpoch; epoch++ {
			epochPerformanceFactor := test.epochPerformanceFactors[epoch]
			reward := calculateEpochReward(t, ts, accountID, epoch, epochPerformanceFactor, totalStake, totalValidatorStake)
			rewards = append(rewards, reward)
		}

		expectedReward := calculateValidatorReward(t, ts, accountID, rewards, startEpoch, claimingEpoch)

		require.Equal(t, expectedReward, actualReward, "expected reward for account %s to be %d, was %d", accountID, expectedReward, actualReward)
	}
}

type reward struct {
	Mana         iotago.Mana
	ProfitMargin uint64
}

// Calculates the reward according to
// https://github.com/iotaledger/tips/blob/tip40/tips/TIP-0040/tip-0040.md#calculations-3.
//
// For testing purposes, assumes that the account's staking data is the same in the latest committed slot
// as in the epoch for which to calculate rewards.
func calculateEpochReward(t *testing.T, ts *testsuite.TestSuite, accountID iotago.AccountID, epoch iotago.EpochIndex, epochPerformanceFactor uint64, totalStake iotago.BaseToken, totalValidatorStake iotago.BaseToken) reward {

	latestCommittedSlot := ts.DefaultWallet().Node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Slot()
	targetReward := lo.PanicOnErr(ts.API.ProtocolParameters().RewardsParameters().TargetReward(epoch, ts.API))
	accountData, exists, err := ts.DefaultWallet().Node.Protocol.Engines.Main.Get().Ledger.Account(accountID, latestCommittedSlot)
	if err != nil || !exists {
		t.Fatal(exists, err)
	}

	poolStake := accountData.ValidatorStake + accountData.DelegationStake
	poolCoefficientExp := iotago.BaseToken(ts.API.ProtocolParameters().RewardsParameters().PoolCoefficientExponent)

	validationBlocksPerSlot := ts.API.ProtocolParameters().ValidationBlocksPerSlot()
	poolCoefficient := ((poolStake << poolCoefficientExp) / totalStake) + (accountData.ValidatorStake<<poolCoefficientExp)/totalValidatorStake

	scaledPoolReward := iotago.Mana(poolCoefficient) * targetReward * iotago.Mana(epochPerformanceFactor)
	poolReward := (scaledPoolReward / iotago.Mana(validationBlocksPerSlot)) >> (poolCoefficientExp + 1)

	profitMargin := (totalValidatorStake << iotago.BaseToken(ts.API.ProtocolParameters().RewardsParameters().ProfitMarginExponent)) / (totalValidatorStake + totalStake)

	return reward{Mana: poolReward, ProfitMargin: uint64(profitMargin)}
}

// Calculates a validator's reward according to
// https://github.com/iotaledger/tips/blob/tip40/tips/TIP-0040/tip-0040.md#calculations-4.
func calculateValidatorReward(t *testing.T, ts *testsuite.TestSuite, accountID iotago.AccountID, rewards []reward, startEpoch iotago.EpochIndex, claimingEpoch iotago.EpochIndex) iotago.Mana {
	latestCommittedSlot := ts.DefaultWallet().Node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Slot()
	accountData, exists, err := ts.DefaultWallet().Node.Protocol.Engines.Main.Get().Ledger.Account(accountID, latestCommittedSlot)
	if err != nil || !exists {
		t.Fatal(exists, err)
	}

	profitMarginExponent := ts.API.ProtocolParameters().RewardsParameters().ProfitMarginExponent
	stakedAmount := accountData.ValidatorStake
	fixedCost := accountData.FixedCost
	poolStake := accountData.ValidatorStake + accountData.DelegationStake
	rewardEpoch := startEpoch
	decayedRewards := iotago.Mana(0)

	for _, reward := range rewards {
		if reward.Mana >= fixedCost {
			rewardWithoutFixedCost := uint64(reward.Mana) - uint64(fixedCost)

			profitMarginComplement := (1 << profitMarginExponent) - reward.ProfitMargin
			profitMarginFactor := (reward.ProfitMargin * rewardWithoutFixedCost) >> profitMarginExponent

			intermediate := ((profitMarginComplement * rewardWithoutFixedCost) >> profitMarginExponent)
			residualValidatorFactor := lo.PanicOnErr(safemath.Safe64MulDiv(intermediate, uint64(stakedAmount), uint64(poolStake)))

			undecayedRewards := uint64(fixedCost) + profitMarginFactor + residualValidatorFactor

			decayedRewards += lo.PanicOnErr(ts.API.ManaDecayProvider().DecayManaByEpochs(iotago.Mana(undecayedRewards), rewardEpoch, claimingEpoch))
		} else {
			decayedRewards += 0
		}

		rewardEpoch++
	}

	return decayedRewards
}
