package accountwallet

import (
	"encoding/json"
	"os"

	"github.com/google/martian/log"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/options"
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

type Configuration struct {
	BindAddress string `json:"bindAddress,omitempty"`
}

var accountConfigFile = "account_config.json"

var accountConfigJSON = `{
	"bindAddress": "http://localhost:8080"
}`

// loadAccountConfig loads the config file.
func loadAccountConfig() *Configuration {
	// open config file
	config := new(Configuration)
	file, err := os.Open(accountConfigFile)
	if err != nil {
		if !os.IsNotExist(err) {
			panic(err)
		}

		//nolint:gosec // users should be able to read the file
		if err = os.WriteFile(accountConfigFile, []byte(accountConfigJSON), 0o644); err != nil {
			panic(err)
		}
		if file, err = os.Open(accountConfigFile); err != nil {
			panic(err)
		}
	}
	defer file.Close()

	// decode config file
	if err = json.NewDecoder(file).Decode(config); err != nil {
		panic(err)
	}

	return config
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

func loadWallet(filename string, opts ...options.Option[AccountWallet]) (wallet *AccountWallet, err error) {
	wallet = NewAccountWallet(opts...)

	walletStateBytes, err := os.ReadFile(filename)
	if err != nil {
		log.Infof("No working wallet file %s found, creating new wallet...", filename)
		return wallet, nil
	}

	err = wallet.fromStateDataBytes(walletStateBytes)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to create wallet from bytes")
	}

	return wallet, nil
}
