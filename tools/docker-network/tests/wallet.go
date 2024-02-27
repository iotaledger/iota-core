//go:build dockertests

package tests

import (
	"crypto/ed25519"
	"math/big"
	"sync/atomic"
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

// DockerWallet holds a keyManager, created outputs and accounts details.
type DockerWallet struct {
	Testing *testing.T

	keyManager *wallet.KeyManager

	lastUsedIndex atomic.Uint32

	outputs  map[iotago.OutputID]*Output
	accounts map[iotago.AccountID]*Account
}

// Output holds the details of an output that can be used to build a transaction.
type Output struct {
	// ID is the unique identifier of the output.
	ID iotago.OutputID
	// Output is the iotago output.
	Output iotago.Output
	// Address is the address of the output.
	Address iotago.Address
	// AddressIndex is the index of the address in the keyManager.
	AddressIndex uint32
}

// Account holds the details of an account that can be used to issue a block or account transition.
type Account struct {
	ID           iotago.AccountID
	AddressIndex uint32
	Address      *iotago.AccountAddress
	Output       *iotago.AccountOutput
	OutputID     iotago.OutputID
}

func NewDockerWallet(t *testing.T) *DockerWallet {
	return &DockerWallet{
		Testing:    t,
		outputs:    make(map[iotago.OutputID]*Output),
		accounts:   make(map[iotago.AccountID]*Account),
		keyManager: lo.PanicOnErr(wallet.NewKeyManagerFromRandom(wallet.DefaultIOTAPath)),
	}
}

func (w *DockerWallet) AddOutput(outputId iotago.OutputID, output *Output) {
	w.outputs[outputId] = output
}

func (w *DockerWallet) AddAccount(accountId iotago.AccountID, data *Account) {
	w.accounts[accountId] = data
}

func (w *DockerWallet) Output(outputName iotago.OutputID) *Output {
	output, exists := w.outputs[outputName]
	if !exists {
		panic(ierrors.Errorf("output %s not registered in wallet", outputName))
	}

	return output
}

func (w *DockerWallet) Account(accountId iotago.AccountID) *Account {
	acc, exists := w.accounts[accountId]
	if !exists {
		panic(ierrors.Errorf("account %s not registered in wallet", accountId.ToHex()))
	}

	return acc
}

func (w *DockerWallet) Address(index ...uint32) (uint32, *iotago.Ed25519Address) {
	if len(index) == 0 {
		index = append(index, w.lastUsedIndex.Add(1))
	}

	address := w.keyManager.Address(iotago.AddressEd25519, index...)
	//nolint:forcetypeassert
	return index[0], address.(*iotago.Ed25519Address)
}

func (w *DockerWallet) ImplicitAccountCreationAddress(index ...uint32) (uint32, *iotago.ImplicitAccountCreationAddress) {
	if len(index) == 0 {
		index = append(index, w.lastUsedIndex.Add(1))
	}

	address := w.keyManager.Address(iotago.AddressImplicitAccountCreation, index...)
	//nolint:forcetypeassert
	return index[0], address.(*iotago.ImplicitAccountCreationAddress)
}

func (w *DockerWallet) KeyPair(indexes ...uint32) (ed25519.PrivateKey, ed25519.PublicKey) {
	return w.keyManager.KeyPair(indexes...)
}

func (w *DockerWallet) AddressSigner(indexes ...uint32) iotago.AddressSigner {
	return w.keyManager.AddressSigner(indexes...)
}

func (w *DockerWallet) AllotManaFromAccount(clt *nodeclient.Client, fromId iotago.AccountID, toId iotago.AccountID, manaToAllot iotago.Mana, inputId iotago.OutputID, issuerResp *api.IssuanceBlockHeaderResponse) *iotago.SignedTransaction {
	from := w.Account(fromId)
	to := w.Account(toId)
	input := w.Output(inputId)

	currentSlot := clt.LatestAPI().TimeProvider().SlotFromTime(time.Now())
	apiForSlot := clt.APIForSlot(currentSlot)

	basicOutput, ok := input.Output.(*iotago.BasicOutput)
	require.True(w.Testing, ok)

	// Subtract stored mana from source outputs to fund Allotment.
	outputBuilder := builder.NewBasicOutputBuilderFromPrevious(basicOutput)
	actualAllottedMana := manaToAllot
	if manaToAllot >= basicOutput.StoredMana() {
		actualAllottedMana = basicOutput.StoredMana()
		outputBuilder.Mana(0)
	} else {
		outputBuilder.Mana(basicOutput.StoredMana() - manaToAllot)
	}

	signedTx, err := builder.NewTransactionBuilder(apiForSlot).
		AddInput(&builder.TxInput{
			UnlockTarget: input.Address,
			InputID:      input.ID,
			Input:        input.Output,
		}).
		IncreaseAllotment(to.ID, actualAllottedMana).
		AddOutput(basicOutput).
		SetCreationSlot(currentSlot).
		AllotAllMana(currentSlot, from.ID, 0).
		Build(w.AddressSigner(input.AddressIndex))
	require.NoError(w.Testing, err)

	allotmentOutputId := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)
	w.AddOutput(allotmentOutputId, &Output{
		ID:           allotmentOutputId,
		Output:       basicOutput,
		Address:      input.Address,
		AddressIndex: input.AddressIndex,
	})

	return signedTx
}

