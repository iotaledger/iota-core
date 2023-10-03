package accountwallet

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/mr-tron/base58/base58"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/iota-core/tools/evil-spammer/models"
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
	BindAddress       string `json:"bindAddress,omitempty"`
	AccountStatesFile string `json:"accountStatesFile,omitempty"`
}

var accountConfigFile = "account_config.json"

var accountConfigJSON = `{
	"bindAddress": "http://localhost:8080",
	"accountStatesFile": "wallet.dat"
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

type StateData struct {
	Seed          string                `serix:"0,mapKey=seed,lengthPrefixType=uint8"`
	LastUsedIndex uint64                `serix:"1,mapKey=lastUsedIndex"`
	AccountsData  []*models.AccountData `serix:"2,mapKey=accounts,lengthPrefixType=uint32"`
}

var dockerFaucetSeed = func() []byte {
	genesisSeed, err := base58.Decode("7R1itJx5hVuo9w9hjg5cwKFmek4HMSoBDgJZN8hKGxih")
	if err != nil {
		fmt.Printf("failed to decode base58 seed, using the default one: %v", err)
	}

	return genesisSeed
}

var genesisTransactionID = iotago.TransactionIDRepresentingData(0, []byte("genesis"))
