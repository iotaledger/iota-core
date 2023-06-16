package tpkg

import (
	"testing"

	"github.com/iotaledger/hive.go/crypto/ed25519"
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

type ExpectedAccountsLedgers map[iotago.SlotIndex]*AccountsLedgerTestScenario

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
	stores := make(map[iotago.SlotIndex]kvstore.KVStore)
	slotDiffFunc = func(index iotago.SlotIndex) *prunable.AccountDiffs {
		if _, exists := stores[index]; !exists {
			stores[index] = mapdb.NewMapDB()
		}
		if slotDiff, exists := slotDiffs[index]; exists {
			return slotDiff
		}
		return prunable.NewAccountDiffs(index, stores[index], tpkg.API())
	}
	return slotDiffFunc, slotDiffs
}

var slotDiffFunc = func(iotago.SlotIndex) *prunable.AccountDiffs {
	return nil
}

// Scenario defines Scenario for account ledger updates per slots and accounts
type Scenario map[iotago.SlotIndex]*SlotActions

type ScenarioFunc func() (Scenario, *TestSuite)

func (s Scenario) updateTimeAndOutputs(testSuite *TestSuite) {
	prevActions := make(map[iotago.AccountID]*AccountActions)
	for index := iotago.SlotIndex(1); index <= iotago.SlotIndex(len(s)); index++ {
		for accID, action := range *s[index] {
			if action.removedKeys == nil {
				action.removedKeys = make([]ed25519.PublicKey, 0)
			}
			if action.addedKeys == nil {
				action.addedKeys = make([]ed25519.PublicKey, 0)
			}

			testSuite.updateActions(accID, index, action, prevActions[accID])

			prevActions[accID] = action
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
	rollingAccountLedger := make(map[iotago.AccountID]*accounts.AccountData)
	// need to go in order to create accountLedger cumulatively
	for slotIndex := iotago.SlotIndex(1); slotIndex <= iotago.SlotIndex(len(s)); slotIndex++ {
		slotActions := s[slotIndex]
		expected[slotIndex] = &AccountsLedgerTestScenario{
			LatestCommittedSlotIndex: 0,
			AccountsLedger:           make(map[iotago.AccountID]*accounts.AccountData),
			AccountsDiffs:            make(map[iotago.AccountID]*prunable.AccountDiff),
		}
		for accountID, actions := range *slotActions {
			change := int64(actions.totalAllotments)
			for _, burn := range actions.burns {
				change -= int64(burn)
			}
			accData := updateExpectedAccLedger(expected[slotIndex], rollingAccountLedger, accountID, actions, change)
			rollingAccountLedger[accountID] = accData.Clone()

			// populate diffs
			expected[slotIndex].AccountsDiffs[accountID] = &prunable.AccountDiff{
				Change:              change,
				PreviousUpdatedTime: actions.prevUpdatedTime,
				NewOutputID:         actions.outputID,
				PreviousOutputID:    actions.prevOutputID,
				PubKeysAdded:        lo.CopySlice(actions.addedKeys),
				PubKeysRemoved:      lo.CopySlice(actions.removedKeys),
			}

			if actions.destroyed {
				delete(expected[slotIndex].AccountsLedger, accountID)
				delete(rollingAccountLedger, accountID)
			}
		}
	}

	return expected
}

// todo destroyed current acc id should be set to empty
// todo make sure that output ID is updated only if an acocunt was transitioned, and not only on allotment
// when are output ids updated on the slot committment
func updateExpectedAccLedger(expectedAccountLedger *AccountsLedgerTestScenario, rollingLedger map[iotago.AccountID]*accounts.AccountData, accountID iotago.AccountID, actions *AccountActions, change int64) *accounts.AccountData {
	accData, exists := expectedAccountLedger.AccountsLedger[accountID]
	if !exists {
		// does this account existed in previous slots?
		prevData, existed := rollingLedger[accountID]
		if existed {
			accData = prevData.Clone()
		} else {
			accData = accounts.NewAccountData(
				accountID,
				accounts.NewBlockIssuanceCredits(int64(0), actions.updatedTime),
				actions.outputID,
			)
			rollingLedger[accountID] = accData.Clone()
		}
		expectedAccountLedger.AccountsLedger[accountID] = accData
	}
	accData.Credits.Update(change, actions.updatedTime)
	accData.OutputID = actions.outputID
	accData.AddPublicKeys(actions.addedKeys...)
	accData.RemovePublicKeys(actions.removedKeys...)
	return accData
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
		2: {
			testSuite.AccountID("A"): {
				totalAllotments: 30,
				burns:           []uint64{15},
				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("A2")},
			},
		},
	}
	return s, testSuite
}

// TODO example where there are both diff changes in the same slot as acocount destruction, we should not consider any balance changes from the destruction slot,
// we need to store the state from the prevSlot accountLedger only as those values will be used to rollback

// Scenario2 creates and destroys an account in the next slot.
func Scenario2() (Scenario, *TestSuite) {
	testSuite := NewTestSuite()
	s := map[iotago.SlotIndex]*SlotActions{
		1: {
			testSuite.AccountID("A"): {
				totalAllotments: 10,
				burns:           []uint64{5},
				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("A1")},
			},
		},
		2: {
			testSuite.AccountID("A"): {
				burns:       []uint64{5},
				removedKeys: []ed25519.PublicKey{testSuite.PublicKey("A1")},
				destroyed:   true,
			},
		},
	}
	return s, testSuite
}

func Scenario3() (Scenario, *TestSuite) {
	testSuite := NewTestSuite()
	s := map[iotago.SlotIndex]*SlotActions{
		1: {
			testSuite.AccountID("B"): {
				totalAllotments: 10,
				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("B1")},
			},
		},
		2: { // no action
		},
		3: { // action again
			testSuite.AccountID("B"): {
				totalAllotments: 2,
			},
		},
	}
	return s, testSuite
}

func Scenario4() (Scenario, *TestSuite) {
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
				totalAllotments: 100,
				addedKeys:       []ed25519.PublicKey{testSuite.PublicKey("B1")},
			},
		},
		2: { // account A destroyed with pubKeys present
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
		3: { // Account B removes all data, but it's not destroyed yet
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
		4: { // D is destroyed with still existing keys marked in slot diffs
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
