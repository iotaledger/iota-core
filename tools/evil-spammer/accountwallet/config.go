package accountwallet

import (
	"os"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
)

// commands

const (
	CreateAccountCommand  = "create"
	DestroyAccountCommand = "destroy"
	AllotAccountCommand   = "allot"
	ListAccountsCommand   = "list"
)

var AvailableCommands = map[string]types.Empty{
	CreateAccountCommand:  types.Void,
	DestroyAccountCommand: types.Void,
	AllotAccountCommand:   types.Void,
	ListAccountsCommand:   types.Void,
}

// TODO how do wee read api
type configuration struct {
	WebAPI string `json:"WebAPI,omitempty"`
}

type CreateAccountParams struct {
	Alias  string
	Amount uint64
}

type DestroyAccountParams struct {
	AccountAlias string
}

type AllotAccountParams struct {
	Amount uint64
	To     string
	From   string // if not set we use faucet
}

// TODO do wee need to restrict that only one instance of the wallet runs?
const lockFile = "wallet.LOCK"

type AccountData struct {
	Accounts []Account `serix:"0,mapKey=accounts,lengthPrefixType=uint8"`
	// TODO: other info that the account wallet needs to store
}

type Account struct {
	Alias     string           `serix:"0,mapKey=alias"`
	AccountID iotago.AccountID `serix:"1,mapKey=accountID"`
	// TODO: other info of an account
}

func loadWallet(filename string) (*AccountWallet, error) {
	wallet := NewAccountWallet()

	//walletStateBytes, err := os.ReadFile(filename)
	//if err != nil {
	//	return nil, ierrors.Wrap(err, "failed to read wallet file")
	//}
	//
	//var data AccountData
	//_, err = wallet.api.Decode(bytes, &data)
	//if err != nil {
	//	return nil, ierrors.Wrap(err, "failed to decode from file")
	//}
	//
	//TODO: import the data to Wallet
	//wallet.FromAccountData(data)

	return wallet, nil
}

func writeWalletStateFile(w *AccountWallet, filename string) error {
	dataToDump := w.ToAccountData()

	bytesToWrite, err := w.api.Encode(dataToDump)
	if err != nil {
		return ierrors.Wrap(err, "failed to encode account data")
	}

	//nolint:gosec // users should be able to read the file
	if err = os.WriteFile(filename, bytesToWrite, 0o644); err != nil {
		return ierrors.Wrap(err, "failed to write account file")
	}

	return nil
}
