//go:build dockertests

package tests

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/nodeclient"
	"github.com/iotaledger/iota.go/v4/wallet"
	"github.com/stretchr/testify/require"
)

// Wallet is an object representing a wallet (similar to a FireFly wallet) capable of the following:
// - hierarchical deterministic key management
// - signing transactions
// - signing blocks
// - keeping track of unspent outputs.
type Wallet struct {
	Testing *testing.T

	Name string

	keyManager *wallet.KeyManager

	outputs map[iotago.OutputID]*Output
}

type Output struct {
	ID         iotago.OutputID
	Output     iotago.Output
	Address    iotago.Address
	PrivateKey ed25519.PrivateKey
}

func NewWallet(t *testing.T, name string, keyManager ...*wallet.KeyManager) *Wallet {
	var km *wallet.KeyManager
	if len(keyManager) == 0 {
		km = lo.PanicOnErr(wallet.NewKeyManagerFromRandom(wallet.DefaultIOTAPath))
	} else {
		km = keyManager[0]
	}

	return &Wallet{
		Testing:    t,
		Name:       name,
		outputs:    make(map[iotago.OutputID]*Output),
		keyManager: km,
	}
}

func (w *Wallet) AddOutput(outputId iotago.OutputID, output *Output) {
	w.outputs[outputId] = output
}

func (w *Wallet) Balance() iotago.BaseToken {
	var balance iotago.BaseToken
	for _, output := range w.outputs {
		balance += output.Output.BaseTokenAmount()
	}

	return balance
}

func (w *Wallet) Output(outputName iotago.OutputID) *Output {
	output, exists := w.outputs[outputName]
	if !exists {
		panic(ierrors.Errorf("output %s not registered in wallet %s", outputName, w.Name))
	}

	return output
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

func (w *Wallet) KeyPair(indexes ...uint32) (ed25519.PrivateKey, ed25519.PublicKey) {
	return w.keyManager.KeyPair(indexes...)
}

func (w *Wallet) AddressSigner(indexes ...uint32) iotago.AddressSigner {
	return w.keyManager.AddressSigner(indexes...)
}

func (w *Wallet) CreateDelegationFromInput(clt *nodeclient.Client, from *Account, validator *Node, inputId iotago.OutputID, issuerResp *api.IssuanceBlockHeaderResponse) *iotago.SignedTransaction {
	input := w.Output(inputId)
	fundsAddr := input.Address
	fundsUTXOOutput := input.Output
	fundsOutputID := input.ID
	fundsAddrSigner := iotago.NewInMemoryAddressSigner(iotago.NewAddressKeysForEd25519Address(fundsAddr.(*iotago.Ed25519Address), input.PrivateKey))

	_, validatorAccountAddr, err := iotago.ParseBech32(validator.AccountAddressBech32)
	require.NoError(w.Testing, err)

	currentSlot := clt.LatestAPI().TimeProvider().SlotFromTime(time.Now())
	apiForSlot := clt.APIForSlot(currentSlot)

	// construct delegation transaction

	delegationOutput := builder.NewDelegationOutputBuilder(validatorAccountAddr.(*iotago.AccountAddress), fundsAddr, fundsUTXOOutput.BaseTokenAmount()).
		StartEpoch(getDelegationStartEpoch(apiForSlot, issuerResp.LatestCommitment.Slot)).
		DelegatedAmount(fundsUTXOOutput.BaseTokenAmount()).MustBuild()

	signedTx, err := builder.NewTransactionBuilder(apiForSlot).
		AddInput(&builder.TxInput{
			UnlockTarget: fundsAddr,
			InputID:      fundsOutputID,
			Input:        fundsUTXOOutput,
		}).
		AddOutput(delegationOutput).
		SetCreationSlot(currentSlot).
		AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: lo.Return1(issuerResp.LatestCommitment.ID())}).
		AllotAllMana(currentSlot, from.AccountID, 0).
		Build(fundsAddrSigner)
	require.NoError(w.Testing, err)

	return signedTx
}
