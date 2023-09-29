package accountwallet

import (
	"os"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/tools/genesis-snapshot/presets"
	iotago "github.com/iotaledger/iota.go/v4"
)

func Run(walletSourceFile string) (*AccountWallet, error) {
	protocolParams := snapshotcreator.NewOptions(presets.Docker...).ProtocolParameters
	api := iotago.V3API(protocolParams)

	// load wallet
	wallet, err := loadWallet(walletSourceFile, api)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to load wallet from file")
	}

	return wallet, nil
}

func SaveState(w *AccountWallet, filename string) error {
	bytesToWrite, err := w.Bytes()

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

	api iotago.API
}

func NewAccountWallet(api iotago.API, opts ...options.Option[AccountWallet]) *AccountWallet {
	return options.Apply(&AccountWallet{
		accountsAliases: make(map[string]iotago.Identifier),
		api:             api,
	}, opts, func(w *AccountWallet) {

	})
}

func AccountWalletFromBytes(api iotago.API, bytes []byte) (*AccountWallet, error) {
	wallet := NewAccountWallet(api)
	var data StateData
	_, err := wallet.api.Decode(bytes, &data)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to decode from file")
	}

	//TODO: import the data to Wallet
	wallet.fromStateData(data)
	return wallet, nil
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

func (a *AccountWallet) getFunds(amount uint64) iotago.Output {
	return nil
}
