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
	for i := 0; i < epochsCount; i++ {
		ts.ApplyEpochActions(iotago.EpochIndex(i), epochActions)
	}

	writer := &writerseeker.WriterSeeker{}

	delegtorRewardBeforeImport, validatorRewardBeforeImport := ts.saveCalculatedRewards(epochsCount, epochActions)
	// export two full epochs
	targetSlot := ts.API().TimeProvider().EpochEnd(3)
	err := ts.Instance.Export(writer, targetSlot)
	require.NoError(t, err)
	err = ts.Instance.Import(writer.BytesReader())
	require.NoError(t, err)
	delegtorRewardAfterImport, validatorRewardAfterImport := ts.saveCalculatedRewards(epochsCount, epochActions)
	require.Equal(t, delegtorRewardBeforeImport, delegtorRewardAfterImport)
	require.Equal(t, validatorRewardBeforeImport, validatorRewardAfterImport)

	delegtorRewardBeforeImport, validatorRewardBeforeImport = ts.saveCalculatedRewards(epochsCount, epochActions)
	// export at the begining of the epoch 2, skip epoch 3 at all
	targetSlot = ts.API().TimeProvider().EpochStart(2)
	err = ts.Instance.Export(writer, targetSlot)
	require.NoError(t, err)
	err = ts.Instance.Import(writer.BytesReader())
	require.NoError(t, err)
	delegtorRewardAfterImport, validatorRewardAfterImport = ts.saveCalculatedRewards(epochsCount, epochActions)
	require.Equal(t, delegtorRewardBeforeImport, delegtorRewardAfterImport)
	require.Equal(t, validatorRewardBeforeImport, validatorRewardAfterImport)
}