func (w *DockerWallet) AllotManaFromInput(clt *nodeclient.Client, toId iotago.AccountID, inputId iotago.OutputID, issuerResp *api.IssuanceBlockHeaderResponse) *iotago.SignedTransaction {
	to := w.Account(toId)
	input := w.Output(inputId)

	currentSlot := clt.LatestAPI().TimeProvider().SlotFromTime(time.Now())
	apiForSlot := clt.APIForSlot(currentSlot)

	basicOutput, err := builder.NewBasicOutputBuilder(input.Address, input.Output.BaseTokenAmount()).Build()
	require.NoError(w.Testing, err)

	signedTx, err := builder.NewTransactionBuilder(apiForSlot).
		AddInput(&builder.TxInput{
			UnlockTarget: input.Address,
			InputID:      input.ID,
			Input:        input.Output,
		}).
		AddOutput(basicOutput).
		AllotAllMana(currentSlot, to.ID, 0).
		SetCreationSlot(currentSlot).
		Build(w.AddressSigner(input.AddressIndex))

	delegationOutputId := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)
	w.AddOutput(delegationOutputId, &Output{
		ID:           delegationOutputId,
		Output:       basicOutput,
		Address:      input.Address,
		AddressIndex: input.AddressIndex,
	})

	return signedTx
}

func (w *DockerWallet) TransitionImplicitAccountToAccountOutput(clt *nodeclient.Client, inputId iotago.OutputID, issuerResp *api.IssuanceBlockHeaderResponse, opts ...options.Option[builder.AccountOutputBuilder]) (*Account, *iotago.SignedTransaction) {
	input := w.Output(inputId)

	accountID := iotago.AccountIDFromOutputID(input.ID)
	accountAddress, ok := accountID.ToAddress().(*iotago.AccountAddress)
	require.True(w.Testing, ok)

	currentSlot := clt.LatestAPI().TimeProvider().SlotFromTime(time.Now())
	apiForSlot := clt.APIForSlot(currentSlot)

	// transition to a full account with new Ed25519 address and staking feature
	accEd25519AddrIndex, accEd25519Addr := w.Address()
	accPrivateKey, _ := w.KeyPair(accEd25519AddrIndex)
	accBlockIssuerKey := iotago.Ed25519PublicKeyHashBlockIssuerKeyFromPublicKey(hiveEd25519.PublicKey(accPrivateKey.Public().(ed25519.PublicKey)))
	accountOutput := options.Apply(builder.NewAccountOutputBuilder(accEd25519Addr, input.Output.BaseTokenAmount()),
		opts, func(b *builder.AccountOutputBuilder) {
			b.AccountID(accountID).
				BlockIssuer(iotago.NewBlockIssuerKeys(accBlockIssuerKey), iotago.MaxSlotIndex)
		}).MustBuild()

	signedTx, err := builder.NewTransactionBuilder(apiForSlot).
		AddInput(&builder.TxInput{
			UnlockTarget: input.Address,
			InputID:      input.ID,
			Input:        input.Output,
		}).
		AddOutput(accountOutput).
		SetCreationSlot(currentSlot).
		AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: lo.Return1(issuerResp.LatestCommitment.ID())}).
		AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: accountID}).
		AllotAllMana(currentSlot, accountID, 0).
		Build(w.AddressSigner(input.AddressIndex))
	require.NoError(w.Testing, err)

	accountInfo := &Account{
		ID:           accountID,
		Address:      accountAddress,
		AddressIndex: accEd25519AddrIndex,
		Output:       accountOutput,
		OutputID:     iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0),
	}

	return accountInfo, signedTx
}

