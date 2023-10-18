package accountwallet

import (
	"encoding/json"
	"os"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/iota-core/tools/evil-spammer/models"
)

// commands

type AccountOperation int

const (
	OperationCreateAccount AccountOperation = iota
	OperationConvertAccount
	OperationDestroyAccound
	OperationAllotAccount
	OperationDelegateAccount
	OperationStakeAccount
	OperationListAccounts
	OperationUpdateAccount

	CmdNameCreateAccount   = "create"
	CmdNameConvertAccount  = "convert"
	CmdNameDestroyAccount  = "destroy"
	CmdNameAllotAccount    = "allot"
	CmdNameDelegateAccount = "delegate"
	CmdNameStakeAccount    = "stake"
	CmdNameListAccounts    = "list"
	CmdNameUpdateAccount   = "update"
)

func (a AccountOperation) String() string {
	return []string{
		CmdNameCreateAccount,
		CmdNameConvertAccount,
		CmdNameDestroyAccount,
		CmdNameAllotAccount,
		CmdNameDelegateAccount,
		CmdNameStakeAccount,
		CmdNameListAccounts,
		CmdNameUpdateAccount,
	}[a]
}

func AvailableCommands(cmd string) bool {
	availableCommands := map[string]types.Empty{
		CmdNameCreateAccount:   types.Void,
		CmdNameConvertAccount:  types.Void,
		CmdNameDestroyAccount:  types.Void,
		CmdNameAllotAccount:    types.Void,
		CmdNameDelegateAccount: types.Void,
		CmdNameStakeAccount:    types.Void,
		CmdNameListAccounts:    types.Void,
		CmdNameUpdateAccount:   types.Void,
	}

	_, ok := availableCommands[cmd]
	return ok
}

type Configuration struct {
	BindAddress           string `json:"bindAddress,omitempty"`
	AccountStatesFile     string `json:"accountStatesFile,omitempty"`
	GenesisSeed           string `json:"genesisSeed,omitempty"`
	BlockIssuerPrivateKey string `json:"blockIssuerPrivateKey,omitempty"`
	AccountID             string `json:"accountID,omitempty"`
}

var accountConfigFile = "config.json"

var (
	dockerAccountConfigJSON = `{
	"bindAddress": "http://localhost:8080",
	"accountStatesFile": "wallet.dat",
	"genesisSeed": "7R1itJx5hVuo9w9hjg5cwKFmek4HMSoBDgJZN8hKGxih",
	"blockIssuerPrivateKey": "db39d2fde6301d313b108dc9db1ee724d0f405f6fde966bd776365bc5f4a5fb31e4b21eb51dcddf65c20db1065e1f1514658b23a3ddbf48d30c0efc926a9a648",
	"accountID": "0x6aee704f25558e8aa7630fed0121da53074188abc423b3c5810f80be4936eb6e"}`
)

// LoadConfiguration loads the config file.
func LoadConfiguration() *Configuration {
	// open config file
	config := new(Configuration)
	file, err := os.Open(accountConfigFile)
	if err != nil {
		if !os.IsNotExist(err) {
			panic(err)
		}

		//nolint:gosec // users should be able to read the file
		if err = os.WriteFile(accountConfigFile, []byte(dockerAccountConfigJSON), 0o644); err != nil {
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

func SaveConfiguration(config *Configuration) {
	// open config file
	file, err := os.Open(accountConfigFile)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	jsonConfigs, err := json.MarshalIndent(config, "", "    ")

	if err != nil {
		log.Errorf("failed to write configs to file %s", err)
	}

	//nolint:gosec // users should be able to read the file
	if err = os.WriteFile(accountConfigFile, jsonConfigs, 0o644); err != nil {
		panic(err)
	}
}

type AccountSubcommands interface {
	Type() AccountOperation
}

type CreateAccountParams struct {
	Alias    string
	Amount   uint64
	NoBIF    bool
	Implicit bool
}

func (c *CreateAccountParams) Type() AccountOperation {
	return OperationCreateAccount
}

type DestroyAccountParams struct {
	AccountAlias string
	ExpirySlot   uint64
}

func (d *DestroyAccountParams) Type() AccountOperation {
	return OperationDestroyAccound
}

type AllotAccountParams struct {
	Amount uint64
	To     string
	From   string // if not set we use faucet
}

func (a *AllotAccountParams) Type() AccountOperation {
	return OperationAllotAccount
}

type ConvertAccountParams struct {
	AccountAlias string
}

func (d *ConvertAccountParams) Type() AccountOperation {
	return OperationConvertAccount
}

type DelegateAccountParams struct {
	Amount uint64
	To     string
	From   string // if not set we use faucet
}

func (a *DelegateAccountParams) Type() AccountOperation {
	return OperationDelegateAccount
}

type StakeAccountParams struct {
	Alias      string
	Amount     uint64
	FixedCost  uint64
	StartEpoch uint64
	EndEpoch   uint64
}

func (a *StakeAccountParams) Type() AccountOperation {
	return OperationStakeAccount
}

type UpdateAccountParams struct {
	Alias          string
	BlockIssuerKey string
	Mana           uint64
	Amount         uint64
	ExpirySlot     uint64
}

func (a *UpdateAccountParams) Type() AccountOperation {
	return OperationUpdateAccount
}

type NoAccountParams struct {
	Operation AccountOperation
}

func (a *NoAccountParams) Type() AccountOperation {
	return a.Operation
}

type StateData struct {
	Seed          string                 `serix:"0,mapKey=seed,lengthPrefixType=uint8"`
	LastUsedIndex uint64                 `serix:"1,mapKey=lastUsedIndex"`
	AccountsData  []*models.AccountState `serix:"2,mapKey=accounts,lengthPrefixType=uint8"`
}
