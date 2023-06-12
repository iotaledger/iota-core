package tpkg

import (
	"testing"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	MaxCommittedAge = 2
)

type AccountsLedgerTestScenario struct {
	latestCommittedSlotIndex iotago.SlotIndex
	accountsLedger           map[iotago.AccountID]*accounts.AccountData
	accountsDiffs            map[iotago.AccountID]*prunable.AccountDiff
}

type AccountsSlotBuildData struct {
	slotIndex         iotago.SlotIndex
	burns             map[iotago.AccountID]uint64
	slotDiff          map[iotago.AccountID]*prunable.AccountDiff
	destroyedAccounts *advancedset.AdvancedSet[iotago.AccountID]
}

type SlotActions map[iotago.AccountID]*AccountActions

type ExpectedAccountsLedgers map[iotago.SlotIndex]map[iotago.AccountID]*accounts.AccountData

type AccountActions struct {
	burns           []uint64
	totalAllotments uint64
	destroyed       bool
	addedKeys       []ed25519.PublicKey
	removedKeys     []ed25519.PublicKey
}

func BlockFuncGen(t *testing.T, burnsPerSlot map[iotago.SlotIndex]map[iotago.AccountID]uint64) (func(iotago.BlockID) (*blocks.Block, bool), []iotago.BlockID) {
	blockIDs := make([]iotago.BlockID, 0)
	blocksMap := make(map[iotago.BlockID]*blocks.Block)
	for slotIndex, burns := range burnsPerSlot {
		slotBlocksMap := RandomBlocksWithBurns(t, burns, slotIndex)
		for blockID, block := range slotBlocksMap {
			blockIDs = append(blockIDs, blockID)
			blocksMap[blockID] = block
		}
	}

	return func(id iotago.BlockID) (*blocks.Block, bool) {
		block, ok := blocksMap[id]
		return block, ok
	}, blockIDs
}

var slotDiffFunc = func(iotago.SlotIndex) *prunable.AccountDiffs {
	return nil
}

// TODO add previous updsted time to scenario
// TODO add outputs to scenario

type scenario map[iotago.SlotIndex]SlotActions

func scenario1() (s scenario) {
	testSuite := NewTestSuite()
	return map[iotago.SlotIndex]SlotActions{
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
}

func (s *scenario) populateExpectedAccountsLedger() ExpectedAccountsLedgers {
	expected := make(ExpectedAccountsLedgers)
	for slotIndex, slotActions := range *s {
		expected[slotIndex] = make(map[iotago.AccountID]*accounts.AccountData)
		for accountID, actions := range slotActions {
			if actions.destroyed {
				continue
			}
			currentBalance := int64(0) // TODO calculate balance and time
			currentkeys := make([]ed25519.PublicKey, 0)
			expected[slotIndex][accountID] = accounts.NewAccountData(
				API(),
				accountID,
				accounts.NewBlockIssuanceCredits(currentBalance, 0),
				utils.RandOutputID(1), // TODO update the scenario to include outputIDs
				currentkeys...,
			)
		}
	}

	return expected
}

func AccountLedgerScenario1() (map[iotago.SlotIndex]*AccountsSlotBuildData, map[iotago.SlotIndex]*AccountsLedgerTestScenario) {
	s := scenario1()

	slotBuildData := make(map[iotago.SlotIndex]*AccountsSlotBuildData)

	for slotIndex, slotActions := range s {
		slotBuildData[slotIndex] = &AccountsSlotBuildData{
			slotIndex:         slotIndex,
			destroyedAccounts: advancedset.New[iotago.AccountID](),
			burns:             make(map[iotago.AccountID]uint64),
			slotDiff:          make(map[iotago.AccountID]*prunable.AccountDiff),
		}
		// populate slot diff data based on scenario
		for accountID, actions := range slotActions {
			if actions.burns != nil {
				slotBuildData[slotIndex].burns[accountID] = sumBurns(actions.burns)
			}
			if actions.destroyed {
				slotBuildData[slotIndex].destroyedAccounts.Add(accountID)
			}
			slotBuildData[slotIndex].slotDiff[accountID] = &prunable.AccountDiff{
				Change:         int64(actions.totalAllotments),
				PubKeysAdded:   actions.addedKeys[:], // TODO does it created a copy
				PubKeysRemoved: actions.removedKeys[:],
			}
		}
	}

	// populate target accounts diffs based on scenario
	expectedAccountLedger := make(map[iotago.SlotIndex]*AccountsLedgerTestScenario)
	for slotIndex, slotActions := range s {
		expectedAccountLedger[slotIndex] = &AccountsLedgerTestScenario{
			latestCommittedSlotIndex: slotIndex,
			accountsDiffs:            make(map[iotago.AccountID]*prunable.AccountDiff),
			accountsLedger:           make(map[iotago.AccountID]*accounts.AccountData),
		}
		for accountID, actions := range slotActions {
			expectedAccountLedger[slotIndex].accountsDiffs[accountID] = &prunable.AccountDiff{
				Change:         int64(actions.totalAllotments) - int64(sumBurns(actions.burns)),
				PubKeysAdded:   actions.addedKeys[:],
				PubKeysRemoved: actions.removedKeys[:],
			}
		}
	}
	// TODO populate expected vector value
	return nil, nil
}

func sumBurns(burns []uint64) uint64 {
	sum := uint64(0)
	for _, b := range burns {
		sum += b
	}
	return sum
}