func (w *DockerWallet) CreateDelegationFromInput(clt *nodeclient.Client, issuerId iotago.AccountID, validator *Node, inputId iotago.OutputID, issuerResp *api.IssuanceBlockHeaderResponse) *iotago.SignedTransaction {
	input := w.Output(inputId)

	_, validatorAccountAddr, err := iotago.ParseBech32(validator.AccountAddressBech32)
	require.NoError(w.Testing, err)

	currentSlot := clt.LatestAPI().TimeProvider().SlotFromTime(time.Now())
	apiForSlot := clt.APIForSlot(currentSlot)

	// construct delegation transaction
	delegationOutput := builder.NewDelegationOutputBuilder(validatorAccountAddr.(*iotago.AccountAddress), input.Address, input.Output.BaseTokenAmount()).
		StartEpoch(getDelegationStartEpoch(apiForSlot, issuerResp.LatestCommitment.Slot)).
		DelegatedAmount(input.Output.BaseTokenAmount()).MustBuild()

	signedTx, err := builder.NewTransactionBuilder(apiForSlot).
		AddInput(&builder.TxInput{
			UnlockTarget: input.Address,
			InputID:      input.ID,
			Input:        input.Output,
		}).
		AddOutput(delegationOutput).
		SetCreationSlot(currentSlot).
		AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: lo.Return1(issuerResp.LatestCommitment.ID())}).
		AllotAllMana(currentSlot, issuerId, 0).
		Build(w.AddressSigner(input.AddressIndex))
	require.NoError(w.Testing, err)

	delegationOutputId := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)
	w.AddOutput(delegationOutputId, &Output{
		ID:           delegationOutputId,
		Output:       delegationOutput,
		Address:      input.Address,
		AddressIndex: input.AddressIndex,
	})

	return signedTx
}

func (w *DockerWallet) CreateFoundryAndNativeTokensFromInput(clt *nodeclient.Client, issuerId iotago.AccountID, inputId iotago.OutputID, mintedAmount iotago.BaseToken, maxSupply iotago.BaseToken, issuerResp *api.IssuanceBlockHeaderResponse) *iotago.SignedTransaction {
	input := w.Output(inputId)

	issuer := w.Account(issuerId)
	currentSlot := clt.LatestAPI().TimeProvider().SlotFromTime(time.Now())
	apiForSlot := clt.APIForSlot(currentSlot)

	// increase foundry counter
	accTransitionOutput := builder.NewAccountOutputBuilderFromPrevious(issuer.Output).
		FoundriesToGenerate(1).MustBuild()

	// build foundry output
	foundryID, err := iotago.FoundryIDFromAddressAndSerialNumberAndTokenScheme(issuer.Address, accTransitionOutput.FoundryCounter, iotago.TokenSchemeSimple)
	require.NoError(w.Testing, err)
	tokenScheme := &iotago.SimpleTokenScheme{
		MintedTokens:  big.NewInt(int64(mintedAmount)),
		MaximumSupply: big.NewInt(int64(maxSupply)),
		MeltedTokens:  big.NewInt(0),
	}

	foundryOutput := builder.NewFoundryOutputBuilder(issuer.Address, input.Output.BaseTokenAmount(), accTransitionOutput.FoundryCounter, tokenScheme).
		NativeToken(&iotago.NativeTokenFeature{
			ID:     foundryID,
			Amount: big.NewInt(int64(mintedAmount)),
		}).MustBuild()

	signedTx, err := builder.NewTransactionBuilder(apiForSlot).
		AddInput(&builder.TxInput{
			UnlockTarget: input.Address,
			InputID:      input.ID,
			Input:        input.Output,
		}).
		AddInput(&builder.TxInput{
			UnlockTarget: issuer.Output.UnlockConditionSet().Address().Address,
			InputID:      issuer.OutputID,
			Input:        issuer.Output,
		}).
		AddOutput(accTransitionOutput).
		AddOutput(foundryOutput).
		SetCreationSlot(currentSlot).
		AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: issuerId}).
		AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: lo.Return1(issuerResp.LatestCommitment.ID())}).
		AllotAllMana(currentSlot, issuerId, 0).
		Build(w.AddressSigner(input.AddressIndex, issuer.AddressIndex))
	require.NoError(w.Testing, err)

	foundryOutputId := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 1)
	w.AddOutput(foundryOutputId, &Output{
		ID:      foundryOutputId,
		Output:  foundryOutput,
		Address: issuer.Address,
	})

	w.AddAccount(issuerId, &Account{
		ID:           issuerId,
		Address:      issuer.Address,
		AddressIndex: issuer.AddressIndex,
		Output:       signedTx.Transaction.Outputs[0].(*iotago.AccountOutput),
		OutputID:     iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0),
	})

	return signedTx
}

