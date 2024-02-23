//go:build dockertests

package tests

import (
	"crypto/ed25519"
	"math/big"
	"testing"
	"time"

	hiveEd25519 "github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
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

func (w *Wallet) TransitionImplicitAccountToAccountOutput(clt *nodeclient.Client, inputId iotago.OutputID, issuerResp *api.IssuanceBlockHeaderResponse, opts ...options.Option[builder.AccountOutputBuilder]) (*Account, *iotago.SignedTransaction) {
	input := w.Output(inputId)
	implicitAddr := input.Address
	implicitOutput := input.Output
	implicitOutputID := input.ID
	implicitAddrSigner := iotago.NewInMemoryAddressSigner(iotago.NewAddressKeysForImplicitAccountCreationAddress(implicitAddr.(*iotago.ImplicitAccountCreationAddress), input.PrivateKey))

	accountID := iotago.AccountIDFromOutputID(implicitOutputID)
	accountAddress, ok := accountID.ToAddress().(*iotago.AccountAddress)
	require.True(w.Testing, ok)

	currentSlot := clt.LatestAPI().TimeProvider().SlotFromTime(time.Now())
	apiForSlot := clt.APIForSlot(currentSlot)

	// transition to a full account with new Ed25519 address and staking feature
	accEd25519Addr := w.Address()
	accPrivateKey, _ := w.KeyPair()
	accBlockIssuerKey := iotago.Ed25519PublicKeyHashBlockIssuerKeyFromPublicKey(hiveEd25519.PublicKey(accPrivateKey.Public().(ed25519.PublicKey)))
	accountOutput := options.Apply(builder.NewAccountOutputBuilder(accEd25519Addr, implicitOutput.BaseTokenAmount()),
		opts, func(b *builder.AccountOutputBuilder) {
			b.AccountID(accountID).
				BlockIssuer(iotago.NewBlockIssuerKeys(accBlockIssuerKey), iotago.MaxSlotIndex)
		}).MustBuild()

	signedTx, err := builder.NewTransactionBuilder(apiForSlot).
		AddInput(&builder.TxInput{
			UnlockTarget: implicitAddr,
			InputID:      implicitOutputID,
			Input:        implicitOutput,
		}).
		AddOutput(accountOutput).
		SetCreationSlot(currentSlot).
		AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: lo.Return1(issuerResp.LatestCommitment.ID())}).
		AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: accountID}).
		AllotAllMana(currentSlot, accountID, 0).
		Build(implicitAddrSigner)
	require.NoError(w.Testing, err)

	accountOutputId := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)
	w.AddOutput(accountOutputId, &Output{
		ID:         accountOutputId,
		Output:     accountOutput,
		Address:    accEd25519Addr,
		PrivateKey: accPrivateKey,
	})

	return &Account{
		AccountID:      accountID,
		AccountAddress: accountAddress,
		BlockIssuerKey: accPrivateKey,
		AccountOutput:  accountOutput,
		OutputID:       accountOutputId,
	}, signedTx
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

	delegationOutputId := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)
	w.AddOutput(delegationOutputId, &Output{
		ID:         delegationOutputId,
		Output:     delegationOutput,
		Address:    fundsAddr,
		PrivateKey: input.PrivateKey,
	})

	return signedTx
}

