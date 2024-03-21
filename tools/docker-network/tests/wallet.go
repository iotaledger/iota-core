//go:build dockertests

package tests

import (
	"crypto/ed25519"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	hiveEd25519 "github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/nodeclient"
	"github.com/iotaledger/iota.go/v4/wallet"
)

// DockerWallet holds a keyManager, created outputs and accounts details.
type DockerWallet struct {
	Testing *testing.T

	// a map of clients for each node in the network
	Clients map[string]*nodeclient.Client

	keyManager *wallet.KeyManager

	lastUsedIndex atomic.Uint32

	outputs     map[iotago.OutputID]*mock.OutputData
	outputsLock sync.RWMutex

	accounts     map[iotago.AccountID]*mock.AccountData
	accountsLock sync.RWMutex
}

func NewDockerWallet(t *testing.T) *DockerWallet {
	return &DockerWallet{
		Testing:    t,
		outputs:    make(map[iotago.OutputID]*mock.OutputData),
		accounts:   make(map[iotago.AccountID]*mock.AccountData),
		Clients:    make(map[string]*nodeclient.Client),
		keyManager: lo.PanicOnErr(wallet.NewKeyManagerFromRandom(wallet.DefaultIOTAPath)),
	}
}

func (w *DockerWallet) DefaultClient() *nodeclient.Client {
	return w.Clients["V1"]
}

func (w *DockerWallet) AddOutput(outputID iotago.OutputID, output *mock.OutputData) {
	w.outputsLock.Lock()
	defer w.outputsLock.Unlock()

	w.outputs[outputID] = output
}

func (w *DockerWallet) AddAccount(accountID iotago.AccountID, data *mock.AccountData) {
	w.accountsLock.Lock()
	defer w.accountsLock.Unlock()

	w.accounts[accountID] = data
}

func (w *DockerWallet) Output(outputName iotago.OutputID) *mock.OutputData {
	w.outputsLock.RLock()
	defer w.outputsLock.RUnlock()

	output, exists := w.outputs[outputName]
	if !exists {
		panic(ierrors.Errorf("output %s not registered in wallet", outputName))
	}

	return output
}

func (w *DockerWallet) Account(accountID iotago.AccountID) *mock.AccountData {
	w.accountsLock.RLock()
	defer w.accountsLock.RUnlock()

	acc, exists := w.accounts[accountID]
	if !exists {
		panic(ierrors.Errorf("account %s not registered in wallet", accountID.ToHex()))
	}

	return acc
}

