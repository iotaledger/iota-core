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

	KeyManager *KeyManager

	BlockIssuer *BlockIssuer

	outputs []*utxoledger.Output
}

func NewWallet(t *testing.T, name string, seed []byte, index uint64) *Wallet {
	return &Wallet{
		Testing:     t,
		Name:        name,
		KeyManager:  NewKeyManager(seed, index),
		BlockIssuer: NewBlockIssuer(t, name, false),
		outputs:     make([]*utxoledger.Output, 0),
	}
}

func (w *Wallet) BookSpents(spentOutputs []*utxoledger.Output) {
	for _, spent := range spentOutputs {
		w.BookSpent(spent)
	}
}

func (w *Wallet) BookSpent(spentOutput *utxoledger.Output) {
	newOutputs := make([]*utxoledger.Output, 0)
	for _, output := range w.outputs {
		if output.OutputID() == spentOutput.OutputID() {
			fmt.Printf("%s spent %s\n", w.Name, output.OutputID().ToHex())

			continue
		}
		newOutputs = append(newOutputs, output)
	}
	w.outputs = newOutputs
}

func (w *Wallet) Balance() iotago.BaseToken {
	var balance iotago.BaseToken
	for _, output := range w.outputs {
		balance += output.BaseTokenAmount()
	}

	return balance
}

func (w *Wallet) BookOutput(output *utxoledger.Output) {
	if output != nil {
		fmt.Printf("%s book %s\n", w.Name, output.OutputID().ToHex())
		w.outputs = append(w.outputs, output)
	}
}

func (w *Wallet) Outputs() []*utxoledger.Output {
	return w.outputs
}

func (w *Wallet) PrintStatus() {
	var status string
	status += fmt.Sprintf("Name: %s\n", w.Name)
	status += fmt.Sprintf("Address: %s\n", w.KeyManager.Address().Bech32(iotago.PrefixTestnet))
	status += fmt.Sprintf("Balance: %d\n", w.Balance())
	status += "Outputs: \n"
	for _, u := range w.outputs {
		nativeTokenDescription := ""
		nativeTokenFeature := u.Output().FeatureSet().NativeToken()
		if nativeTokenFeature != nil {
			nativeTokenDescription += fmt.Sprintf("%s: %s, ", nativeTokenFeature.ID.ToHex(), nativeTokenFeature.Amount)
		}
		status += fmt.Sprintf("\t%s [%s] = %d %v\n", u.OutputID().ToHex(), u.OutputType(), u.BaseTokenAmount(), nativeTokenDescription)
	}
	fmt.Printf("%s\n", status)
}
