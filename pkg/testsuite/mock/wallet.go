package mock

import (
	"crypto/ed25519"
	"fmt"
	"testing"

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

	keyManager *KeyManager

	BlockIssuer *BlockIssuer

	outputs []*utxoledger.Output
}

func NewWallet(t *testing.T, name string, seed ...[]byte) *Wallet {
	if len(seed) == 0 {
		randomSeed := tpkg.RandEd25519Seed()
		seed = append(seed, randomSeed[:])
	}
	keyManager := NewKeyManager(seed[0], 0)

	return &Wallet{
		Testing:    t,
		Name:       name,
		keyManager: keyManager,
		outputs:    make([]*utxoledger.Output, 0),
	}
}

func (w *Wallet) AddBlockIssuer(accountID iotago.AccountID) {
	w.BlockIssuer = NewBlockIssuer(w.Testing, w.Name, w.keyManager, accountID, false)
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
	status += fmt.Sprintf("Address: %s\n", w.keyManager.Address().Bech32(iotago.PrefixTestnet))
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

func (w *Wallet) Address(addressType ...iotago.AddressType) iotago.DirectUnlockableAddress {
	return w.keyManager.Address(addressType...)
}

func (w *Wallet) ImplicitAccountCreationAddress() *iotago.ImplicitAccountCreationAddress {
	address := w.keyManager.Address(iotago.AddressImplicitAccountCreation)
	//nolint:forcetypeassert
	return address.(*iotago.ImplicitAccountCreationAddress)
}

func (w *Wallet) KeyPair() (ed25519.PrivateKey, ed25519.PublicKey) {
	return w.keyManager.KeyPair()
}

func (w *Wallet) AddressSigner() iotago.AddressSigner {
	return w.keyManager.AddressSigner()
}
