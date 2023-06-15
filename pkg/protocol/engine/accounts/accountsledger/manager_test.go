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
	manager := New(blockFunc, slotDiffFunc, accountsStore, tpkg.API(), params.MaxCommitableAge)

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
	params := tpkg.ProtocolParams()
	slotDiffFunc, _ := tpkg.InitSlotDiff()
	blockFunc := func(iotago.BlockID) (*blocks.Block, bool) { return nil, false }
	accountsStore := mapdb.NewMapDB()
	manager := New(blockFunc, slotDiffFunc, accountsStore, tpkg.API(), params.MaxCommitableAge)

	scenarioBuildData, scenarioExpected, _, _ := tpkg.InitScenario(t, tpkg.Scenario1)

	for index := iotago.SlotIndex(1); index <= iotago.SlotIndex(len(scenarioBuildData)); index++ {
		// apply burns to the slot diff
		manager.updateSlotDiffWithBurns(scenarioBuildData[index].Burns, scenarioBuildData[index].SlotDiff)

		err := manager.commitAccountTree(index, scenarioBuildData[index].SlotDiff, scenarioBuildData[index].DestroyedAccounts)
		require.NoError(t, err)

		for accountID, expectedData := range scenarioExpected[index].AccountsLedger {
			actualData, exists, err2 := manager.Account(accountID)
			assert.NoError(t, err2)
			assert.True(t, exists)
			assert.Equal(t, expectedData, actualData)
		}
	}
}

func TestManager_CommitSlot(t *testing.T) {
	scenarioBuildData, scenarioExpected, blockFunc, burnedBlocks := tpkg.InitScenario(t, tpkg.Scenario2)
	params := tpkg.ProtocolParams()

	slotDiffFunc, _ := tpkg.InitSlotDiff()
	accountsStore := mapdb.NewMapDB()

	manager := New(blockFunc, slotDiffFunc, accountsStore, tpkg.API(), params.MaxCommitableAge)

	for index := iotago.SlotIndex(1); index <= iotago.SlotIndex(len(scenarioBuildData)); index++ {
		for _, burningBlock := range burnedBlocks[index] {
			block, exists := blockFunc(burningBlock)
			assert.True(t, exists)
			manager.TrackBlock(block)
		}
		slotBuildData := scenarioBuildData[index]
		err := manager.ApplyDiff(index, slotBuildData.SlotDiff, slotBuildData.DestroyedAccounts)
		require.NoError(t, err)

		// assert accounts vector is updated correctly
		expectedData := scenarioExpected[index]
		for accID, expectedAccData := range expectedData.AccountsLedger {
			actualData, exists, err2 := manager.Account(accID)
			assert.NoError(t, err2)
			assert.True(t, exists)
			assert.Equal(t, expectedAccData, actualData)
		}
		for accID, expectedDiff := range expectedData.AccountsDiffs {
			actualAccDiff, _, err2 := manager.LoadSlotDiff(index, accID)
			require.NoError(t, err2)
			assert.Equal(t, expectedDiff, actualAccDiff)
		}
	}
}
