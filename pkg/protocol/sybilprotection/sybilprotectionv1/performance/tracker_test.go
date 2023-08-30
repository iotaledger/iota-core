package performance

import (
	"testing"

	iotago "github.com/iotaledger/iota.go/v4"
)

func TestManager_Rewards(t *testing.T) {
	ts := NewTestSuite(t)

	epochActions := map[string]*EpochActions{
		"A": {
			PoolStake:                   10,
			ValidatorStake:              4,
			Delegators:                  []iotago.BaseToken{2, 4},
			FixedCost:                   1,
			ActiveSlotsCount:            8,
			ValidationBlocksSentPerSlot: 10,
			SlotPerformance:             8,
		},
		"B": {
			PoolStake:                   20,
			ValidatorStake:              8,
			Delegators:                  []iotago.BaseToken{1, 2, 4, 5},
			FixedCost:                   100,
			ActiveSlotsCount:            6,
			ValidationBlocksSentPerSlot: 3,
			SlotPerformance:             10,
		},
	}
	ts.ApplyEpochActions(2, epochActions)
	ts.AssertEpochRewards(2, epochActions)

}
