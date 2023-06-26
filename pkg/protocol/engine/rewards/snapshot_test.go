package rewards

import (
	"testing"

	"github.com/orcaman/writerseeker"
	"github.com/stretchr/testify/require"
)

func TestManager_Import_Export(t *testing.T) {
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
	ts.ApplyEpochActions(1, epochActions)
	ts.ApplyEpochActions(2, epochActions)
	writer := &writerseeker.WriterSeeker{}
	targetSlot := ts.API().TimeProvider().EpochEnd(2)
	err := ts.Instance.Export(writer, targetSlot)
	require.NoError(t, err)
	err = ts.Instance.Import(writer.BytesReader())
	require.NoError(t, err)
	// export at the begining of the epoch
	targetSlot = ts.API().TimeProvider().EpochStart(2)
	err = ts.Instance.Export(writer, targetSlot)
	require.NoError(t, err)
	err = ts.Instance.Import(writer.BytesReader())
	require.NoError(t, err)

	// TODO assert state
}