// TransitionFoundry transitions a FoundryOutput by increasing the native token amount on the output by one.
func (w *DockerWallet) TransitionFoundry(clt *nodeclient.Client, issuerId iotago.AccountID, inputId iotago.OutputID, issuerResp *api.IssuanceBlockHeaderResponse) *iotago.SignedTransaction {
	issuer := w.Account(issuerId)
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

	outputAccount := builder.NewAccountOutputBuilderFromPrevious(issuer.Output).
		MustBuild()

	currentSlot := clt.LatestAPI().TimeProvider().SlotFromTime(time.Now())
	apiForSlot := clt.APIForSlot(currentSlot)

	signedTx, err := builder.NewTransactionBuilder(apiForSlot).
		AddInput(&builder.TxInput{
			UnlockTarget: input.Address,
			InputID:      input.ID,
			Input:        input.Output,
		}).
		AddInput(&builder.TxInput{
			UnlockTarget: issuer.Output.UnlockConditionSet().Address().Address,
			InputID:      issuer.OutputID,
			Input:        issuer.Output,
		}).
		AddOutput(outputAccount).
		AddOutput(outputFoundry).
		SetCreationSlot(currentSlot).
		AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: issuer.ID}).
		AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: lo.Return1(issuerResp.LatestCommitment.ID())}).
		AllotAllMana(currentSlot, issuerId, 0).
		Build(w.AddressSigner(input.AddressIndex, issuer.AddressIndex))
	require.NoError(w.Testing, err)

	foundryOutputId := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 1)
	w.AddOutput(foundryOutputId, &Output{
		ID:      foundryOutputId,
		Output:  outputFoundry,
		Address: issuer.Address,
	})

	w.AddAccount(issuerId, &Account{
		ID:           issuerId,
		Address:      issuer.Address,
		AddressIndex: issuer.AddressIndex,
		Output:       signedTx.Transaction.Outputs[0].(*iotago.AccountOutput),
		OutputID:     iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0),
	})

	return signedTx
}

func (w *DockerWallet) CreateNFTFromInput(clt *nodeclient.Client, issuerId iotago.AccountID, inputId iotago.OutputID, issuerResp *api.IssuanceBlockHeaderResponse, opts ...options.Option[builder.NFTOutputBuilder]) *iotago.SignedTransaction {
	input := w.Output(inputId)

	currentSlot := clt.LatestAPI().TimeProvider().SlotFromTime(time.Now())
	apiForSlot := clt.APIForSlot(currentSlot)

	nftAddressIndex, nftAddress := w.Address()
	nftOutputBuilder := builder.NewNFTOutputBuilder(nftAddress, input.Output.BaseTokenAmount())
	options.Apply(nftOutputBuilder, opts)
	nftOutput := nftOutputBuilder.MustBuild()

	signedTx, err := builder.NewTransactionBuilder(apiForSlot).
		AddInput(&builder.TxInput{
			UnlockTarget: input.Address,
			InputID:      input.ID,
			Input:        input.Output,
		}).
		AddOutput(nftOutput).
		SetCreationSlot(currentSlot).
		AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: issuerId}).
		AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: lo.Return1(issuerResp.LatestCommitment.ID())}).
		AllotAllMana(currentSlot, issuerId, 0).
		Build(w.AddressSigner(input.AddressIndex))
	require.NoError(w.Testing, err)

	nftOutputId := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)
	w.AddOutput(nftOutputId, &Output{
		ID:           nftOutputId,
		Output:       nftOutput,
		Address:      nftAddress,
		AddressIndex: nftAddressIndex,
	})

	return signedTx
}
