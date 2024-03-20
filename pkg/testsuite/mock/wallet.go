package mock

import (
	"crypto/ed25519"
	"testing"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/wallet"
)

type OutputData struct {
	// ID is the unique identifier of the output.
	ID iotago.OutputID
	// Output is the iotago Output.
	Output iotago.Output
	// Address is the address of the output.
	Address iotago.Address
	// AddressIndex is the index of the address in the keyManager.
	AddressIndex uint32
}

func OutputDataFromUTXOLedgerOutput(output *utxoledger.Output) *OutputData {
	return &OutputData{
		ID:     output.OutputID(),
		Output: output.Output(),
	}
}

// AccountData holds the details of an account that can be used to issue a block or account transition.
type AccountData struct {
	// ID is the unique identifier of the account.
	ID iotago.AccountID
	// AddressIndex is the index of the address in the keyManager.
	AddressIndex uint32
	// Address is the Address of the account output.
	Address *iotago.AccountAddress
	// Output is the latest iotago AccountOutput of the account.
	Output *iotago.AccountOutput
	// OutputID is the unique identifier of the Output.
	OutputID iotago.OutputID
}

// WalletClock is an interface that provides the current slot.
type WalletClock interface {
	SetCurrentSlot(slot iotago.SlotIndex)
	CurrentSlot() iotago.SlotIndex
}

type TestSuiteWalletClock struct {
	currentSlot iotago.SlotIndex
}

func (c *TestSuiteWalletClock) SetCurrentSlot(slot iotago.SlotIndex) {
	c.currentSlot = slot
}

func (c *TestSuiteWalletClock) CurrentSlot() iotago.SlotIndex {
	return c.currentSlot
}

// Wallet is an object representing a wallet (similar to a FireFly wallet) capable of the following:
// - hierarchical deterministic key management
// - signing transactions
// - signing blocks
// - keeping track of unspent outputs.
type Wallet struct {
	Testing *testing.T

	Name string

	Client Client

	keyManager *wallet.KeyManager

	BlockIssuer   *BlockIssuer
	IssuerAccount *AccountData

	outputs      map[string]*OutputData
	transactions map[string]*iotago.Transaction
	clock        WalletClock
}

func NewWallet(t *testing.T, name string, client Client, keyManager ...*wallet.KeyManager) *Wallet {
	t.Helper()

	var km *wallet.KeyManager
	if len(keyManager) == 0 {
		km = lo.PanicOnErr(wallet.NewKeyManagerFromRandom(wallet.DefaultIOTAPath))
	} else {
		km = keyManager[0]
	}
	issuerAccountData := &AccountData{
		ID:           iotago.EmptyAccountID,
		AddressIndex: 0,
	}

	return &Wallet{
		Testing:       t,
		Name:          name,
		Client:        client,
		outputs:       make(map[string]*OutputData),
		transactions:  make(map[string]*iotago.Transaction),
		keyManager:    km,
		IssuerAccount: issuerAccountData,
		BlockIssuer:   NewBlockIssuer(t, name, km, client, issuerAccountData.AddressIndex, issuerAccountData.ID, false),
		clock:         &TestSuiteWalletClock{},
	}
}

func (w *Wallet) SetBlockIssuer(accountData *AccountData) {
	w.BlockIssuer = NewBlockIssuer(w.Testing, w.Name, w.keyManager, w.Client, accountData.AddressIndex, accountData.ID, false)
}

func (w *Wallet) SetDefaultClient(client Client) {
	w.Client = client
	w.BlockIssuer.Client = client
}

func (w *Wallet) SetCurrentSlot(slot iotago.SlotIndex) {
	w.clock.SetCurrentSlot(slot)
}

func (w *Wallet) CurrentSlot() iotago.SlotIndex {
	return w.clock.CurrentSlot()
}

func (w *Wallet) AddOutput(outputName string, output *OutputData) {
	w.outputs[outputName] = output
}

func (w *Wallet) Balance() iotago.BaseToken {
	var balance iotago.BaseToken
	for _, outputData := range w.outputs {
		balance += outputData.Output.BaseTokenAmount()
	}

	return balance
}

func (w *Wallet) OutputData(outputName string) *OutputData {
	output, exists := w.outputs[outputName]
	if !exists {
		panic(ierrors.Errorf("output %s not registered in wallet %s", outputName, w.Name))
	}

	return output
}

func (w *Wallet) AccountOutputData(outputName string) *OutputData {
	output := w.OutputData(outputName)
	if _, ok := output.Output.(*iotago.AccountOutput); !ok {
		panic(ierrors.Errorf("output %s is not an account output", outputName))
	}

	return output
}

func (w *Wallet) Transaction(alias string) *iotago.Transaction {
	transaction, exists := w.transactions[alias]
	if !exists {
		panic(ierrors.Errorf("transaction with given alias does not exist %s", alias))
	}

	return transaction
}

func (w *Wallet) Transactions(transactionNames ...string) []*iotago.Transaction {
	return lo.Map(transactionNames, w.Transaction)
}

func (w *Wallet) TransactionID(alias string) iotago.TransactionID {
	return w.Transaction(alias).MustID()
}

func (w *Wallet) Address(index ...uint32) iotago.DirectUnlockableAddress {
	address := w.keyManager.Address(iotago.AddressEd25519, index...)
	//nolint:forcetypeassert
	return address.(*iotago.Ed25519Address)
}

func (w *Wallet) ImplicitAccountCreationAddress(index ...uint32) *iotago.ImplicitAccountCreationAddress {
	address := w.keyManager.Address(iotago.AddressImplicitAccountCreation, index...)
	//nolint:forcetypeassert
	return address.(*iotago.ImplicitAccountCreationAddress)
}

func (w *Wallet) HasAddress(address iotago.Address, index ...uint32) bool {
	return address.Equal(w.Address(index...)) || address.Equal(w.ImplicitAccountCreationAddress(index...))
}

func (w *Wallet) KeyPair() (ed25519.PrivateKey, ed25519.PublicKey) {
	return w.keyManager.KeyPair()
}

func (w *Wallet) AddressSigner(indexes ...uint32) iotago.AddressSigner {
	return w.keyManager.AddressSigner(indexes...)
}

func (w *Wallet) GetNewBlockIssuanceResponse() *api.IssuanceBlockHeaderResponse {
	return w.BlockIssuer.GetNewBlockIssuanceResponse()
}