func (w *Wallet) CreateFoundryAndNativeTokensFromInput(clt *nodeclient.Client, from *Account, inputId iotago.OutputID, mintedAmount iotago.BaseToken, maxSupply iotago.BaseToken, issuerResp *api.IssuanceBlockHeaderResponse) *iotago.SignedTransaction {
	input := w.Output(inputId)
	fundsAddr := input.Address
	fundsUTXOOutput := input.Output
	fundsOutputID := input.ID

	currentSlot := clt.LatestAPI().TimeProvider().SlotFromTime(time.Now())
	apiForSlot := clt.APIForSlot(currentSlot)

	// increase foundry counter
	accTransitionOutput := builder.NewAccountOutputBuilderFromPrevious(from.AccountOutput).
		FoundriesToGenerate(1).MustBuild()

	// build foundry output
	foundryID, err := iotago.FoundryIDFromAddressAndSerialNumberAndTokenScheme(from.AccountAddress, accTransitionOutput.FoundryCounter, iotago.TokenSchemeSimple)
	require.NoError(w.Testing, err)
	tokenScheme := &iotago.SimpleTokenScheme{
		MintedTokens:  big.NewInt(int64(mintedAmount)),
		MaximumSupply: big.NewInt(int64(maxSupply)),
		MeltedTokens:  big.NewInt(0),
	}

	foundryOutput := builder.NewFoundryOutputBuilder(from.AccountAddress, fundsUTXOOutput.BaseTokenAmount(), accTransitionOutput.FoundryCounter, tokenScheme).
		NativeToken(&iotago.NativeTokenFeature{
			ID:     foundryID,
			Amount: big.NewInt(int64(mintedAmount)),
		}).MustBuild()

	signer := iotago.NewInMemoryAddressSigner(iotago.NewAddressKeysForEd25519Address(fundsAddr.(*iotago.Ed25519Address), input.PrivateKey),
		iotago.NewAddressKeysForEd25519Address(from.AccountOutput.UnlockConditionSet().Address().Address.(*iotago.Ed25519Address), from.BlockIssuerKey))

	signedTx, err := builder.NewTransactionBuilder(apiForSlot).
		AddInput(&builder.TxInput{
			UnlockTarget: fundsAddr,
			InputID:      fundsOutputID,
			Input:        fundsUTXOOutput,
		}).
		AddInput(&builder.TxInput{
			UnlockTarget: from.AccountOutput.UnlockConditionSet().Address().Address,
			InputID:      from.OutputID,
			Input:        from.AccountOutput,
		}).
		AddOutput(accTransitionOutput).
		AddOutput(foundryOutput).
		SetCreationSlot(currentSlot).
		AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: from.AccountID}).
		AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: lo.Return1(issuerResp.LatestCommitment.ID())}).
		AllotAllMana(currentSlot, from.AccountID, 0).
		Build(signer)
	require.NoError(w.Testing, err)

	foundryOutputId := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 1)
	w.AddOutput(foundryOutputId, &Output{
		ID:      foundryOutputId,
		Output:  foundryOutput,
		Address: from.AccountAddress,
	})

	return signedTx
}

// TransitionFoundry transitions a FoundryOutput by increasing the native token amount on the output by one.
func (w *Wallet) TransitionFoundry(clt *nodeclient.Client, from *Account, inputId iotago.OutputID, issuerResp *api.IssuanceBlockHeaderResponse) *iotago.SignedTransaction {
	input := w.Output(inputId)
	inputFoundry, isFoundry := input.Output.(*iotago.FoundryOutput)
	require.True(w.Testing, isFoundry)

	nativeTokenAmount := inputFoundry.FeatureSet().NativeToken().Amount
	previousTokenScheme, isSimple := inputFoundry.TokenScheme.(*iotago.SimpleTokenScheme)
	require.True(w.Testing, isSimple)

	tokenScheme := &iotago.SimpleTokenScheme{
		MaximumSupply: previousTokenScheme.MaximumSupply,
		MeltedTokens:  previousTokenScheme.MeltedTokens,
		MintedTokens:  previousTokenScheme.MintedTokens.Add(previousTokenScheme.MintedTokens, big.NewInt(1)),
	}

	require.Greater(w.Testing, tokenScheme.MintedTokens.Cmp(tokenScheme.MaximumSupply), 0)

	outputFoundry := builder.NewFoundryOutputBuilderFromPrevious(inputFoundry).
		NativeToken(&iotago.NativeTokenFeature{
			ID:     inputFoundry.MustFoundryID(),
			Amount: nativeTokenAmount.Add(nativeTokenAmount, big.NewInt(1)),
		}).
		TokenScheme(tokenScheme).
		MustBuild()

	outputAccount := builder.NewAccountOutputBuilderFromPrevious(from.AccountOutput).
		MustBuild()

	currentSlot := clt.LatestAPI().TimeProvider().SlotFromTime(time.Now())
	apiForSlot := clt.APIForSlot(currentSlot)

	signer := iotago.NewInMemoryAddressSigner(iotago.NewAddressKeysForEd25519Address(input.Address.(*iotago.Ed25519Address), input.PrivateKey),
		iotago.NewAddressKeysForEd25519Address(from.AccountOutput.UnlockConditionSet().Address().Address.(*iotago.Ed25519Address), from.BlockIssuerKey))

	signedTx, err := builder.NewTransactionBuilder(apiForSlot).
		AddInput(&builder.TxInput{
			UnlockTarget: input.Address,
			InputID:      input.ID,
			Input:        input.Output,
		}).
		AddInput(&builder.TxInput{
			UnlockTarget: from.AccountOutput.UnlockConditionSet().Address().Address,
			InputID:      from.OutputID,
			Input:        from.AccountOutput,
		}).
		AddOutput(outputAccount).
		AddOutput(outputFoundry).
		SetCreationSlot(currentSlot).
		AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: from.AccountID}).
		AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: lo.Return1(issuerResp.LatestCommitment.ID())}).
		AllotAllMana(currentSlot, from.AccountID, 0).
		Build(signer)
	require.NoError(w.Testing, err)

	foundryOutputId := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 1)
	w.AddOutput(foundryOutputId, &Output{
		ID:      foundryOutputId,
		Output:  outputFoundry,
		Address: from.AccountAddress,
	})

	return signedTx
}
