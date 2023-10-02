package accountwallet

import (
	"os"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/tools/evil-spammer/models"
	iotago "github.com/iotaledger/iota.go/v4"
)

func Run(walletSourceFile string) (*AccountWallet, error) {
	// read config here
	config := loadAccountConfig()

	// load wallet
	wallet, err := loadWallet(walletSourceFile, WithClientURL(config.BindAddress))
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to load wallet from file")
	}

	return wallet, nil
}

func SaveState(w *AccountWallet, filename string) error {
	bytesToWrite, err := w.Bytes()
	if err != nil {
		return ierrors.Wrap(err, "failed to encode wallet state")
	}

	//nolint:gosec // users should be able to read the file
	if err = os.WriteFile(filename, bytesToWrite, 0o644); err != nil {
		return ierrors.Wrap(err, "failed to write account file")
	}

	return nil
}

type AccountWallet struct {
	// TODO can we reuse faucet requests from evil wallet?
	// faucetFunds    map[string]*Output
	seed [32]byte

	accountsAliases map[string]iotago.AccountID

	client *models.WebClient
	api    iotago.API

	optsClientBindAddress string
}

func NewAccountWallet(opts ...options.Option[AccountWallet]) *AccountWallet {
	return options.Apply(&AccountWallet{
		accountsAliases:       make(map[string]iotago.Identifier),
		optsClientBindAddress: "http://localhost:8080",
	}, opts, func(w *AccountWallet) {
		w.client = models.NewWebClient(w.optsClientBindAddress)
		w.api = w.client.CurrentAPI()
	})
}

func (a *AccountWallet) Bytes() ([]byte, error) {
	state := a.toStateData()
	stateBytes, err := a.api.Encode(state)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to encode state")
	}

	return stateBytes, nil
}

// toStateData converts accounts information to ToAccountData
func (a *AccountWallet) toStateData() *StateData {
	accounts := make([]Account, 0)

	for alias, acc := range a.accountsAliases {
		accounts = append(accounts, Account{
			Alias:     alias,
			AccountID: acc,
		})
	}

	return &StateData{Accounts: accounts}
}

func (a *AccountWallet) fromStateData(data StateData) {
	for _, acc := range data.Accounts {
		a.accountsAliases[acc.Alias] = acc.AccountID
	}
}

func (a *AccountWallet) fromStateDataBytes(bytes []byte) error {
	var data StateData
	_, err := a.api.Decode(bytes, &data)
	if err != nil {
		return ierrors.Wrap(err, "failed to decode from file")
	}

	//TODO: import the data to Wallet
	a.fromStateData(data)

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
