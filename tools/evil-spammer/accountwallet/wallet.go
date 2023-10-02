package accountwallet

import (
	"os"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/tools/evil-spammer/models"
	iotago "github.com/iotaledger/iota.go/v4"
)

func Run() (*AccountWallet, error) {
	// read config here
	config := loadAccountConfig()

	var opts []options.Option[AccountWallet]
	if config.BindAddress != "" {
		opts = append(opts, WithClientURL(config.BindAddress))
	}
	if config.AccountStatesFile != "" {
		opts = append(opts, WithAccountStatesFile(config.AccountStatesFile))
	}

	wallet := NewAccountWallet(opts...)

	// load wallet
	err := wallet.fromAccountStateFile()
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to load wallet from file")
	}

	return wallet, nil
}

func SaveState(w *AccountWallet) error {
	return w.toAccountStateFile()
}

type AccountWallet struct {
	// TODO can we reuse faucet requests from evil wallet?
	// faucetFunds    map[string]*Output
	seed [32]byte

	accountsAliases map[string]iotago.AccountID

	client *models.WebClient
	api    iotago.API

	optsClientBindAddress string
	optsAccountStatesFile string
}

func NewAccountWallet(opts ...options.Option[AccountWallet]) *AccountWallet {
	return options.Apply(&AccountWallet{
		accountsAliases:       make(map[string]iotago.Identifier),
		optsClientBindAddress: "http://localhost:8080",
		optsAccountStatesFile: "wallet.dat",
	}, opts, func(w *AccountWallet) {
		w.client = models.NewWebClient(w.optsClientBindAddress)
		w.api = w.client.CurrentAPI()
	})
}

// toAccountStateFile write account states to file.
func (a *AccountWallet) toAccountStateFile() error {
	accounts := make([]Account, 0)

	for alias, acc := range a.accountsAliases {
		accounts = append(accounts, Account{
			Alias:     alias,
			AccountID: acc,
		})
	}

	stateBytes, err := a.api.Encode(&StateData{Accounts: accounts})
	if err != nil {
		return ierrors.Wrap(err, "failed to encode state")
	}

	//nolint:gosec // users should be able to read the file
	if err = os.WriteFile(a.optsAccountStatesFile, stateBytes, 0o644); err != nil {
		return ierrors.Wrap(err, "failed to write account states to file")
	}

	return nil
}

func (a *AccountWallet) fromAccountStateFile() error {
	walletStateBytes, err := os.ReadFile(a.optsAccountStatesFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return ierrors.Wrap(err, "failed to read file")
		}
		return nil
	}

	var data StateData
	_, err = a.api.Decode(walletStateBytes, &data)
	if err != nil {
		return ierrors.Wrap(err, "failed to decode from file")
	}

	for _, acc := range data.Accounts {
		a.accountsAliases[acc.Alias] = acc.AccountID
	}

	return nil
}

func (a *AccountWallet) getFunds(amount uint64) iotago.Output {
	return nil
}

// WithClientURL sets the client bind address.
func WithClientURL(url string) options.Option[AccountWallet] {
	return func(w *AccountWallet) {
		w.optsClientBindAddress = url
	}
}

func WithAccountStatesFile(fileName string) options.Option[AccountWallet] {
	return func(w *AccountWallet) {
		w.optsAccountStatesFile = fileName
	}
}
