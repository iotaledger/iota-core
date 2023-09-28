package accountwallet

import iotago "github.com/iotaledger/iota.go/v4"

type AccountWallet struct {
	// TODO can we reuse faucet requests from evil wallet?
	// faucetFunds    map[string]*Output
	seed [32]byte

	accountsAliases map[string]iotago.AccountID

	api iotago.API
}

func Run() *AccountWallet {

	// load wallet
	wallet := loadWallet()
	defer writeWalletStateFile(wallet, "wallet.dat")

	return wallet
}

func (a *AccountWallet) getFunds(amount uint64) iotago.Output {
	return nil
}