func (w *DockerWallet) Accounts(accountIds ...iotago.AccountID) []*mock.AccountData {
	w.accountsLock.RLock()
	defer w.accountsLock.RUnlock()

	accounts := make([]*mock.AccountData, 0)
	if len(accountIds) == 0 {
		for _, acc := range w.accounts {
			accounts = append(accounts, acc)
		}
	}

	for _, id := range accountIds {
		acc, exists := w.accounts[id]
		if !exists {
			panic(ierrors.Errorf("account %s not registered in wallet", id.ToHex()))
		}
		accounts = append(accounts, acc)
	}

	return accounts
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

func (w *DockerWallet) AllotManaFromAccount(fromID iotago.AccountID, toID iotago.AccountID, manaToAllot iotago.Mana, inputID iotago.OutputID) *iotago.SignedTransaction {
	from := w.Account(fromID)
	to := w.Account(toID)
	input := w.Output(inputID)

	currentSlot := w.DefaultClient().LatestAPI().TimeProvider().CurrentSlot()
	apiForSlot := w.DefaultClient().APIForSlot(currentSlot)

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

	signedTx, err := builder.NewTransactionBuilder(apiForSlot, w.AddressSigner(input.AddressIndex)).
		AddInput(&builder.TxInput{
			UnlockTarget: input.Address,
			InputID:      input.ID,
			Input:        input.Output,
		}).
		IncreaseAllotment(to.ID, actualAllottedMana).
		AddOutput(basicOutput).
		SetCreationSlot(currentSlot).
		AllotAllMana(currentSlot, from.ID, 0).
		Build()
	require.NoError(w.Testing, err)

	allotmentOutputID := iotago.OutputIDFromTransactionIDAndIndex(signedTx.Transaction.MustID(), 0)
	w.AddOutput(allotmentOutputID, &mock.OutputData{
		ID:           allotmentOutputID,
		Output:       basicOutput,
		Address:      input.Address,
		AddressIndex: input.AddressIndex,
	})

	return signedTx
}

func (w *DockerWallet) AllotManaFromInput(toID iotago.AccountID, inputID iotago.OutputID) *iotago.SignedTransaction {
	to := w.Account(toID)
	input := w.Output(inputID)

	currentSlot := w.DefaultClient().LatestAPI().TimeProvider().CurrentSlot()
	apiForSlot := w.DefaultClient().APIForSlot(currentSlot)

	basicOutput, err := builder.NewBasicOutputBuilder(input.Address, input.Output.BaseTokenAmount()).Build()
	require.NoError(w.Testing, err)

	signedTx, err := builder.NewTransactionBuilder(apiForSlot, w.AddressSigner(input.AddressIndex)).
		AddInput(&builder.TxInput{
			UnlockTarget: input.Address,
			InputID:      input.ID,
			Input:        input.Output,
		}).
		AddOutput(basicOutput).
		AllotAllMana(currentSlot, to.ID, 0).
		SetCreationSlot(currentSlot).
		Build()
	require.NoError(w.Testing, err)

	delegationOutputID := iotago.OutputIDFromTransactionIDAndIndex(signedTx.Transaction.MustID(), 0)
	w.AddOutput(delegationOutputID, &mock.OutputData{
		ID:           delegationOutputID,
		Output:       basicOutput,
		Address:      input.Address,
		AddressIndex: input.AddressIndex,
	})

	return signedTx
}

func (w *DockerWallet) TransitionImplicitAccountToAccountOutput(inputID iotago.OutputID, issuerResp *api.IssuanceBlockHeaderResponse, opts ...options.Option[builder.AccountOutputBuilder]) (*mock.AccountData, *iotago.SignedTransaction) {
	input := w.Output(inputID)

	accountID := iotago.AccountIDFromOutputID(input.ID)
	accountAddress, ok := accountID.ToAddress().(*iotago.AccountAddress)
	require.True(w.Testing, ok)

	currentSlot := w.DefaultClient().LatestAPI().TimeProvider().CurrentSlot()
	apiForSlot := w.DefaultClient().APIForSlot(currentSlot)

	// transition to a full account with new Ed25519 address and staking feature
	accEd25519AddrIndex, accEd25519Addr := w.Address()
	accPrivateKey, _ := w.KeyPair(accEd25519AddrIndex)
	//nolint:forcetypeassert
	accBlockIssuerKey := iotago.Ed25519PublicKeyHashBlockIssuerKeyFromPublicKey(hiveEd25519.PublicKey(accPrivateKey.Public().(ed25519.PublicKey)))
	accountOutput := options.Apply(builder.NewAccountOutputBuilder(accEd25519Addr, input.Output.BaseTokenAmount()),
		opts, func(b *builder.AccountOutputBuilder) {
			b.AccountID(accountID).
				BlockIssuer(iotago.NewBlockIssuerKeys(accBlockIssuerKey), iotago.MaxSlotIndex)
		}).MustBuild()

	signedTx, err := builder.NewTransactionBuilder(apiForSlot, w.AddressSigner(input.AddressIndex)).
		AddInput(&builder.TxInput{
			UnlockTarget: input.Address,
			InputID:      input.ID,
			Input:        input.Output,
		}).
		AddOutput(accountOutput).
		SetCreationSlot(currentSlot).
		AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: lo.Return1(issuerResp.LatestCommitment.ID())}).
		AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: accountID}).
		AddTaggedDataPayload(&iotago.TaggedData{Tag: []byte("account")}).
		AllotAllMana(currentSlot, accountID, 0).
		Build()
	require.NoError(w.Testing, err)

	accountOutputID := iotago.OutputIDFromTransactionIDAndIndex(signedTx.Transaction.MustID(), 0)
	w.AddOutput(accountOutputID, &mock.OutputData{
		ID:           accountOutputID,
		Output:       accountOutput,
		Address:      accEd25519Addr,
		AddressIndex: accEd25519AddrIndex,
	})

	accountInfo := &mock.AccountData{
		ID:           accountID,
		Address:      accountAddress,
		AddressIndex: accEd25519AddrIndex,
		Output:       accountOutput,
		OutputID:     accountOutputID,
	}

	return accountInfo, signedTx
}

