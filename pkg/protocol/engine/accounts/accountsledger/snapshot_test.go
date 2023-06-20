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
	})

	ts.AssertAccountLedgerUntil(1, map[string]*AccountState{
		"A": {
			UpdatedTime: 1,
			Amount:      5,
			PubKeys:     []string{"A.P1"},
			OutputID:    "A1",
		},
	})

	ts.ApplySlotActions(2, nil)
	ts.AssertAccountLedgerUntil(2, map[string]*AccountState{
		"A": {
			UpdatedTime: 1,
			Amount:      5,
			PubKeys:     []string{"A.P1"},
			OutputID:    "A1",
		},
	})

	ts.ApplySlotActions(3, map[string]*AccountActions{
		"A": { // zero out the account data before removal
			Burns:       []uint64{5},
			RemovedKeys: []string{"A.P1"},

			NewOutputID: "A2",
		}},
	)

	ts.AssertAccountLedgerUntil(3, map[string]*AccountState{
		"A": {
			Amount:      0,
			PubKeys:     []string{},
			OutputID:    "A2",
			UpdatedTime: 3,
		},
	})

	ts.ApplySlotActions(4, map[string]*AccountActions{
		"A": {
			Destroyed: true,
		},
	})

	ts.AssertAccountLedgerUntil(4, map[string]*AccountState{
		"A": {
			Destroyed: true,

			UpdatedTime: 4,
		},
	})

	// Export and import the account ledger into new manager.
	{

		writer := &writerseeker.WriterSeeker{}

		err := ts.Instance.Export(writer, iotago.SlotIndex(4))
		require.NoError(t, err)

		ts.Instance = ts.initAccountLedger()
		err = ts.Instance.Import(writer.BytesReader())
		require.NoError(t, err)
		ts.Instance.SetLatestCommittedSlot(4)

		ts.AssertAccountLedgerUntilWithoutNewState(4)
	}
}
