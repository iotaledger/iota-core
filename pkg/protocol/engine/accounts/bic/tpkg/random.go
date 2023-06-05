package tpkg

import (
	"math/rand"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate/tpkg"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/utils"
)

func RandomBicDiffChange() *prunable.BICDiff {
	return &prunable.BICDiff{
		Change:              int64(rand.Int()),
		PreviousUpdatedTime: utils.RandSlotIndex(),
		PubKeysAdded:        utils.RandPubKeys(),
		PubKeysRemoved:      utils.RandPubKeys(),
	}
}

func RandomAccountData() *accounts.AccountData {
	return accounts.NewAccountData(
		tpkg.API(),
		utils.RandAccountID(),
		accounts.NewCredits(10, utils.RandSlotIndex()),
		utils.RandPubKey(),
		utils.RandPubKey(),
	)
}
