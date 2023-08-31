package accountsledger_test

import (
	"testing"
)

func TestManager_Scenario1(t *testing.T) {
	ts := NewTestSuite(t)

	ts.ApplySlotActions(1, 5, map[string]*AccountActions{
		"A": {
			TotalAllotments: 10,
			NumBlocks:       1,
			AddedKeys:       []string{"A.P1"},

			NewOutputID: "A1",
		},
	})

	ts.AssertAccountLedgerUntil(1, map[string]*AccountState{
		"A": {
			BICUpdatedTime:  1,
			BICAmount:       5,
			BlockIssuerKeys: []string{"A.P1"},
			OutputID:        "A1",
		},
	})

	ts.ApplySlotActions(2, 15, map[string]*AccountActions{
		"A": {
			TotalAllotments: 30,
			NumBlocks:       1,
			AddedKeys:       []string{"A.P2"},

			NewOutputID: "A2",
		}},
	)

	ts.AssertAccountLedgerUntil(2, map[string]*AccountState{
		"A": {
			BICAmount:       20,
			BlockIssuerKeys: []string{"A.P1", "A.P2"},
			OutputID:        "A2",
			BICUpdatedTime:  2,
		},
	})
}

func TestManager_Scenario2(t *testing.T) {
	ts := NewTestSuite(t)

	ts.ApplySlotActions(1, 5, map[string]*AccountActions{
		"A": {
			TotalAllotments: 10,
			NumBlocks:       1,
			AddedKeys:       []string{"A.P1"},

			NewOutputID: "A1",
		},
	})

	ts.AssertAccountLedgerUntil(1, map[string]*AccountState{
		"A": {
			BICUpdatedTime:  1,
			BICAmount:       5,
			BlockIssuerKeys: []string{"A.P1"},
			OutputID:        "A1",
		},
	})

	ts.ApplySlotActions(2, 0, nil)
	ts.AssertAccountLedgerUntil(2, map[string]*AccountState{
		"A": {
			BICUpdatedTime:  1,
			BICAmount:       5,
			BlockIssuerKeys: []string{"A.P1"},
			OutputID:        "A1",
		},
	})

	ts.ApplySlotActions(3, 15, map[string]*AccountActions{
		"A": {
			TotalAllotments: 30,
			NumBlocks:       1,
			AddedKeys:       []string{"A.P2"},

			NewOutputID: "A2",
		}},
	)

	ts.AssertAccountLedgerUntil(3, map[string]*AccountState{
		"A": {
			BICAmount:       20,
			BlockIssuerKeys: []string{"A.P1", "A.P2"},
			OutputID:        "A2",
			BICUpdatedTime:  3,
		},
	})
}

func TestManager_Scenario3(t *testing.T) {
	ts := NewTestSuite(t)
	ts.ApplySlotActions(1, 5, map[string]*AccountActions{
		"A": {
			TotalAllotments: 10,
			NumBlocks:       1,
			AddedKeys:       []string{"A.P1"},

			NewOutputID: "A1",
		},
	})

	ts.AssertAccountLedgerUntil(1, map[string]*AccountState{
		"A": {
			BICUpdatedTime:  1,
			BICAmount:       5,
			BlockIssuerKeys: []string{"A.P1"},
			OutputID:        "A1",
		},
	})

	ts.ApplySlotActions(2, 0, map[string]*AccountActions{
		"A": {
			Destroyed: true,
		},
	})

	ts.AssertAccountLedgerUntil(2, map[string]*AccountState{
		"A": {
			Destroyed:      true,
			BICUpdatedTime: 2,
		},
	})
}

func TestManager_Scenario4(t *testing.T) {
	ts := NewTestSuite(t)

	ts.ApplySlotActions(1, 5, map[string]*AccountActions{
		"A": {
			TotalAllotments: 10,
			NumBlocks:       1,
			AddedKeys:       []string{"A.P1"},

			NewOutputID: "A1",
		},
	})

	ts.AssertAccountLedgerUntil(1, map[string]*AccountState{
		"A": {
			BICUpdatedTime:  1,
			BICAmount:       5,
			BlockIssuerKeys: []string{"A.P1"},
			OutputID:        "A1",
		},
	})

	ts.ApplySlotActions(2, 0, nil)
	ts.AssertAccountLedgerUntil(2, map[string]*AccountState{
		"A": {
			BICUpdatedTime:  1,
			BICAmount:       5,
			BlockIssuerKeys: []string{"A.P1"},
			OutputID:        "A1",
		},
	})

	ts.ApplySlotActions(3, 5, map[string]*AccountActions{
		"A": { // zero out the account data before removal
			NumBlocks:   1,
			RemovedKeys: []string{"A.P1"},

			NewOutputID: "A2",
		}},
	)

	ts.AssertAccountLedgerUntil(3, map[string]*AccountState{
		"A": {
			BICAmount:       0,
			BlockIssuerKeys: []string{},
			OutputID:        "A2",
			BICUpdatedTime:  3,
		},
	})

	ts.ApplySlotActions(4, 0, map[string]*AccountActions{
		"A": {
			Destroyed: true,
		},
	})

	ts.AssertAccountLedgerUntil(4, map[string]*AccountState{
		"A": {
			Destroyed: true,

			BICUpdatedTime: 4,
		},
	})
}

func TestManager_Scenario5(t *testing.T) {
	ts := NewTestSuite(t)
	ts.ApplySlotActions(1, 5, map[string]*AccountActions{
		"A": {
			TotalAllotments: 5,
			NumBlocks:       1,
			AddedKeys:       []string{"A1", "A2"},

			NewOutputID: "A1",
		},
	})

	ts.AssertAccountLedgerUntil(1, map[string]*AccountState{
		"A": {
			BICUpdatedTime:  1,
			BICAmount:       0,
			BlockIssuerKeys: []string{"A1", "A2"},
			OutputID:        "A1",
		},
	})

	ts.ApplySlotActions(2, 5, map[string]*AccountActions{
		"A": {
			TotalAllotments: 5,
			NumBlocks:       1,
			AddedKeys:       []string{"A3", "A4"},
			Destroyed:       true,
		},
	})
	ts.AssertAccountLedgerUntil(2, map[string]*AccountState{
		"A": {
			Destroyed: true,

			BICUpdatedTime: 2,
		},
	})
}
