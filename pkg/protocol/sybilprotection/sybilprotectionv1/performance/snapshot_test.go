package performance

import (
	"testing"

	"github.com/orcaman/writerseeker"
	"github.com/stretchr/testify/require"

	iotago "github.com/iotaledger/iota.go/v4"
)

func TestManager_Import_Export(t *testing.T) {
	ts := NewTestSuite(t)
	epochsCount := 3
	epochActions := map[string]*EpochActions{
		"A": {
			PoolStake:                   10,
			ValidatorStake:              4,
			Delegators:                  []iotago.BaseToken{3, 3},
			FixedCost:                   1,
			ActiveSlotsCount:            8, // we have 8 slots in epoch
			ValidationBlocksSentPerSlot: 10,
			SlotPerformance:             7,
		},
		"B": {
			PoolStake:                   20,
			ValidatorStake:              8,
			Delegators:                  []iotago.BaseToken{1, 2, 4, 5},
			FixedCost:                   100,
			ActiveSlotsCount:            6,
			ValidationBlocksSentPerSlot: 3,
			SlotPerformance:             3,
		},
	}
	for i := 1; i <= epochsCount; i++ {
		ts.ApplyEpochActions(iotago.EpochIndex(i), epochActions)
	}

	{
		writer := &writerseeker.WriterSeeker{}

		delegatorRewardBeforeImport, validatorRewardBeforeImport := ts.calculateExpectedRewards(epochsCount, epochActions)
		// export two full epochs
		targetSlot := ts.api.TimeProvider().EpochEnd(3)
		err := ts.Instance.Export(writer, targetSlot)
		require.NoError(t, err)

		ts.InitPerformanceTracker()

		err = ts.Instance.Import(writer.BytesReader())
		require.NoError(t, err)
		delegatorRewardAfterImport, validatorRewardAfterImport := ts.calculateExpectedRewards(epochsCount, epochActions)
		require.Equal(t, delegatorRewardBeforeImport, delegatorRewardAfterImport)
		require.Equal(t, validatorRewardBeforeImport, validatorRewardAfterImport)
	}
	{
		writer := &writerseeker.WriterSeeker{}

		delegatorRewardBeforeImport, validatorRewardBeforeImport := ts.calculateExpectedRewards(epochsCount, epochActions)
		// export at the beginning of epoch 2, skip epoch 3 at all
		targetSlot := ts.api.TimeProvider().EpochStart(2)
		err := ts.Instance.Export(writer, targetSlot)
		require.NoError(t, err)

		ts.InitPerformanceTracker()

		err = ts.Instance.Import(writer.BytesReader())
		require.NoError(t, err)
		delegatorRewardAfterImport, validatorRewardAfterImport := ts.calculateExpectedRewards(epochsCount, epochActions)

		require.Equal(t, delegatorRewardBeforeImport[1]["A"], delegatorRewardAfterImport[1]["A"])
		require.Equal(t, delegatorRewardBeforeImport[1]["B"], delegatorRewardAfterImport[1]["B"])
		require.EqualValues(t, delegatorRewardAfterImport[2]["A"], 0)
		require.EqualValues(t, delegatorRewardAfterImport[2]["B"], 0)
		require.EqualValues(t, delegatorRewardAfterImport[3]["A"], 0)
		require.EqualValues(t, delegatorRewardAfterImport[3]["B"], 0)
		require.EqualValues(t, delegatorRewardAfterImport[4]["A"], 0)
		require.EqualValues(t, delegatorRewardAfterImport[4]["B"], 0)

		require.EqualValues(t, validatorRewardBeforeImport[1]["A"], validatorRewardAfterImport[1]["A"])
		require.EqualValues(t, validatorRewardBeforeImport[1]["B"], validatorRewardAfterImport[1]["B"])
		require.EqualValues(t, validatorRewardAfterImport[2]["A"], 0)
		require.EqualValues(t, validatorRewardAfterImport[2]["B"], 0)
		require.EqualValues(t, validatorRewardAfterImport[3]["A"], 0)
		require.EqualValues(t, validatorRewardAfterImport[3]["B"], 0)
		require.EqualValues(t, validatorRewardAfterImport[4]["A"], 0)
		require.EqualValues(t, validatorRewardAfterImport[4]["B"], 0)

	}
}
