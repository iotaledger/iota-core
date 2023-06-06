package accountsledger_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/accountsledger"
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
	manager := accountsledger.New(blockFunc, slotDiffFunc, accountsStore, tpkg.API())

	for _, blockID := range blockIDs {
		block, exist := blockFunc(blockID)
		require.True(t, exist)
		manager.TrackBlock(block)
	}
	managerBurns, err := manager.CreateBlockBurnsForSlot(1)
	require.NoError(t, err)
	assert.EqualValues(t, burns[1], managerBurns)
}
