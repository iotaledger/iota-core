package tpkg

import (
	"testing"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger/tpkg"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
)

type ExpectedAccountsLedgers map[iotago.SlotIndex]*AccountsExpectedData

type AccountsExpectedData struct {
	LatestCommittedSlotIndex iotago.SlotIndex
	AccountsLedger           map[iotago.AccountID]*accounts.AccountData
	AccountsDiffs            map[iotago.AccountID]*prunable.AccountDiff
}

type AccountsSlotBuildData struct {
	SlotIndex         iotago.SlotIndex
	Burns             map[iotago.AccountID]uint64
	SlotDiff          map[iotago.AccountID]*prunable.AccountDiff
	DestroyedAccounts *advancedset.AdvancedSet[iotago.AccountID]
}

func BlockFuncGen(t *testing.T, burnsPerSlot map[iotago.SlotIndex]map[iotago.AccountID]uint64) (func(iotago.BlockID) (*blocks.Block, bool), map[iotago.SlotIndex][]iotago.BlockID) {
	blockIDs := make(map[iotago.SlotIndex][]iotago.BlockID)
	blocksMap := make(map[iotago.BlockID]*blocks.Block)
	for slotIndex, burns := range burnsPerSlot {
		slotBlocksMap := RandomBlocksWithBurns(t, burns, slotIndex)
		blockIDs[slotIndex] = make([]iotago.BlockID, 0)
		for blockID, block := range slotBlocksMap {
			blockIDs[slotIndex] = append(blockIDs[slotIndex], blockID)
			blocksMap[blockID] = block
		}
	}

	return func(id iotago.BlockID) (*blocks.Block, bool) {
		block, ok := blocksMap[id]
		return block, ok
	}, blockIDs
}

func InitSlotDiff() func(index iotago.SlotIndex) *prunable.AccountDiffs {
	stores := make(map[iotago.SlotIndex]kvstore.KVStore)

	return func(index iotago.SlotIndex) *prunable.AccountDiffs {
		if _, exists := stores[index]; !exists {
			stores[index] = mapdb.NewMapDB()
		}

		return prunable.NewAccountDiffs(index, stores[index], tpkg.API())
	}
}

// Scenario defines Scenario for account ledger updates per slots and accounts.
type Scenario struct {
	Name        string
	Description string
	Scenario    map[iotago.SlotIndex]map[string]*SlotScenario
}

type SlotScenario struct {
	SlotActions  *AccountActions
	ExpectedData *AccountExpectedData
}

type AccountExpectedData struct {
	Amount              uint64
	PreviousUpdatedTime iotago.SlotIndex
	UpdatedTime         iotago.SlotIndex
	OutputID            string
	PrevOutputID        string
	PubKeys             []string
}

type AccountActions struct {
	totalAllotments uint64
	burns           []uint64
	destroyed       bool
	addedKeys       []string
	removedKeys     []string
}

type ScenarioFunc func() Scenario

func (s Scenario) InitScenario(t *testing.T) (
	map[iotago.SlotIndex]*AccountsSlotBuildData,
	ExpectedAccountsLedgers,
	func(iotago.BlockID) (*blocks.Block, bool),
	map[iotago.SlotIndex][]iotago.BlockID,
) {
	testSuite := NewTestSuite()
	slotBuildData := s.populateSlotBuildData(testSuite)
	expectedAccountLedger := s.populateExpectedAccountsLedger(testSuite)

	f, blks := s.blockFunc(t, testSuite)

	return slotBuildData, expectedAccountLedger, f, blks
}

func (s Scenario) populateSlotBuildData(testSuite *TestSuite) map[iotago.SlotIndex]*AccountsSlotBuildData {
	slotBuildData := make(map[iotago.SlotIndex]*AccountsSlotBuildData)

	for slotIndex, slotActions := range s.Scenario {
		slotBuildData[slotIndex] = &AccountsSlotBuildData{
			SlotIndex:         slotIndex,
			DestroyedAccounts: advancedset.New[iotago.AccountID](),
			Burns:             make(map[iotago.AccountID]uint64),
			SlotDiff:          make(map[iotago.AccountID]*prunable.AccountDiff),
		}
		// populate slot diff data based on Scenario
		for accAlias, slotScenario := range slotActions {
			actions := slotScenario.SlotActions
			expected := slotScenario.ExpectedData
			accountID := testSuite.AccountID(accAlias)
			if actions.burns != nil {
				slotBuildData[slotIndex].Burns[accountID] = lo.Sum[uint64](actions.burns...)
			}
			if actions.destroyed {
				slotBuildData[slotIndex].DestroyedAccounts.Add(accountID)
			}
			slotBuildData[slotIndex].SlotDiff[accountID] = &prunable.AccountDiff{
				Change:              int64(actions.totalAllotments), // manager takes AccountDiff only with allotments filled in when applyDiff is triggered
				PreviousUpdatedTime: expected.PreviousUpdatedTime,
				NewOutputID:         testSuite.OutputID(expected.OutputID),
				PreviousOutputID:    testSuite.OutputID(expected.PrevOutputID),
				PubKeysAdded:        testSuite.PubKeys(actions.addedKeys),
				PubKeysRemoved:      testSuite.PubKeys(actions.removedKeys),
			}
		}
	}

	return slotBuildData
}

