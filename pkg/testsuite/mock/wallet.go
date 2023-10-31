package mock

import (
	"crypto/ed25519"
	"testing"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

// Wallet is an object representing a wallet (similar to a FireFly wallet) capable of the following:
// - hierarchical deterministic key management
// - signing transactions
// - signing blocks
// - keeping track of unspent outputs.
type Wallet struct {
	Testing *testing.T

	Name string

	Node *Node

	keyManager *KeyManager

	BlockIssuer *BlockIssuer

	outputs      map[string]*utxoledger.Output
	transactions map[string]*iotago.Transaction
}

func NewWallet(t *testing.T, name string, node *Node, keyManager ...*KeyManager) *Wallet {
	var km *KeyManager
	if len(keyManager) == 0 {
		randomSeed := tpkg.RandEd25519Seed()
		km = NewKeyManager(randomSeed[:], 0)
	} else {
		km = keyManager[0]
	}

	return &Wallet{
		Testing:      t,
		Name:         name,
		Node:         node,
		outputs:      make(map[string]*utxoledger.Output),
		transactions: make(map[string]*iotago.Transaction),
		keyManager:   km,
	}
}

func (w *Wallet) SetBlockIssuer(accountID iotago.AccountID) {
	w.BlockIssuer = NewBlockIssuer(w.Testing, w.Name, w.keyManager, accountID, false)
}

func (w *Wallet) SetDefaultNode(node *Node) {
	w.Node = node
}

func (w *Wallet) AddOutput(outputName string, output *utxoledger.Output) {
	w.outputs[outputName] = output
}

func (w *Wallet) Balance() iotago.BaseToken {
	var balance iotago.BaseToken
	for _, output := range w.outputs {
		balance += output.BaseTokenAmount()
	}

	return balance
}

func (w *Wallet) Output(outputName string) *utxoledger.Output {
	output, exists := w.outputs[outputName]
	if !exists {
		panic(ierrors.Errorf("output %s not registered in wallet %s", outputName, w.Name))
	}

	return output
}

func (w *Wallet) AccountOutput(outputName string) *utxoledger.Output {
	output := w.Output(outputName)
	if _, ok := output.Output().(*iotago.AccountOutput); !ok {
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
	return lo.PanicOnErr(w.Transaction(alias).ID())
}

func (w *Wallet) Address() iotago.DirectUnlockableAddress {
	address := w.keyManager.Address(iotago.AddressEd25519)
	//nolint:forcetypeassert
	return address.(*iotago.Ed25519Address)
}

func (w *Wallet) ImplicitAccountCreationAddress() *iotago.ImplicitAccountCreationAddress {
	address := w.keyManager.Address(iotago.AddressImplicitAccountCreation)
	//nolint:forcetypeassert
	return address.(*iotago.ImplicitAccountCreationAddress)
}

func (w *Wallet) HasAddress(address iotago.Address) bool {
	return address.Equal(w.Address()) || address.Equal(w.ImplicitAccountCreationAddress())
}

func (w *Wallet) KeyPair() (ed25519.PrivateKey, ed25519.PublicKey) {
	return w.keyManager.KeyPair()
}

func (w *Wallet) AddressSigner() iotago.AddressSigner {
	return w.keyManager.AddressSigner()
}
