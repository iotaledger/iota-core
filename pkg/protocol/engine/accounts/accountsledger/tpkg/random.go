package tpkg

import (
	"math/rand"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger/tpkg"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/utils"
)

func RandomAccountDiff() *prunable.AccountDiff {
	return &prunable.AccountDiff{
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
		accounts.NewBlockIssuanceCredits(10, utils.RandSlotIndex()),
		utils.RandOutputID(),
		utils.RandPubKey(),
		utils.RandPubKey(),
	)
}