func (s Scenario) populateExpectedAccountsLedger(testSuite *TestSuite) ExpectedAccountsLedgers {
	expectedData := make(ExpectedAccountsLedgers)

	for slotIndex, slotActions := range s.Scenario {

		expectedData[slotIndex] = &AccountsExpectedData{
			LatestCommittedSlotIndex: slotIndex,
			AccountsLedger:           make(map[iotago.AccountID]*accounts.AccountData),
			AccountsDiffs:            make(map[iotago.AccountID]*prunable.AccountDiff),
		}
		for accAlias, slotScenario := range slotActions {
			expected := slotScenario.ExpectedData
			actions := slotScenario.SlotActions
			accountID := testSuite.AccountID(accAlias)

			if slotScenario.SlotActions.destroyed {
				if slotIndex-1 > 0 {
					prevAccData := s.Scenario[slotIndex-1][accAlias].ExpectedData
					expectedData[slotIndex].AccountsDiffs[accountID] = &prunable.AccountDiff{
						Change:              -int64(prevAccData.Amount),
						PreviousUpdatedTime: prevAccData.UpdatedTime,
						NewOutputID:         testSuite.OutputID(prevAccData.OutputID),
						PreviousOutputID:    testSuite.OutputID(prevAccData.PrevOutputID),
						PubKeysRemoved:      testSuite.PubKeys(expected.PubKeys),
					}
				}
				expectedData[slotIndex].AccountsLedger[accountID] = nil

				continue
			}
			expectedData[slotIndex].AccountsLedger[accountID] = &accounts.AccountData{
				ID:       accountID,
				Credits:  accounts.NewBlockIssuanceCredits(int64(expected.Amount), expected.UpdatedTime),
				OutputID: testSuite.OutputID(expected.OutputID),
				PubKeys:  testSuite.PubKeysSet(expected.PubKeys),
			}
			expectedData[slotIndex].AccountsDiffs[accountID] = &prunable.AccountDiff{
				Change:              int64(expected.Amount),
				PreviousUpdatedTime: expected.PreviousUpdatedTime,
				NewOutputID:         testSuite.OutputID(expected.OutputID),
				PreviousOutputID:    testSuite.OutputID(expected.PrevOutputID),
				PubKeysAdded:        testSuite.PubKeys(actions.addedKeys),
				PubKeysRemoved:      testSuite.PubKeys(actions.removedKeys),
			}
		}
	}

	return expectedData
}

func (s Scenario) blockFunc(t *testing.T, testSuite *TestSuite) (func(iotago.BlockID) (*blocks.Block, bool), map[iotago.SlotIndex][]iotago.BlockID) {
	burns := make(map[iotago.SlotIndex]map[iotago.AccountID]uint64)
	for slotIndex, slotActions := range s.Scenario {
		burns[slotIndex] = make(map[iotago.AccountID]uint64)
		for accAlias, scenario := range slotActions {
			accountID := testSuite.AccountID(accAlias)
			for _, burned := range scenario.SlotActions.burns {
				burns[slotIndex][accountID] += burned
			}
		}
	}

	return BlockFuncGen(t, burns)
}

