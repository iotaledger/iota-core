package accountwallet

import (
	"os"

	"github.com/google/martian/log"

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

type StateData struct {
	Accounts []Account `serix:"0,mapKey=accounts,lengthPrefixType=uint8"`
	// TODO: other info that the account wallet needs to store
}

type Account struct {
	Alias     string           `serix:"0,mapKey=alias"`
	AccountID iotago.AccountID `serix:"1,mapKey=accountID"`
	// TODO: other info of an account
}

func loadWallet(filename string, api iotago.API) (wallet *AccountWallet, err error) {
	walletStateBytes, err := os.ReadFile(filename)
	if err != nil {
		log.Infof("No working wallet file %s found, creating new wallet...", filename)
		wallet = NewAccountWallet(api)
	} else {
		wallet, err = AccountWalletFromBytes(api, walletStateBytes)
		if err != nil {
			return nil, ierrors.Wrap(err, "failed to create wallet from bytes")
		}
	}

	return wallet, nil
}
