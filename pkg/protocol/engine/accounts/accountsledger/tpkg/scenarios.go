package tpkg

import (
	"testing"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger/tpkg"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
)

type AccountsLedgerTestScenario struct {
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

type SlotActions map[iotago.AccountID]*AccountActions

type ExpectedAccountsLedgers map[iotago.SlotIndex]AccountsLedgerTestScenario

type AccountActions struct {
	burns           []uint64
	totalAllotments uint64
	destroyed       bool
	addedKeys       []ed25519.PublicKey
	removedKeys     []ed25519.PublicKey
	updatedTime     iotago.SlotIndex
	prevUpdatedTime iotago.SlotIndex
	outputID        iotago.OutputID
	prevOutputID    iotago.OutputID
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

func InitSlotDiff() (func(index iotago.SlotIndex) *prunable.AccountDiffs, map[iotago.SlotIndex]*prunable.AccountDiffs) {
	slotDiffs := make(map[iotago.SlotIndex]*prunable.AccountDiffs)
	store := mapdb.NewMapDB()
	slotDiffFunc = func(index iotago.SlotIndex) *prunable.AccountDiffs {
		if slotDiff, exists := slotDiffs[index]; exists {
			return slotDiff
		}
		return prunable.NewAccountDiffs(index, store, tpkg.API())
	}
	return slotDiffFunc, slotDiffs
}

var slotDiffFunc = func(iotago.SlotIndex) *prunable.AccountDiffs {
	return nil
}

// Scenario defines Scenario for accout ledger updates per slots and accounts
type Scenario map[iotago.SlotIndex]*SlotActions

type ScenarioFunc func() (Scenario, *TestSuite)

func (s Scenario) updateTimeAndOutputs(testSuite *TestSuite) {
	for index := iotago.SlotIndex(1); index <= iotago.SlotIndex(len(s)); index++ {
		for accID, actions := range *s[index] {
			if actions.removedKeys == nil {
				actions.removedKeys = make([]ed25519.PublicKey, 0)
			}
			if actions.addedKeys == nil {
				actions.addedKeys = make([]ed25519.PublicKey, 0)
			}
			testSuite.updateActions(accID, index, actions)
		}
	}
}

func (s Scenario) populateSlotBuildData() map[iotago.SlotIndex]*AccountsSlotBuildData {
	slotBuildData := make(map[iotago.SlotIndex]*AccountsSlotBuildData)

	for slotIndex, slotActions := range s {
		slotBuildData[slotIndex] = &AccountsSlotBuildData{
			SlotIndex:         slotIndex,
			DestroyedAccounts: advancedset.New[iotago.AccountID](),
			Burns:             make(map[iotago.AccountID]uint64),
			SlotDiff:          make(map[iotago.AccountID]*prunable.AccountDiff),
		}
		// populate slot diff data based on Scenario
		for accountID, actions := range *slotActions {
			if actions.burns != nil {
				slotBuildData[slotIndex].Burns[accountID] = sumBurns(actions.burns)
			}
			if actions.destroyed {
				slotBuildData[slotIndex].DestroyedAccounts.Add(accountID)
			}

			slotBuildData[slotIndex].SlotDiff[accountID] = &prunable.AccountDiff{
				Change:              int64(actions.totalAllotments), // manager takes AccountDiff only with allotments filled in when applyDiff is triggered
				NewOutputID:         actions.outputID,
				PreviousOutputID:    actions.prevOutputID,
				PreviousUpdatedTime: actions.prevUpdatedTime,
				PubKeysAdded:        lo.CopySlice(actions.addedKeys),
				PubKeysRemoved:      lo.CopySlice(actions.removedKeys),
			}
		}
	}
	return slotBuildData
}

func (s Scenario) populateExpectedAccountsLedger() ExpectedAccountsLedgers {
	expected := make(ExpectedAccountsLedgers)
	for slotIndex, slotActions := range s {
		expected[slotIndex] = AccountsLedgerTestScenario{
			LatestCommittedSlotIndex: slotIndex,
			AccountsLedger:           make(map[iotago.AccountID]*accounts.AccountData),
			AccountsDiffs:            make(map[iotago.AccountID]*prunable.AccountDiff),
		}
		for accountID, actions := range *slotActions {
			if actions.destroyed {
				delete(expected[slotIndex].AccountsLedger, accountID)
			}
			accData, exists := expected[slotIndex].AccountsLedger[accountID]
			change := int64(actions.totalAllotments)
			for _, burn := range actions.burns {
				change -= int64(burn)
			}
			if !exists {
				accData = accounts.NewAccountData(
					accountID,
					accounts.NewBlockIssuanceCredits(int64(0), actions.updatedTime),
					actions.outputID,
				)
			}
			accData.Credits.Update(change)
			accData.AddPublicKeys(actions.addedKeys...)
			accData.RemovePublicKeys(actions.removedKeys...)

			expected[slotIndex].AccountsLedger[accountID] = accData

			// populate diffs
			expected[slotIndex].AccountsDiffs[accountID] = &prunable.AccountDiff{
				Change:              change,
				PreviousUpdatedTime: actions.prevUpdatedTime,
				NewOutputID:         actions.outputID,
				PreviousOutputID:    actions.prevOutputID,
				PubKeysAdded:        lo.CopySlice(actions.addedKeys),
				PubKeysRemoved:      lo.CopySlice(actions.removedKeys),
			}
		}
	}

	return expected
}

func (s Scenario) blockFunc(t *testing.T) (func(iotago.BlockID) (*blocks.Block, bool), map[iotago.SlotIndex][]iotago.BlockID) {
	burns := make(map[iotago.SlotIndex]map[iotago.AccountID]uint64)
	for slotIndex, slotActions := range s {
		burns[slotIndex] = make(map[iotago.AccountID]uint64)
		for accountID, actions := range *slotActions {
			for _, burned := range actions.burns {
				burns[slotIndex][accountID] += burned
			}
		}
	}
	return BlockFuncGen(t, burns)
}

func Scenario1() (Scenario, *TestSuite) {
	testSuite := NewTestSuite()
	s := map[iotago.SlotIndex]*SlotActions{
		1: {
			testSuite.AccountID("A"): {
				totalAllotments: 10,
				burns:           []uint64{5},
				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("A1")},
			},
		},
	}
	return s, testSuite
}

func Scenario2() (Scenario, *TestSuite) {
	testSuite := NewTestSuite()
	s := map[iotago.SlotIndex]*SlotActions{
		1: { // zero balance at the end
			testSuite.AccountID("A"): {
				totalAllotments: 10,
				burns:           []uint64{10},
				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("A1")},
			},
			// only allotment
			testSuite.AccountID("B"): {
				totalAllotments: 10,
				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("B1")},
			},
		},
		2: { // account A destroyed with pubkeys present
			testSuite.AccountID("A"): {
				totalAllotments: 10,
				burns:           []uint64{5, 5},
				destroyed:       true,
				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("A2")},
				removedKeys:     []ed25519.PublicKey{testSuite.PublicKey("A1"), testSuite.PublicKey("A2")},
			},
			testSuite.AccountID("B"): {
				addedKeys:   []ed25519.PublicKey{testSuite.PublicKey("B2")},
				removedKeys: []ed25519.PublicKey{testSuite.PublicKey("B1")},
			},
			testSuite.AccountID("C"): {
				totalAllotments: 15,
				burns:           []uint64{15},
				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("C1"), testSuite.PublicKey("C2")},
			},
			testSuite.AccountID("D"): {
				totalAllotments: 20,
				burns:           []uint64{10, 10},
				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("D1"), testSuite.PublicKey("D2")},
			},
		},
		3: { // Account B removes all data but it's not destroyed yet
			testSuite.AccountID("B"): {
				burns:       []uint64{10},
				removedKeys: []ed25519.PublicKey{testSuite.PublicKey("B2")},
			},
			testSuite.AccountID("C"): {
				totalAllotments: 10,
				burns:           []uint64{15}, // going negative
				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("C3")},
				removedKeys:     []ed25519.PublicKey{testSuite.PublicKey("C1")},
			},
			testSuite.AccountID("D"): {
				burns:       []uint64{5, 5},
				removedKeys: []ed25519.PublicKey{testSuite.PublicKey("D1")},
			},
			testSuite.AccountID("E"): {
				totalAllotments: 15,
				burns:           []uint64{5, 10},
				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("E1")},
			},
			testSuite.AccountID("F"): {
				totalAllotments: 10,
				burns:           []uint64{5, 2},
				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("F1")},
			},
		},
		4: {
			testSuite.AccountID("D"): {
				removedKeys: []ed25519.PublicKey{testSuite.PublicKey("D2")},
				destroyed:   true,
			},
			testSuite.AccountID("E"): {
				totalAllotments: 50,
				burns:           []uint64{10, 10, 10},
				// removing key added in the same slot
				addedKeys:   []ed25519.PublicKey{testSuite.PublicKey("E2"), testSuite.PublicKey("E3"), testSuite.PublicKey("E4")},
				removedKeys: []ed25519.PublicKey{testSuite.PublicKey("E2")},
			},
			testSuite.AccountID("F"): {
				totalAllotments: 5,
				burns:           []uint64{5},
			},
		},
		5: {
			testSuite.AccountID("B"): {
				destroyed: true,
			},
			testSuite.AccountID("C"): {
				totalAllotments: 5,
			},
			testSuite.AccountID("E"): {
				burns:       []uint64{5, 5},
				removedKeys: []ed25519.PublicKey{testSuite.PublicKey("E3")},
			},
			testSuite.AccountID("F"): {
				totalAllotments: 10,
				burns:           []uint64{10},
			},
			testSuite.AccountID("G"): {
				burns:     []uint64{5},
				addedKeys: []ed25519.PublicKey{testSuite.PublicKey("G1")},
			},
		},
	}
	return s, testSuite
}

func InitScenario(t *testing.T, scenarioFunc ScenarioFunc) (
	map[iotago.SlotIndex]*AccountsSlotBuildData,
	ExpectedAccountsLedgers,
	func(iotago.BlockID) (*blocks.Block, bool),
	map[iotago.SlotIndex][]iotago.BlockID,
) {

	s, testSuite := scenarioFunc()
	s.updateTimeAndOutputs(testSuite)

	slotBuildData := s.populateSlotBuildData()
	expectedAccountLedger := s.populateExpectedAccountsLedger()

	f, blks := s.blockFunc(t)

	return slotBuildData, expectedAccountLedger, f, blks
}

func sumBurns(burns []uint64) uint64 {
	sum := uint64(0)
	for _, b := range burns {
		sum += b
	}

	return sum
}
