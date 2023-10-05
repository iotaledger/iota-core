package mock

import (
	"fmt"
	"testing"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	pathString = "44'/4218'/0'/%d'"
)

// Wallet is an object representing a wallet (similar to a FireFly wallet) capable of the following:
// - hierarchical deterministic key management
// - signing transactions
// - signing blocks
// - keeping track of unspent outputs.
type Wallet struct {
	Testing *testing.T

	Name string
	utxo []*utxoledger.Output

	KeyManager *KeyManager

	BlockIssuer *BlockIssuer
}

func NewWallet(t *testing.T, name string, seed []byte, index uint64) *Wallet {
	return &Wallet{
		Testing: t,
		Name:    name,
		utxo:    make([]*utxoledger.Output, 0),

		KeyManager: NewKeyManager(seed, index),

		BlockIssuer: NewBlockIssuer(t, name, false),
	}
}

func (w *Wallet) BookSpents(spentOutputs []*utxoledger.Output) {
	for _, spent := range spentOutputs {
		w.BookSpent(spent)
	}
}

func (w *Wallet) BookSpent(spentOutput *utxoledger.Output) {
	newUtxo := make([]*utxoledger.Output, 0)
	for _, u := range w.utxo {
		if u.OutputID() == spentOutput.OutputID() {
			fmt.Printf("%s spent %s\n", w.Name, u.OutputID().ToHex())

			continue
		}
		newUtxo = append(newUtxo, u)
	}
	w.utxo = newUtxo
}

func (w *Wallet) Balance() iotago.BaseToken {
	var balance iotago.BaseToken
	for _, u := range w.utxo {
		balance += u.BaseTokenAmount()
	}

	return balance
}

func (w *Wallet) BookOutput(output *utxoledger.Output) {
	if output != nil {
		fmt.Printf("%s book %s\n", w.Name, output.OutputID().ToHex())
		w.utxo = append(w.utxo, output)
	}
}

func (w *Wallet) Outputs() []*utxoledger.Output {
	return w.utxo
}

func (w *Wallet) PrintStatus() {
	var status string
	status += fmt.Sprintf("Name: %s\n", w.Name)
	status += fmt.Sprintf("Address: %s\n", w.KeyManager.Address().Bech32(iotago.PrefixTestnet))
	status += fmt.Sprintf("Balance: %d\n", w.Balance())
	status += "Outputs: \n"
	for _, u := range w.utxo {
		nativeTokenDescription := ""
		nativeTokenFeature := u.Output().FeatureSet().NativeToken()
		if nativeTokenFeature != nil {
			nativeTokenDescription += fmt.Sprintf("%s: %s, ", nativeTokenFeature.ID.ToHex(), nativeTokenFeature.Amount)
		}
		status += fmt.Sprintf("\t%s [%s] = %d %v\n", u.OutputID().ToHex(), u.OutputType(), u.BaseTokenAmount(), nativeTokenDescription)
	}
	fmt.Printf("%s\n", status)
}
