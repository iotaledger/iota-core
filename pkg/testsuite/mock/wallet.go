package mock

import (
	"crypto/ed25519"
	"testing"

	hiveEd25519 "github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/wallet"
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

	keyManager *wallet.KeyManager

	BlockIssuer *BlockIssuer

	outputs      map[string]*utxoledger.Output
	transactions map[string]*iotago.Transaction
	currentSlot  iotago.SlotIndex
}

func NewWallet(t *testing.T, name string, node *Node, keyManager ...*wallet.KeyManager) *Wallet {
	var km *wallet.KeyManager
	if len(keyManager) == 0 {
		km = lo.PanicOnErr(wallet.NewKeyManagerFromRandom(wallet.DefaultIOTAPath))
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
		BlockIssuer:  NewBlockIssuer(t, name, km, iotago.EmptyAccountID, false),
	}
}

func (w *Wallet) SetBlockIssuer(accountID iotago.AccountID) {
	w.BlockIssuer = NewBlockIssuer(w.Testing, w.Name, w.keyManager, accountID, false)
}

func (w *Wallet) BlockIssuerKey() iotago.BlockIssuerKey {
	if w.BlockIssuer != nil {
		return w.BlockIssuer.BlockIssuerKey()
	}
	_, pub := w.keyManager.KeyPair()

	return iotago.Ed25519PublicKeyHashBlockIssuerKeyFromPublicKey(hiveEd25519.PublicKey(pub))
}

func (w *Wallet) SetDefaultNode(node *Node) {
	w.Node = node
}

func (w *Wallet) SetCurrentSlot(slot iotago.SlotIndex) {
	w.currentSlot = slot
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
