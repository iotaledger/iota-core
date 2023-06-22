package accountsledger_test

import (
	"testing"
)

func TestManager_Scenario1(t *testing.T) {
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
			BICUpdatedTime: 1,
			BICAmount:      5,
			PubKeys:        []string{"A.P1"},
			OutputID:       "A1",
		},
	})

	ts.ApplySlotActions(2, map[string]*AccountActions{
		"A": {
			TotalAllotments: 30,
			Burns:           []uint64{15},
			AddedKeys:       []string{"A.P2"},

			NewOutputID: "A2",
		}},
	)

	ts.AssertAccountLedgerUntil(2, map[string]*AccountState{
		"A": {
			BICAmount:      20,
			PubKeys:        []string{"A.P1", "A.P2"},
			OutputID:       "A2",
			BICUpdatedTime: 2,
		},
	})
}

func TestManager_Scenario2(t *testing.T) {
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
			BICUpdatedTime: 1,
			BICAmount:      5,
			PubKeys:        []string{"A.P1"},
			OutputID:       "A1",
		},
	})

	ts.ApplySlotActions(2, nil)
	ts.AssertAccountLedgerUntil(2, map[string]*AccountState{
		"A": {
			BICUpdatedTime: 1,
			BICAmount:      5,
			PubKeys:        []string{"A.P1"},
			OutputID:       "A1",
		},
	})

	ts.ApplySlotActions(3, map[string]*AccountActions{
		"A": {
			TotalAllotments: 30,
			Burns:           []uint64{15},
			AddedKeys:       []string{"A.P2"},

			NewOutputID: "A2",
		}},
	)

	ts.AssertAccountLedgerUntil(3, map[string]*AccountState{
		"A": {
			BICAmount:      20,
			PubKeys:        []string{"A.P1", "A.P2"},
			OutputID:       "A2",
			BICUpdatedTime: 3,
		},
	})
}

func TestManager_Scenario3(t *testing.T) {
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
			BICUpdatedTime: 1,
			BICAmount:      5,
			PubKeys:        []string{"A.P1"},
			OutputID:       "A1",
		},
	})

	ts.ApplySlotActions(2, map[string]*AccountActions{
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
			BICUpdatedTime: 1,
			BICAmount:      5,
			PubKeys:        []string{"A.P1"},
			OutputID:       "A1",
		},
	})

	ts.ApplySlotActions(2, nil)
	ts.AssertAccountLedgerUntil(2, map[string]*AccountState{
		"A": {
			BICUpdatedTime: 1,
			BICAmount:      5,
			PubKeys:        []string{"A.P1"},
			OutputID:       "A1",
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
			BICAmount:      0,
			PubKeys:        []string{},
			OutputID:       "A2",
			BICUpdatedTime: 3,
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

			BICUpdatedTime: 4,
		},
	})
}

func TestManager_Scenario5(t *testing.T) {
	ts := NewTestSuite(t)
	ts.ApplySlotActions(1, map[string]*AccountActions{
		"A": {
			TotalAllotments: 5,
			Burns:           []uint64{5},
			AddedKeys:       []string{"A1", "A2"},

			NewOutputID: "A1",
		},
	})

	ts.AssertAccountLedgerUntil(1, map[string]*AccountState{
		"A": {
			BICUpdatedTime: 1,
			BICAmount:      0,
			PubKeys:        []string{"A1", "A2"},
			OutputID:       "A1",
		},
	})

	ts.ApplySlotActions(2, map[string]*AccountActions{
		"A": {
			TotalAllotments: 5,
			Burns:           []uint64{5},
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