// Scenario1 is a simple Scenario with allotment and adding public keys.
var Scenario1 = &Scenario{
	Name:        "Scenario1",
	Description: "A simple Scenario with allotment and adding public keys",
	Scenario: map[iotago.SlotIndex]map[string]*SlotScenario{
		1: {
			"A": {
				&AccountActions{
					totalAllotments: 10,
					burns:           []uint64{5},
					addedKeys:       []string{"A1"},
				},
				&AccountExpectedData{
					Amount:              5,
					PubKeys:             []string{"A1"},
					PreviousUpdatedTime: 0,
					UpdatedTime:         1,
					OutputID:            "A1",
				},
			},
		},
		2: {
			"A": {
				&AccountActions{
					totalAllotments: 30,
					burns:           []uint64{15},
					addedKeys:       []string{"A2"},
				},
				&AccountExpectedData{
					Amount:              20,
					UpdatedTime:         2,
					PreviousUpdatedTime: 1,
					PubKeys:             []string{"A1", "A2"},
					OutputID:            "A2",
					PrevOutputID:        "A1",
				},
			},
		},
	},
}

// Scenario2 is a simple Scenario with allotment,  adding public keys with one empty slot in between. No account transition only abalance changed.
var Scenario2 = &Scenario{
	Name:        "Scenario2",
	Description: "A simple Scenario with allotment, adding public keys with one empty slot in between. No account transition only abalance changed",
	Scenario: map[iotago.SlotIndex]map[string]*SlotScenario{
		1: {
			"A": {
				&AccountActions{
					totalAllotments: 10,
					burns:           []uint64{5},
					addedKeys:       []string{"A1"},
				},
				&AccountExpectedData{
					Amount:      5,
					UpdatedTime: 1,
					OutputID:    "A1",
					PubKeys:     []string{"A1"},
				},
			},
		},
		2: {
			// no updates
		},
		3: {
			"A": { // only allotment so no transition and new outputID
				&AccountActions{
					totalAllotments: 30,
					burns:           []uint64{15},
				},
				&AccountExpectedData{
					Amount:              20,
					PreviousUpdatedTime: 1,
					UpdatedTime:         3,
					OutputID:            "A1",
					PubKeys:             []string{"A1"},
				},
			},
		},
	},
}

// Scenario3 is a Scenario where an account is destroyed with positive balance and public keys still present.
var Scenario3 = &Scenario{
	Name:        "Scenario3",
	Description: "A Scenario where an account is destroyed with positive balance and public keys still present",
	Scenario: map[iotago.SlotIndex]map[string]*SlotScenario{
		1: {
			"A": {
				&AccountActions{
					totalAllotments: 10,
					burns:           []uint64{5},
					addedKeys:       []string{"A1"},
				},
				&AccountExpectedData{
					Amount:      5,
					UpdatedTime: 1,
					OutputID:    "A1",
					PubKeys:     []string{"A1"},
				},
			},
		},
		2: {
			"A": {
				&AccountActions{
					destroyed: true,
				},
				&AccountExpectedData{},
			},
		},
	},
}

// Scenario4 is a Scenario where an account is destroyed after one
var Scenario4 = &Scenario{
	Name:        "Scenario4",
	Description: "A Scenario where an account is destroyed after one slot with allotment and public keys",
	Scenario: map[iotago.SlotIndex]map[string]*SlotScenario{
		1: {
			"A": {
				&AccountActions{
					totalAllotments: 10,
					burns:           []uint64{5},
					addedKeys:       []string{"A1"},
				},
				&AccountExpectedData{
					Amount:      5,
					UpdatedTime: 1,
					OutputID:    "A1",
					PubKeys:     []string{"A1"},
				},
			},
		},
		2: {
			// no changes
		},
		3: {
			"A": { // zero out the account data before removal
				&AccountActions{
					burns:       []uint64{5},
					removedKeys: []string{"A1"},
				},
				&AccountExpectedData{
					Amount:              0,
					UpdatedTime:         3,
					PreviousUpdatedTime: 1,
					OutputID:            "A2",
					PrevOutputID:        "A1",
				},
			},
		},
		4: {
			"A": {
				&AccountActions{
					destroyed: true,
				},
				&AccountExpectedData{},
			},
		},
	},
}

// Scenario5 is a Scenario where an account is destroyed but in the same slot there are allotment changestha should not be included in the expected vector.
var Scenario5 = &Scenario{
	Name:        "Scenario5",
	Description: "A Scenario where an account is destroyed but in the same slot there are allotment changes",
	Scenario: map[iotago.SlotIndex]map[string]*SlotScenario{
		1: {
			"A": {
				&AccountActions{
					totalAllotments: 5,
					burns:           []uint64{5},
					addedKeys:       []string{"A1", "A2"},
				},
				&AccountExpectedData{
					Amount:      10,
					UpdatedTime: 1,
					OutputID:    "A1",
					PubKeys:     []string{"A1", "A2"},
				},
			},
		},
		2: {
			"A": {
				&AccountActions{
					totalAllotments: 5,
					burns:           []uint64{5},
					addedKeys:       []string{"A3", "A4"},
					destroyed:       true,
				},
				&AccountExpectedData{},
			},
		},
	},
}

