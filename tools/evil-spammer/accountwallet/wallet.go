package accountwallet

import (
	"os"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/tools/evil-spammer/logger"
	"github.com/iotaledger/iota-core/tools/evil-spammer/models"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
	"github.com/mr-tron/base58"
)

var log = logger.New("AccountWallet")

func Run(lastFaucetUnspentOutputID iotago.OutputID) (*AccountWallet, error) {
	// read config here
	config := loadAccountConfig()

	var opts []options.Option[AccountWallet]
	if config.BindAddress != "" {
		opts = append(opts, WithClientURL(config.BindAddress))
	}
	if config.AccountStatesFile != "" {
		opts = append(opts, WithAccountStatesFile(config.AccountStatesFile))
	}

	opts = append(opts, WithFaucetUnspendOutputID(lastFaucetUnspentOutputID))

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
	faucet *Faucet
	seed   [32]byte

	accountsAliases map[string]iotago.AccountID
	latestUsedIndex uint64
	aliasIndexMap   map[string]uint64

	client *models.WebClient
	api    iotago.API

	optsClientBindAddress     string
	optsAccountStatesFile     string
	optsFaucetUnspendOutputID iotago.OutputID
}

func NewAccountWallet(opts ...options.Option[AccountWallet]) *AccountWallet {
	return options.Apply(&AccountWallet{
		accountsAliases: make(map[string]iotago.Identifier),
		seed:            tpkg.RandEd25519Seed(),
	}, opts, func(w *AccountWallet) {
		w.client = models.NewWebClient(w.optsClientBindAddress)
		w.api = w.client.CurrentAPI()
		w.faucet = NewFaucet(w.client, w.optsFaucetUnspendOutputID)
	})
}

func (a *AccountWallet) LastFaucetUnspentOutputID() iotago.OutputID {
	return a.faucet.unspentOutput.OutputID
}

// toAccountStateFile write account states to file.
func (a *AccountWallet) toAccountStateFile() error {
	accounts := make([]Account, 0)

	for alias, acc := range a.accountsAliases {
		accounts = append(accounts, Account{
			Alias:     alias,
			AccountID: acc,
			Index:     a.aliasIndexMap[alias],
		})
	}

	stateBytes, err := a.api.Encode(&StateData{
		Seed:     base58.Encode(a.seed[:]),
		Accounts: accounts,
	})
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

	// copy seeds
	decodedSeeds, err := base58.Decode(data.Seed)
	if err != nil {
		return ierrors.Wrap(err, "failed to decode seed")
	}
	copy(a.seed[:], decodedSeeds)

	// account data
	for _, acc := range data.Accounts {
		a.accountsAliases[acc.Alias] = acc.AccountID
		a.aliasIndexMap[acc.Alias] = acc.Index
	}

	return nil
}

func (a *AccountWallet) getFunds(amount uint64, addressType iotago.AddressType) (*models.Output, error) {
	hdWallet := mock.NewHDWallet("", a.seed[:], a.latestUsedIndex+1)

	receiverAddr := hdWallet.Address(addressType)
	createdOutput, err := a.faucet.RequestFunds(receiverAddr, iotago.BaseToken(amount))
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to request funds from faucet")
	}

	a.latestUsedIndex++
	createdOutput.Index = a.latestUsedIndex

	return createdOutput, nil
}

func (a *AccountWallet) destroyAccount(alias string) error {
	if _, ok := a.accountsAliases[alias]; !ok {
		return ierrors.Errorf("account with alias %s does not exist", alias)
	}
	hdWallet := mock.NewHDWallet("", a.seed[:], a.aliasIndexMap[alias])

	// get output from node
	// From TIP42: Indexers and node plugins shall map the account address of the output derived with Account ID to the regular address -> output mapping table, so that given an Account Address, its most recent unspent account output can be retrieved.
	// TODO: use correct outputID
	accountOutput := a.client.GetOutput(iotago.EmptyOutputID)

	txBuilder := builder.NewTransactionBuilder(a.api)
	txBuilder.AddInput(&builder.TxInput{
		UnlockTarget: a.accountsAliases[alias].ToAddress(),
		// InputID:      accountOutput.ID(),
		Input: accountOutput,
	})

	// send all tokens to faucet
	txBuilder.AddOutput(&iotago.BasicOutput{
		Amount: accountOutput.BaseTokenAmount(),
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: a.faucet.facuetAddress},
		},
	})

	tx, err := txBuilder.Build(hdWallet.AddressSigner())
	if err != nil {
		return ierrors.Wrap(err, "failed to build transaction")
	}

	_, err = a.client.PostTransaction(tx)
	if err != nil {
		return ierrors.Wrap(err, "failed to post transaction")
	}

	// remove account from wallet
	delete(a.accountsAliases, alias)
	delete(a.aliasIndexMap, alias)

	return nil
}
