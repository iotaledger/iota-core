package accountsledger_test

import (
	"testing"

	"github.com/orcaman/writerseeker"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func TestManager_Import_Export(t *testing.T) {
	ts := NewTestSuite(t)
	latestSupportedVersionHash1 := tpkg.Rand32ByteArray()
	latestSupportedVersionHash2 := tpkg.Rand32ByteArray()
	ts.ApplySlotActions(1, 5, map[string]*AccountActions{
		"A": {
			TotalAllotments: 10,
			NumBlocks:       1,
			AddedKeys:       []string{"A.P1"},

			NewOutputID: "A1",
		},
		"B": {
			TotalAllotments: 20,
			NumBlocks:       2,
			AddedKeys:       []string{"B.P1", "B.P2"},

			NewOutputID: "B1",
		},
		"D": { // create a staking account
			TotalAllotments: 0,
			AddedKeys:       []string{"D.P1"},

			ValidatorStakeChange:                  20,
			DelegationStakeChange:                 20,
			FixedCostChange:                       10,
			StakeEndEpochChange:                   10,
			LatestSupportedProtocolVersionAndHash: model.VersionAndHash{Version: 1, Hash: latestSupportedVersionHash1},

			NewOutputID: "D1",
		},
	})

	ts.AssertAccountLedgerUntil(1, map[string]*AccountState{
		"A": {
			BICUpdatedTime: 1,
			BICAmount:      5,
			PubKeys:        []string{"A.P1"},
			OutputID:       "A1",
		},
		"B": {
			BICUpdatedTime: 1,
			BICAmount:      10,
			PubKeys:        []string{"B.P1", "B.P2"},
			OutputID:       "B1",
		},
		"D": {
			BICUpdatedTime:                        1,
			BICAmount:                             0,
			PubKeys:                               []string{"D.P1"},
			OutputID:                              "D1",
			ValidatorStake:                        20,
			DelegationStake:                       20,
			FixedCost:                             10,
			StakeEndEpoch:                         10,
			LatestSupportedProtocolVersionAndHash: model.VersionAndHash{Version: 1, Hash: latestSupportedVersionHash1},
		},
	})

	ts.ApplySlotActions(2, 1, map[string]*AccountActions{
		"A": { // zero out the account data before removal
			NumBlocks:   5,
			RemovedKeys: []string{"A.P1"},

			NewOutputID: "A2",
		},
		"B": {
			TotalAllotments: 5,
			NumBlocks:       2,
			RemovedKeys:     []string{"B.P1"},

			NewOutputID: "B2",
		},
		"D": { // update only delegation stake
			DelegationStakeChange:                 -5,
			LatestSupportedProtocolVersionAndHash: model.VersionAndHash{Version: 3, Hash: latestSupportedVersionHash2},
		},
	})

	ts.AssertAccountLedgerUntil(2, map[string]*AccountState{
		"A": {
			BICUpdatedTime: 2,
			BICAmount:      0,
			PubKeys:        []string{},
			OutputID:       "A2",
		},
		"B": {
			BICUpdatedTime: 2,
			BICAmount:      13,
			PubKeys:        []string{"B.P2"},
			OutputID:       "B2",
		},
		"D": {
			BICUpdatedTime:                        1,
			BICAmount:                             0,
			PubKeys:                               []string{"D.P1"},
			OutputID:                              "D1",
			ValidatorStake:                        20,
			DelegationStake:                       15,
			FixedCost:                             10,
			StakeEndEpoch:                         10,
			LatestSupportedProtocolVersionAndHash: model.VersionAndHash{Version: 3, Hash: latestSupportedVersionHash2},
		},
	})

	ts.ApplySlotActions(3, 5, map[string]*AccountActions{
		"A": {
			Destroyed: true,
		},
		"B": {
			TotalAllotments: 10,
			NumBlocks:       1,
			AddedKeys:       []string{"B.P3"},

			NewOutputID: "B3",
		},
		"C": {
			TotalAllotments: 10,
			NumBlocks:       2,
			AddedKeys:       []string{"C.P1"},

			NewOutputID: "C1",
		},
		"D": {
			ValidatorStakeChange:                  40,
			DelegationStakeChange:                 -10,
			StakeEndEpochChange:                   8,
			FixedCostChange:                       -2,
			LatestSupportedProtocolVersionAndHash: model.VersionAndHash{Version: 3, Hash: latestSupportedVersionHash2},

			NewOutputID: "D2",
		},
	})

	ts.AssertAccountLedgerUntil(3, map[string]*AccountState{
		"A": {
			Destroyed: true,

			BICUpdatedTime: 3,
		},
		"B": {
			BICUpdatedTime: 3,
			BICAmount:      18,
			PubKeys:        []string{"B.P2", "B.P3"},
			OutputID:       "B3",
		},
		"C": {
			BICUpdatedTime: 3,
			BICAmount:      0,
			PubKeys:        []string{"C.P1"},
			OutputID:       "C1",
		},
		"D": {
			BICUpdatedTime:                        1,
			BICAmount:                             0,
			PubKeys:                               []string{"D.P1"},
			OutputID:                              "D2",
			ValidatorStake:                        60,
			DelegationStake:                       5,
			FixedCost:                             8,
			StakeEndEpoch:                         18,
			LatestSupportedProtocolVersionAndHash: model.VersionAndHash{Version: 3, Hash: latestSupportedVersionHash2},
		},
	})

	//// Export and import the account ledger into new manager for the latest slot.
	{
		writer := &writerseeker.WriterSeeker{}

		err := ts.Instance.Export(writer, iotago.SlotIndex(3))
		require.NoError(t, err)

		ts.Instance = ts.initAccountLedger()
		err = ts.Instance.Import(writer.BytesReader())
		require.NoError(t, err)
		ts.Instance.SetLatestCommittedSlot(3)

		ts.AssertAccountLedgerUntilWithoutNewState(3)
	}

	// Export and import for pre-latest slot.
	{
		writer := &writerseeker.WriterSeeker{}

		err := ts.Instance.Export(writer, iotago.SlotIndex(2))
		require.NoError(t, err)

		ts.Instance = ts.initAccountLedger()
		err = ts.Instance.Import(writer.BytesReader())
		require.NoError(t, err)
		ts.Instance.SetLatestCommittedSlot(2)

		ts.AssertAccountLedgerUntilWithoutNewState(2)
	}
}