// TODO example where there are both diff changes in the same slot as acocount destruction,
//  we should not consider any balance changes from the destruction slot,
// we need to store the state from the prevSlot accountLedger only as those values will be used to rollback

//
//func Scenario5() (Scenario, *TestSuite) {
//	testSuite := NewTestSuite()
//	s := map[iotago.SlotIndex]*SlotActions{
//		1: { // zero balance at the end
//			testSuite.AccountID("A"): {
//				totalAllotments: 10,
//				burns:           []uint64{10},
//				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("A1")},
//			},
//			// only allotment
//			testSuite.AccountID("B"): {
//				totalAllotments: 100,
//				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("B1")},
//			},
//		},
//		2: { // account A destroyed with pubKeys present
//			testSuite.AccountID("A"): {
//				totalAllotments: 10,
//				burns:           []uint64{5, 5},
//				destroyed:       true,
//				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("A2")},
//				removedKeys:     []ed25519.PublicKey{testSuite.PublicKey("A1"), testSuite.PublicKey("A2")},
//			},
//			testSuite.AccountID("B"): {
//				addedKeys:   []ed25519.PublicKey{testSuite.PublicKey("B2")},
//				removedKeys: []ed25519.PublicKey{testSuite.PublicKey("B1")},
//			},
//			testSuite.AccountID("C"): {
//				totalAllotments: 15,
//				burns:           []uint64{15},
//				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("C1"), testSuite.PublicKey("C2")},
//			},
//			testSuite.AccountID("D"): {
//				totalAllotments: 20,
//				burns:           []uint64{10, 10},
//				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("D1"), testSuite.PublicKey("D2")},
//			},
//		},
//		3: { // Account B removes all data, but it's not destroyed yet
//			testSuite.AccountID("B"): {
//				burns:       []uint64{10},
//				removedKeys: []ed25519.PublicKey{testSuite.PublicKey("B2")},
//			},
//			testSuite.AccountID("C"): {
//				totalAllotments: 10,
//				burns:           []uint64{15}, // going negative
//				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("C3")},
//				removedKeys:     []ed25519.PublicKey{testSuite.PublicKey("C1")},
//			},
//			testSuite.AccountID("D"): {
//				burns:       []uint64{5, 5},
//				removedKeys: []ed25519.PublicKey{testSuite.PublicKey("D1")},
//			},
//			testSuite.AccountID("E"): {
//				totalAllotments: 15,
//				burns:           []uint64{5, 10},
//				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("E1")},
//			},
//			testSuite.AccountID("F"): {
//				totalAllotments: 10,
//				burns:           []uint64{5, 2},
//				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("F1")},
//			},
//		},
//		4: { // D is destroyed with still existing keys marked in slot diffs
//			testSuite.AccountID("D"): {
//				removedKeys: []ed25519.PublicKey{testSuite.PublicKey("D2")},
//				destroyed:   true,
//			},
//			testSuite.AccountID("E"): {
//				totalAllotments: 50,
//				burns:           []uint64{10, 10, 10},
//				// removing key added in the same slot
//				addedKeys:   []ed25519.PublicKey{testSuite.PublicKey("E2"), testSuite.PublicKey("E3"), testSuite.PublicKey("E4")},
//				removedKeys: []ed25519.PublicKey{testSuite.PublicKey("E2")},
//			},
//			testSuite.AccountID("F"): {
//				totalAllotments: 5,
//				burns:           []uint64{5},
//			},
//		},
//		5: {
//			testSuite.AccountID("B"): {
//				destroyed: true,
//			},
//			testSuite.AccountID("C"): {
//				totalAllotments: 5,
//			},
//			testSuite.AccountID("E"): {
//				burns:       []uint64{5, 5},
//				removedKeys: []ed25519.PublicKey{testSuite.PublicKey("E3")},
//			},
//			testSuite.AccountID("F"): {
//				totalAllotments: 10,
//				burns:           []uint64{10},
//			},
//			testSuite.AccountID("G"): {
//				burns:     []uint64{5},
//				addedKeys: []ed25519.PublicKey{testSuite.PublicKey("G1")},
//			},
//		},
//	}
//
//	return s, testSuite
//}
