package accountwallet

import (
	"os"

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

type configuration struct {
	WebAPI string `json:"WebAPI,omitempty"`
}

type CommandParams interface {
	CreateAccountParams | DestroyAccountParams | AllotAccountParams
}

type CreateAccountParams struct {
	Alias  string
	Amount uint64
}

type DestroyAccountParams struct {
	AccountAlias string
}

type AllotAccountParams struct {
	AccountAlias string
	Amount       uint64
}

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

func loadWallet() (*AccountWallet, error) {
	wallet := NewAccountWallet()

	bytes, err := os.ReadFile(lockFile)
	if err != nil {
		if err == os.ErrNotExist {
			return wallet, nil
		}
		return nil, ierrors.Wrap(err, "failed to read account file")
	}

	var data AccountData
	_, err = wallet.api.Decode(bytes, &data)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to decode from file")
	}

	// TODO: import the data to Wallet
	wallet.FromAccountData(data)

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
