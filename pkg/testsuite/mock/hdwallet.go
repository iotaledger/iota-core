package mock

import (
	"crypto/ed25519"
	"fmt"

	"github.com/wollac/iota-crypto-demo/pkg/bip32path"
	"github.com/wollac/iota-crypto-demo/pkg/slip10"
	"github.com/wollac/iota-crypto-demo/pkg/slip10/eddsa"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	pathString = "44'/4218'/0'/%d'"
)

type HDWallet struct {
	name  string
	seed  []byte
	index uint64
	utxo  []*utxoledger.Output
}

func NewHDWallet(name string, seed []byte, index uint64) *HDWallet {
	return &HDWallet{
		name:  name,
		seed:  seed,
		index: index,
		utxo:  make([]*utxoledger.Output, 0),
	}
}

func (hd *HDWallet) BookSpents(spentOutputs []*utxoledger.Output) {
	for _, spent := range spentOutputs {
		hd.BookSpent(spent)
	}
}

func (hd *HDWallet) BookSpent(spentOutput *utxoledger.Output) {
	newUtxo := make([]*utxoledger.Output, 0)
	for _, u := range hd.utxo {
		if u.OutputID() == spentOutput.OutputID() {
			fmt.Printf("%s spent %s\n", hd.name, u.OutputID().ToHex())

			continue
		}
		newUtxo = append(newUtxo, u)
	}
	hd.utxo = newUtxo
}

func (hd *HDWallet) Name() string {
	return hd.name
}

func (hd *HDWallet) Balance() iotago.BaseToken {
	var balance iotago.BaseToken
	for _, u := range hd.utxo {
		balance += u.Deposit()
	}

	return balance
}

func (hd *HDWallet) BookOutput(output *utxoledger.Output) {
	if output != nil {
		fmt.Printf("%s book %s\n", hd.name, output.OutputID().ToHex())
		hd.utxo = append(hd.utxo, output)
	}
}

// KeyPair calculates an ed25519 key pair by using slip10.
func (hd *HDWallet) KeyPair() (ed25519.PrivateKey, ed25519.PublicKey) {
	path, err := bip32path.ParsePath(fmt.Sprintf(pathString, hd.index))
	if err != nil {
		panic(err)
	}

	curve := eddsa.Ed25519()
	key, err := slip10.DeriveKeyFromPath(hd.seed, curve, path)
	if err != nil {
		panic(err)
	}

	pubKey, privKey := key.Key.(eddsa.Seed).Ed25519Key()

	return ed25519.PrivateKey(privKey), ed25519.PublicKey(pubKey)
}

func (hd *HDWallet) AddressSigner() iotago.AddressSigner {
	privKey, pubKey := hd.KeyPair()
	address := iotago.Ed25519AddressFromPubKey(pubKey)

	return iotago.NewInMemoryAddressSigner(iotago.NewAddressKeysForEd25519Address(address, privKey))
}

func (hd *HDWallet) Outputs() []*utxoledger.Output {
	return hd.utxo
}

// Address calculates an ed25519 address by using slip10.
func (hd *HDWallet) Address() *iotago.Ed25519Address {
	_, pubKey := hd.KeyPair()
	addr := iotago.Ed25519AddressFromPubKey(pubKey)

	return addr
}

func (hd *HDWallet) PrintStatus() {
	var status string
	status += fmt.Sprintf("Name: %s\n", hd.name)
	status += fmt.Sprintf("Address: %s\n", hd.Address().Bech32(iotago.PrefixTestnet))
	status += fmt.Sprintf("Balance: %d\n", hd.Balance())
	status += "Outputs: \n"
	for _, u := range hd.utxo {
		nativeTokenDescription := ""
		nativeTokens := u.Output().NativeTokenList().MustSet()
		if len(nativeTokens) > 0 {
			nativeTokenDescription = "["
			for id, amount := range nativeTokens {
				nativeTokenDescription += fmt.Sprintf("%s: %s, ", id.ToHex(), amount.Amount.String())
			}
			nativeTokenDescription += "]"
		}
		status += fmt.Sprintf("\t%s [%s] = %d %v\n", u.OutputID().ToHex(), u.OutputType().String(), u.Deposit(), nativeTokenDescription)
	}
	fmt.Printf("%s\n", status)
}
