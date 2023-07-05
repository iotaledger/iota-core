package performance

import (
	"testing"

	"github.com/orcaman/writerseeker"
	"github.com/stretchr/testify/require"

	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func TestManager_Import_Export(t *testing.T) {
	ts := NewTestSuite(t)
	epochsCount := 3
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
	for i := 1; i <= epochsCount; i++ {
		ts.ApplyEpochActions(iotago.EpochIndex(i), epochActions)
	}

	{
		writer := &writerseeker.WriterSeeker{}

		delegatorRewardBeforeImport, validatorRewardBeforeImport := ts.calculateExpectedRewards(epochsCount, epochActions)
		// export two full epochs
		targetSlot := tpkg.TestAPI.TimeProvider().EpochEnd(3)
		err := ts.Instance.Export(writer, targetSlot)
		require.NoError(t, err)

		ts.InitRewardManager()

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
		targetSlot := tpkg.TestAPI.TimeProvider().EpochStart(2)
		err := ts.Instance.Export(writer, targetSlot)
		require.NoError(t, err)

		ts.InitRewardManager()

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
