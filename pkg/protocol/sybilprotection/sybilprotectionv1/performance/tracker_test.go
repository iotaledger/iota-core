package performance

import (
	"testing"

	iotago "github.com/iotaledger/iota.go/v4"
)

func TestManager_Rewards(t *testing.T) {
	ts := NewTestSuite(t)
	// performance factor testing
	epoch := iotago.EpochIndex(2)
	epochActions := map[string]*EpochActions{
		"A": {
			PoolStake:                   200,
			ValidatorStake:              40,
			Delegators:                  []iotago.BaseToken{20, 40, 40, 40, 20},
			FixedCost:                   10,
			ActiveSlotsCount:            8, // ideal performance
			ValidationBlocksSentPerSlot: 10,
			SlotPerformance:             10,
		},
		"B": {
			PoolStake:                   200,
			ValidatorStake:              40,
			Delegators:                  []iotago.BaseToken{20, 20, 10, 30, 80},
			FixedCost:                   10,
			ActiveSlotsCount:            8,
			ValidationBlocksSentPerSlot: 6, // versus low performance, one block per subslot
			SlotPerformance:             6,
		},
		"C": {
			PoolStake:                   200,
			ValidatorStake:              40,
			Delegators:                  []iotago.BaseToken{20, 20, 10, 30, 80},
			FixedCost:                   10,
			ActiveSlotsCount:            8,
			ValidationBlocksSentPerSlot: 10, // versus the same performance, many blocks in one subslot
			SlotPerformance:             4,
		},
	}
	ts.ApplyEpochActions(epoch, epochActions)
	ts.AssertEpochRewards(epoch, epochActions)
	// better performin validator should get more rewards
	ts.AssertValidatorRewardGreaterThan("A", "B", epoch, epochActions)

	epoch = iotago.EpochIndex(3)
	epochActions = map[string]*EpochActions{
		"A": {
			PoolStake:                   10,
			ValidatorStake:              5,
			Delegators:                  []iotago.BaseToken{2, 3},
			FixedCost:                   10,
			ActiveSlotsCount:            6, // validator dropped out for two last slots
			ValidationBlocksSentPerSlot: 10,
			SlotPerformance:             10,
		},
		"C": {
			PoolStake:                   10,
			ValidatorStake:              5,
			Delegators:                  []iotago.BaseToken{3, 2},
			FixedCost:                   100,
			ActiveSlotsCount:            8,
			ValidationBlocksSentPerSlot: uint64(ts.api.ProtocolParameters().RewardsParameters().ValidatorBlocksPerSlot + 2), // no reward for validator issuing more blocks than allowed
			SlotPerformance:             10,
		},
		"D": {
			PoolStake:                   10,
			ValidatorStake:              5,
			Delegators:                  []iotago.BaseToken{3, 2},
			FixedCost:                   100_000_000_000, // fixed cost higher than the pool reward, no reward for validator
			ActiveSlotsCount:            8,
			ValidationBlocksSentPerSlot: 10,
			SlotPerformance:             10,
		},
	}
	ts.ApplyEpochActions(epoch, epochActions)
	ts.AssertEpochRewards(epoch, epochActions)
	ts.AssertNoReward("C", epoch, epochActions)
	ts.AssertRewardForDelegatorsOnly("D", epoch, epochActions)

	// test the epoch after initial phase

}
