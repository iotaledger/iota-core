//nolint:gosec
package tpkg

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
	tpkg2 "github.com/iotaledger/iota.go/v4/tpkg"
)

func RandomAccountDiff() *prunable.AccountDiff {
	return &prunable.AccountDiff{
		Change:              int64(rand.New(rand.NewSource(time.Now().UnixNano())).Int()),
		PreviousUpdatedTime: utils.RandSlotIndex(),
		PubKeysAdded:        utils.RandPubKeys(),
		PubKeysRemoved:      utils.RandPubKeys(),
	}
}

func RandomAccountData() *accounts.AccountData {
	return accounts.NewAccountData(
		utils.RandAccountID(),
		accounts.NewBlockIssuanceCredits(10, utils.RandSlotIndex()),
		utils.RandOutputID(),
		utils.RandPubKey(),
		utils.RandPubKey(),
	)
}

func RandomBlocksWithBurns(t *testing.T, burns map[iotago.AccountID]uint64, index iotago.SlotIndex) map[iotago.BlockID]*blocks.Block {
	blocksMap := make(map[iotago.BlockID]*blocks.Block)
	for issuerID, burn := range burns {
		innerBlock := tpkg2.RandBlockWithIssuerAndBurnedMana(issuerID, burn)
		innerBlock.IssuingTime = API().SlotTimeProvider().StartTime(index)
		modelBlock, err := model.BlockFromBlock(innerBlock, API())

		require.NoError(t, err)
		block := blocks.NewBlock(modelBlock)
		blocksMap[block.ID()] = block
	}

	return blocksMap
}
