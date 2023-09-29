package accountwallet

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/options"
	iotago "github.com/iotaledger/iota.go/v4"
)

type AccountWallet struct {
	// TODO can we reuse faucet requests from evil wallet?
	// faucetFunds    map[string]*Output
	seed [32]byte

	accountsAliases map[string]iotago.AccountID

	api iotago.API
}

func NewAccountWallet(opts ...options.Option[AccountWallet]) *AccountWallet {
	return options.Apply(&AccountWallet{
		accountsAliases: make(map[string]iotago.Identifier),
	}, opts, func(w *AccountWallet) {

	})
}

func Run() (*AccountWallet, error) {
	// load wallet
	wallet, err := loadWallet()
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to load wallet from file")
	}
	defer writeWalletStateFile(wallet, "wallet.dat")

	return wallet, nil
}

// ToAccountData converts accounts information to ToAccountData
func (a *AccountWallet) ToAccountData() *AccountData {
	accounts := make([]Account, 0)

	for alias, acc := range a.accountsAliases {
		accounts = append(accounts, Account{
			Alias:     alias,
			AccountID: acc,
		})
	}

	return &AccountData{Accounts: accounts}
}

func (a *AccountWallet) FromAccountData(data AccountData) {
	for _, acc := range data.Accounts {
		a.accountsAliases[acc.Alias] = acc.AccountID
	}
}

func (a *AccountWallet) getFunds(amount uint64) iotago.Output {
	return nil
}
