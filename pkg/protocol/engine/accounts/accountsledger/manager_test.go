package accountsledger

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/accountsledger/tpkg"
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

func TestManager_CommitSlot(t *testing.T) {
	scenarioBuildData, scenarioExpected := tpkg.AccountLedgerScenario1()

	params := tpkg.ProtocolParams()
	blockFunc, burnedBlocks := tpkg.BlockFuncScenario1(t)

	slotDiffFunc, _ := tpkg.InitSlotDiff()
	accountsStore := mapdb.NewMapDB()

	manager := New(blockFunc, slotDiffFunc, accountsStore, tpkg.API(), params.MaxCommitableAge)

	// todo pass info about start/stop index through the scenario
	for index := iotago.SlotIndex(1); index <= 5; index++ {
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
			// todo waht with destroyed assertion
			fmt.Print(accID, expectedDiff.Change)
			actualAccDiff, _, err := manager.LoadSlotDiff(index, accID)
			require.NoError(t, err)
			assert.Equal(t, expectedDiff, actualAccDiff)
		}
	}
}
