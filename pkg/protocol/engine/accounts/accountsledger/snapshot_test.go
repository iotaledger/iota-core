package accountsledger_test

import (
	"testing"

	"github.com/orcaman/writerseeker"
	"github.com/stretchr/testify/require"

	iotago "github.com/iotaledger/iota.go/v4"
)

func TestManager_Import_Export(t *testing.T) {
	ts := NewTestSuite(t)

	ts.ApplySlotActions(1, map[string]*AccountActions{
		"A": {
			TotalAllotments: 10,
			Burns:           []uint64{5},
			AddedKeys:       []string{"A.P1"},

			NewOutputID: "A1",
		},
		"B": {
			TotalAllotments: 20,
			Burns:           []uint64{10},
			AddedKeys:       []string{"B.P1", "B.P2"},

			NewOutputID: "B1",
		},
	})

	ts.AssertAccountLedgerUntil(1, map[string]*AccountState{
		"A": {
			UpdatedTime: 1,
			Amount:      5,
			PubKeys:     []string{"A.P1"},
			OutputID:    "A1",
		},
		"B": {
			UpdatedTime: 1,
			Amount:      10,
			PubKeys:     []string{"B.P1", "B.P2"},
			OutputID:    "B1",
		},
	})

	ts.ApplySlotActions(2, map[string]*AccountActions{
		"A": { // zero out the account data before removal
			Burns:       []uint64{5},
			RemovedKeys: []string{"A.P1"},

			NewOutputID: "A2",
		},
		"B": {
			TotalAllotments: 5,
			Burns:           []uint64{2},
			RemovedKeys:     []string{"B.P1"},

			NewOutputID: "B2",
		},
	})

	ts.AssertAccountLedgerUntil(2, map[string]*AccountState{
		"A": {
			Amount:      0,
			PubKeys:     []string{},
			OutputID:    "A2",
			UpdatedTime: 2,
		},
		"B": {
			UpdatedTime: 2,
			Amount:      13,
			PubKeys:     []string{"B.P2"},
			OutputID:    "B2",
		},
	})

	ts.ApplySlotActions(3, map[string]*AccountActions{
		"A": {
			Destroyed: true,
		},
		"B": {
			TotalAllotments: 10,
			Burns:           []uint64{5},
			AddedKeys:       []string{"B.P3"},

			NewOutputID: "B3",
		},
		"C": {
			TotalAllotments: 10,
			Burns:           []uint64{10},
			AddedKeys:       []string{"C.P1"},

			NewOutputID: "C1",
		},
	})

	ts.AssertAccountLedgerUntil(3, map[string]*AccountState{
		"A": {
			Destroyed: true,

			UpdatedTime: 3,
		},
		"B": {
			Amount:      18,
			PubKeys:     []string{"B.P2", "B.P3"},
			OutputID:    "B3",
			UpdatedTime: 3,
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