func (w *DockerWallet) CreateDelegationFromInput(issuerID iotago.AccountID, validatorAccountAddr *iotago.AccountAddress, inputID iotago.OutputID, issuerResp *api.IssuanceBlockHeaderResponse) *iotago.SignedTransaction {
	input := w.Output(inputID)

	currentSlot := w.DefaultClient().LatestAPI().TimeProvider().CurrentSlot()
	apiForSlot := w.DefaultClient().APIForSlot(currentSlot)

	// construct delegation transaction
	//nolint:forcetypeassert
	delegationOutput := builder.NewDelegationOutputBuilder(validatorAccountAddr, input.Address, input.Output.BaseTokenAmount()).
		StartEpoch(getDelegationStartEpoch(apiForSlot, issuerResp.LatestCommitment.Slot)).
		DelegatedAmount(input.Output.BaseTokenAmount()).MustBuild()
	signedTx, err := builder.NewTransactionBuilder(apiForSlot, w.AddressSigner(input.AddressIndex)).
		AddInput(&builder.TxInput{
			UnlockTarget: input.Address,
			InputID:      input.ID,
			Input:        input.Output,
		}).
		AddOutput(delegationOutput).
		SetCreationSlot(currentSlot).
		AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: lo.Return1(issuerResp.LatestCommitment.ID())}).
		AddTaggedDataPayload(&iotago.TaggedData{Tag: []byte("delegation")}).
		AllotAllMana(currentSlot, issuerID, 0).
		Build()
	require.NoError(w.Testing, err)

	delegationOutputID := iotago.OutputIDFromTransactionIDAndIndex(signedTx.Transaction.MustID(), 0)
	w.AddOutput(delegationOutputID, &mock.OutputData{
		ID:           delegationOutputID,
		Output:       delegationOutput,
		Address:      input.Address,
		AddressIndex: input.AddressIndex,
	})

	return signedTx
}

func (w *DockerWallet) CreateFoundryAndNativeTokensFromInput(issuerID iotago.AccountID, inputID iotago.OutputID, mintedAmount iotago.BaseToken, maxSupply iotago.BaseToken, issuerResp *api.IssuanceBlockHeaderResponse) *iotago.SignedTransaction {
	input := w.Output(inputID)

	issuer := w.Account(issuerID)
	currentSlot := w.DefaultClient().LatestAPI().TimeProvider().CurrentSlot()
	apiForSlot := w.DefaultClient().APIForSlot(currentSlot)

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

	signedTx, err := builder.NewTransactionBuilder(apiForSlot, w.AddressSigner(input.AddressIndex, issuer.AddressIndex)).
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
		AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: issuerID}).
		AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: lo.Return1(issuerResp.LatestCommitment.ID())}).
		AddTaggedDataPayload(&iotago.TaggedData{Tag: []byte("foundry")}).
		AllotAllMana(currentSlot, issuerID, 0).
		Build()
	require.NoError(w.Testing, err)

	foundryOutputID := iotago.OutputIDFromTransactionIDAndIndex(signedTx.Transaction.MustID(), 1)
	w.AddOutput(foundryOutputID, &mock.OutputData{
		ID:      foundryOutputID,
		Output:  foundryOutput,
		Address: issuer.Address,
	})

	//nolint:forcetypeassert
	w.AddAccount(issuerID, &mock.AccountData{
		ID:           issuerID,
		Address:      issuer.Address,
		AddressIndex: issuer.AddressIndex,
		Output:       signedTx.Transaction.Outputs[0].(*iotago.AccountOutput),
		OutputID:     iotago.OutputIDFromTransactionIDAndIndex(signedTx.Transaction.MustID(), 0),
	})

	return signedTx
}

