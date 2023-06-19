package accountsledger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/accountsledger/tpkg"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
	tpkg2 "github.com/iotaledger/iota.go/v4/tpkg"
)

var testScenarios = []*tpkg.Scenario{tpkg.Scenario1, tpkg.Scenario2, tpkg.Scenario3, tpkg.Scenario4, tpkg.Scenario5}

func TestManager_TrackBlock(t *testing.T) {
	burns := map[iotago.SlotIndex]map[iotago.AccountID]uint64{
		1: {
			tpkg2.RandAccountID(): utils.RandAmount(),
			tpkg2.RandAccountID(): utils.RandAmount(),
			tpkg2.RandAccountID(): utils.RandAmount(),
			tpkg2.RandAccountID(): utils.RandAmount(),
		},
	}
	blockFunc, blockIDs := tpkg.BlockFuncGen(t, burns)
	slotDiffFunc := func(iotago.SlotIndex) *prunable.AccountDiffs {
		return nil
	}
	accountsStore := mapdb.NewMapDB()
	params := tpkg.ProtocolParams()
	manager := New(blockFunc, slotDiffFunc, accountsStore, tpkg.API())
	manager.SetMaxCommittableAge(iotago.SlotIndex(params.MaxCommitableAge))

	for _, blockID := range blockIDs[1] {
		block, exist := blockFunc(blockID)
		require.True(t, exist)
		manager.TrackBlock(block)
	}
	managerBurns, err := manager.computeBlockBurnsForSlot(1)
	require.NoError(t, err)
	assert.EqualValues(t, burns[1], managerBurns)
}

func TestManager_CommitAccountTree(t *testing.T) {
	for _, test := range testScenarios {
		t.Run(test.Name, func(t *testing.T) {
			params := tpkg.ProtocolParams()
			slotDiffFunc := tpkg.InitSlotDiff()
			blockFunc := func(iotago.BlockID) (*blocks.Block, bool) { return nil, false }
			accountsStore := mapdb.NewMapDB()
			manager := New(blockFunc, slotDiffFunc, accountsStore, tpkg.API())
			manager.SetMaxCommittableAge(iotago.SlotIndex(params.MaxCommitableAge))

			scenarioBuildData, scenarioExpected, _, _ := test.InitScenario(t)

			for index := iotago.SlotIndex(1); index <= iotago.SlotIndex(len(scenarioBuildData)); index++ {
				// apply burns to the slot diff
				manager.updateSlotDiffWithBurns(scenarioBuildData[index].Burns, scenarioBuildData[index].SlotDiff)

				manager.commitAccountTree(index, scenarioBuildData[index].SlotDiff, scenarioBuildData[index].DestroyedAccounts)

				for accountID, expectedData := range scenarioExpected[index].AccountsLedger {
					actualData, exists, err2 := manager.Account(accountID)
					assert.NoError(t, err2)
					assert.True(t, exists)
					assert.Equal(t, expectedData, actualData)
				}
			}
		})
	}
}

func TestManager_Account(t *testing.T) {
	for _, test := range testScenarios {
		t.Run(test.Name, func(t *testing.T) {
			params := tpkg.ProtocolParams()
			// account vector is now on the last slot of the scenario
			scenarioBuildData, scenarioExpected, blockFunc, burnedBlocks := test.InitScenario(t)
			manager := InitAccountLedger(t, blockFunc, params.MaxCommitableAge, scenarioBuildData, burnedBlocks)
			// get the value from the past slots, testing the rollback
			for index := iotago.SlotIndex(1); index <= iotago.SlotIndex(len(scenarioBuildData)); index++ {
				for accountID, accData := range scenarioExpected[index].AccountsLedger {
					actualData, exists, err := manager.Account(accountID, index)
					assert.NoError(t, err)
					assert.True(t, exists)
					tpkg.EqualAccountData(t, accData, actualData)
				}
				// check if all destroyed accounts are not found
				err := scenarioBuildData[index].DestroyedAccounts.ForEach(func(accountID iotago.AccountID) error {
					_, exists, err := manager.Account(accountID, index)
					assert.NoError(t, err)
					assert.False(t, exists)

					return nil
				})
				require.NoError(t, err)
			}
		})
	}
}

func TestManager_CommitSlot(t *testing.T) {
	for _, test := range testScenarios {
		t.Run(test.Name, func(t *testing.T) {
			scenarioBuildData, scenarioExpected, blockFunc, burnedBlocks := test.InitScenario(t)
			params := tpkg.ProtocolParams()

			slotDiffFunc := tpkg.InitSlotDiff()
			accountsStore := mapdb.NewMapDB()

			manager := New(blockFunc, slotDiffFunc, accountsStore, tpkg.API())
			manager.SetMaxCommittableAge(iotago.SlotIndex(params.MaxCommitableAge))

			for index := iotago.SlotIndex(1); index <= iotago.SlotIndex(len(scenarioBuildData)); index++ {
				for _, burningBlock := range burnedBlocks[index] {
					block, exists := blockFunc(burningBlock)
					assert.True(t, exists)
					manager.TrackBlock(block)
				}
				slotBuildData := scenarioBuildData[index]

				err := manager.ApplyDiff(index, slotBuildData.SlotDiff, slotBuildData.DestroyedAccounts)
				require.NoError(t, err)
				expectedData := scenarioExpected[index]
				AssertAccountManagerSlotState(t, manager, expectedData)
			}
		})
	}

}

func AssertAccountManagerSlotState(t *testing.T, manager *Manager, expectedData *tpkg.AccountsExpectedData) {
	// assert accounts vector is updated correctly
	for accID, expectedAccData := range expectedData.AccountsLedger {
		actualData, exists, err2 := manager.Account(accID)
		assert.NoError(t, err2)
		assert.True(t, exists)
		tpkg.EqualAccountData(t, expectedAccData, actualData)
	}
	for accID, expectedDiff := range expectedData.AccountsDiffs {
		actualAccDiff, _, err2 := manager.LoadSlotDiff(expectedData.LatestCommittedSlotIndex, accID)
		require.NoError(t, err2)
		assert.Equal(t, expectedDiff, actualAccDiff)
	}
}
