package performance

import "testing"

func TestManager_Rewards(t *testing.T) {
	ts := NewTestSuite(t)

	epochActions := map[string]*EpochActions{
		"A": {
			PoolStake:            10,
			ValidatorStake:       4,
			FixedCost:            1,
			ValidationBlocksSent: 10,
		},
		"B": {
			PoolStake:            20,
			ValidatorStake:       8,
			FixedCost:            100,
			ValidationBlocksSent: 3,
		},
	}
	ts.ApplyEpochActions(2, epochActions)
	ts.AssertEpochRewards(2, epochActions,
		&PoolsStats{
			TotalStake:          30,
			TotalValidatorStake: 12,
			ProfitMargin:        1,
		})

}