// TransitionFoundry transitions a FoundryOutput by increasing the native token amount on the output by one.
func (w *DockerWallet) TransitionFoundry(issuerID iotago.AccountID, inputID iotago.OutputID, issuerResp *api.IssuanceBlockHeaderResponse) *iotago.SignedTransaction {
	issuer := w.Account(issuerID)
	input := w.Output(inputID)
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

	currentSlot := w.DefaultClient().LatestAPI().TimeProvider().CurrentSlot()
	apiForSlot := w.DefaultClient().APIForSlot(currentSlot)

	signedTx, err := builder.NewTransactionBuilder(apiForSlot, w.AddressSigner(input.AddressIndex, issuer.AddressIndex)).
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
		AddTaggedDataPayload(&iotago.TaggedData{Tag: []byte("foundry")}).
		AllotAllMana(currentSlot, issuerID, 0).
		Build()
	require.NoError(w.Testing, err)

	foundryOutputID := iotago.OutputIDFromTransactionIDAndIndex(signedTx.Transaction.MustID(), 1)
	w.AddOutput(foundryOutputID, &mock.OutputData{
		ID:      foundryOutputID,
		Output:  outputFoundry,
		Address: issuer.Address,
	})

	//nolint:forcetypeassert
	w.AddAccount(issuerID, &mock.AccountData{
		ID:           issuerID,
		Address:      issuer.Address,
		AddressIndex: issuer.AddressIndex,
		Output:       signedTx.Transaction.Outputs[0].(*iotago.AccountOutput),
		OutputID:     iotago.OutputIDFromTransactionIDAndIndex(signedTx.Transaction.MustID(), 0),
	})

	return signedTx
}

func (w *DockerWallet) CreateNFTFromInput(issuerID iotago.AccountID, inputID iotago.OutputID, issuerResp *api.IssuanceBlockHeaderResponse, opts ...options.Option[builder.NFTOutputBuilder]) *iotago.SignedTransaction {
	input := w.Output(inputID)

	currentSlot := w.DefaultClient().LatestAPI().TimeProvider().CurrentSlot()
	apiForSlot := w.DefaultClient().APIForSlot(currentSlot)

	nftAddressIndex, nftAddress := w.Address()
	nftOutputBuilder := builder.NewNFTOutputBuilder(nftAddress, input.Output.BaseTokenAmount())
	options.Apply(nftOutputBuilder, opts)
	nftOutput := nftOutputBuilder.MustBuild()

	signedTx, err := builder.NewTransactionBuilder(apiForSlot, w.AddressSigner(input.AddressIndex)).
		AddInput(&builder.TxInput{
			UnlockTarget: input.Address,
			InputID:      input.ID,
			Input:        input.Output,
		}).
		AddOutput(nftOutput).
		SetCreationSlot(currentSlot).
		AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: issuerID}).
		AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: lo.Return1(issuerResp.LatestCommitment.ID())}).
		AddTaggedDataPayload(&iotago.TaggedData{Tag: []byte("nft")}).
		AllotAllMana(currentSlot, issuerID, 0).
		Build()
	require.NoError(w.Testing, err)

	nftOutputID := iotago.OutputIDFromTransactionIDAndIndex(signedTx.Transaction.MustID(), 0)
	w.AddOutput(nftOutputID, &mock.OutputData{
		ID:           nftOutputID,
		Output:       nftOutput,
		Address:      nftAddress,
		AddressIndex: nftAddressIndex,
	})

	return signedTx
}

func (w *DockerWallet) CreateBasicOutputFromInput(input *mock.OutputData, issuerAccountID iotago.AccountID) *iotago.SignedTransaction {
	currentSlot := w.DefaultClient().LatestAPI().TimeProvider().SlotFromTime(time.Now())
	apiForSlot := w.DefaultClient().APIForSlot(currentSlot)
	_, ed25519Addr := w.Address()
	basicOutput := builder.NewBasicOutputBuilder(ed25519Addr, input.Output.BaseTokenAmount()).MustBuild()
	signedTx, err := builder.NewTransactionBuilder(apiForSlot, w.AddressSigner(input.AddressIndex)).
		AddInput(&builder.TxInput{
			UnlockTarget: input.Address,
			InputID:      input.ID,
			Input:        input.Output,
		}).
		AddOutput(basicOutput).
		SetCreationSlot(currentSlot).
		AllotAllMana(currentSlot, issuerAccountID, 0).
		AddTaggedDataPayload(&iotago.TaggedData{Tag: []byte("basic")}).
		Build()
	require.NoError(w.Testing, err)

	return signedTx
}
