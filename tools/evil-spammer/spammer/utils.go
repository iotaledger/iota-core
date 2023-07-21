package spammer

import (
	"time"

	"github.com/iotaledger/iota-core/tools/evil-spammer/wallet"
)

// BigWalletsNeeded calculates how many big wallets needs to be prepared for a spam based on provided spam details.
func BigWalletsNeeded(rate int, timeUnit, duration time.Duration) int {
	bigWalletSize := wallet.FaucetRequestSplitNumber * wallet.FaucetRequestSplitNumber
	outputsNeeded := rate * int(duration/timeUnit)
	walletsNeeded := outputsNeeded/bigWalletSize + 1

	return walletsNeeded
}
