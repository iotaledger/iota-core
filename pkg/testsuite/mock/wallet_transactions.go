package mock

import (
	"context"
	"fmt"
	"math/big"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/safemath"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
	"github.com/iotaledger/iota.go/v4/vm"
)

// Functionality for creating transactions in the mock wallet.

func (w *Wallet) CreateAccountFromInput(transactionName string, inputName string, recipientWallet *Wallet, opts ...options.Option[builder.AccountOutputBuilder]) *iotago.SignedTransaction {
	input := w.OutputData(inputName)

	accountOutput := options.Apply(builder.NewAccountOutputBuilder(recipientWallet.Address(), input.Output.BaseTokenAmount()),
		opts).MustBuild()

	outputStates := iotago.Outputs[iotago.Output]{accountOutput}

	// if amount was set by options, a remainder output needs to be created
	remainderBaseToken := lo.PanicOnErr(safemath.SafeSub(input.Output.BaseTokenAmount(), accountOutput.Amount))
	remainderMana := lo.PanicOnErr(safemath.SafeSub(input.Output.StoredMana(), accountOutput.Mana))

	if accountOutput.Amount != input.Output.BaseTokenAmount() {
		remainderOutput := &iotago.BasicOutput{
			Amount: remainderBaseToken,
			Mana:   remainderMana,
			UnlockConditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: recipientWallet.Address()},
			},
			Features: iotago.BasicOutputFeatures{},
		}
		outputStates = append(outputStates, remainderOutput)
	}

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.GetNewBlockIssuanceResponse().LatestCommitment.MustID(),
		}),
		WithInputs(input),
		WithOutputs(outputStates...),
	)

	// register the outputs in the recipient wallet (so wallet doesn't have to scan for outputs on its addresses)
	recipientWallet.registerOutputs(transactionName, signedTransaction.Transaction)

	return signedTransaction
}

func (w *Wallet) CreateAccountsFromInput(transactionName string, inputName string, outputCount int, opts ...options.Option[builder.AccountOutputBuilder]) *iotago.SignedTransaction {
	input := w.OutputData(inputName)

	outputBaseToken := input.Output.BaseTokenAmount() / iotago.BaseToken(outputCount)
	outputMana := input.Output.StoredMana() / iotago.Mana(outputCount)
	remainderBaseToken := input.Output.BaseTokenAmount() - outputBaseToken*iotago.BaseToken(outputCount)
	remainderMana := input.Output.StoredMana() - outputMana*iotago.Mana(outputCount)

	outputStates := make(iotago.Outputs[iotago.Output], 0, outputCount)
	for i := range outputCount {
		if i+1 == outputCount {
			outputBaseToken += remainderBaseToken
			outputMana += remainderMana
		}
		outputStates = append(outputStates, options.Apply(builder.NewAccountOutputBuilder(w.Address(), outputBaseToken), opts).MustBuild())
	}

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.GetNewBlockIssuanceResponse().LatestCommitment.MustID(),
		}),
		WithInputs(input),
		WithOutputs(outputStates...),
	)

	// register the outputs in the wallet
	w.registerOutputs(transactionName, signedTransaction.Transaction)

	return signedTransaction
}

// CreateDelegationFromInput creates a new DelegationOutput with given options from an input. If the remainder Output
// is not created, then StoredMana from the input is not passed and can potentially be burned.
// In order not to burn it, it needs to be assigned manually in another output in the transaction.
func (w *Wallet) CreateDelegationFromInput(transactionName string, input *OutputData, opts ...options.Option[builder.DelegationOutputBuilder]) *iotago.SignedTransaction {
	delegationOutput := options.Apply(builder.NewDelegationOutputBuilder(&iotago.AccountAddress{}, w.Address(), input.Output.BaseTokenAmount()).
		DelegatedAmount(input.Output.BaseTokenAmount()),
		opts).MustBuild()

	if delegationOutput.ValidatorAddress.AccountID() == iotago.EmptyAccountID ||
		delegationOutput.DelegatedAmount == 0 ||
		delegationOutput.StartEpoch == 0 {
		panic(fmt.Sprintf("delegation output created incorrectly %+v", delegationOutput))
	}

	outputStates := iotago.Outputs[iotago.Output]{delegationOutput}

	// if options set an Amount, a remainder output needs to be created
	if delegationOutput.Amount != input.Output.BaseTokenAmount() {
		outputStates = append(outputStates, &iotago.BasicOutput{
			Amount: input.Output.BaseTokenAmount() - delegationOutput.Amount,
			Mana:   input.Output.StoredMana(),
			UnlockConditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: w.Address()},
			},
			Features: iotago.BasicOutputFeatures{},
		})
	}

	// create the signed transaction
	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.GetNewBlockIssuanceResponse().LatestCommitment.MustID(),
		}),
		WithInputs(input),
		WithOutputs(outputStates...),
		WithAllotAllManaToAccount(w.CurrentSlot(), w.BlockIssuer.AccountData.ID),
		WithTaggedDataPayload(&iotago.TaggedData{Tag: []byte("delegation")}),
	)

	return signedTransaction
}

func (w *Wallet) DelegationStartFromSlot(slot, latestCommitmentSlot iotago.SlotIndex) iotago.EpochIndex {
	apiForSlot := w.Client.APIForSlot(slot)

	pastBoundedSlotIndex := latestCommitmentSlot + apiForSlot.ProtocolParameters().MaxCommittableAge()
	pastBoundedEpochIndex := apiForSlot.TimeProvider().EpochFromSlot(pastBoundedSlotIndex)

	registrationSlot := w.registrationSlot(slot)

	if pastBoundedSlotIndex <= registrationSlot {
		return pastBoundedEpochIndex + 1
	}

	return pastBoundedEpochIndex + 2
}

func (w *Wallet) DelegationEndFromSlot(slot, latestCommitmentSlot iotago.SlotIndex) iotago.EpochIndex {
	apiForSlot := w.Client.APIForSlot(slot)

	futureBoundedSlotIndex := latestCommitmentSlot + apiForSlot.ProtocolParameters().MinCommittableAge()
	futureBoundedEpochIndex := apiForSlot.TimeProvider().EpochFromSlot(futureBoundedSlotIndex)

	registrationSlot := w.registrationSlot(slot)

	if futureBoundedEpochIndex <= iotago.EpochIndex(registrationSlot) {
		return futureBoundedEpochIndex
	}

	return futureBoundedEpochIndex + 1
}

// Returns the registration slot in the epoch X corresponding to the given slot.
// This is the registration slot for epoch X+1.
func (w *Wallet) registrationSlot(slot iotago.SlotIndex) iotago.SlotIndex {
	apiForSlot := w.Client.APIForSlot(slot)

	return apiForSlot.TimeProvider().EpochEnd(apiForSlot.TimeProvider().EpochFromSlot(slot)) - apiForSlot.ProtocolParameters().EpochNearingThreshold()
}

// DelayedClaimingTransition transitions DelegationOutput into delayed claiming state by setting DelegationID and EndEpoch.
func (w *Wallet) DelayedClaimingTransition(transactionName string, inputName string, delegationEndEpoch iotago.EpochIndex) *iotago.SignedTransaction {
	input := w.OutputData(inputName)
	if input.Output.Type() != iotago.OutputDelegation {
		panic(ierrors.Errorf("%s is not a delegation output, cannot transition to delayed claiming state", inputName))
	}

	prevOutput, ok := input.Output.Clone().(*iotago.DelegationOutput)
	if !ok {
		panic(ierrors.Errorf("cloned output %s is not a delegation output, cannot transition to delayed claiming state", inputName))
	}

	delegationBuilder := builder.NewDelegationOutputBuilderFromPrevious(prevOutput).EndEpoch(delegationEndEpoch)
	if prevOutput.DelegationID == iotago.EmptyDelegationID() {
		delegationBuilder.DelegationID(iotago.DelegationIDFromOutputID(input.ID))
	}

	delegationOutput := delegationBuilder.MustBuild()

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.GetNewBlockIssuanceResponse().LatestCommitment.MustID(),
		}),
		WithInputs(input),
		WithOutputs(delegationOutput),
	)

	return signedTransaction
}

func (w *Wallet) TransitionAccount(transactionName string, inputName string, opts ...options.Option[builder.AccountOutputBuilder]) *iotago.SignedTransaction {
	input, exists := w.outputs[inputName]
	if !exists {
		panic(fmt.Sprintf("account with alias %s does not exist", inputName))
	}

	accountOutput, ok := input.Output.Clone().(*iotago.AccountOutput)
	if !ok {
		panic(fmt.Sprintf("output with alias %s is not *iotago.AccountOutput", inputName))
	}

	accountBuilder := builder.NewAccountOutputBuilderFromPrevious(accountOutput)
	accountOutput = options.Apply(accountBuilder, opts).MustBuild()

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithAccountInput(input),
		WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: accountOutput.AccountID,
		}),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.GetNewBlockIssuanceResponse().LatestCommitment.MustID(),
		}),
		WithOutputs(accountOutput),
	)

	return signedTransaction
}

func (w *Wallet) TransitionAccounts(transactionName string, inputNames []string, opts ...options.Option[builder.AccountOutputBuilder]) *iotago.SignedTransaction {
	inputs := make([]*OutputData, 0, len(inputNames))
	outputs := make(iotago.Outputs[iotago.Output], 0, len(inputNames))
	txOpts := []options.Option[builder.TransactionBuilder]{}
	for _, inputName := range inputNames {
		input := w.AccountOutputData(inputName)
		inputs = append(inputs, input)

		accountInput, ok := input.Output.Clone().(*iotago.AccountOutput)
		if !ok {
			panic(fmt.Sprintf("output with alias %s is not *iotago.AccountOutput", inputName))
		}

		accountBuilder := builder.NewAccountOutputBuilderFromPrevious(accountInput)
		accountOutput := options.Apply(accountBuilder, opts).MustBuild()
		outputs = append(outputs, accountOutput)
		txOpts = append(txOpts, WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: accountOutput.AccountID,
		}))
	}

	txOpts = append(txOpts,
		WithInputs(inputs...),
		WithOutputs(outputs...),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.GetNewBlockIssuanceResponse().LatestCommitment.MustID(),
		}),
	)

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		txOpts...,
	)

	return signedTransaction
}

func (w *Wallet) DestroyAccount(transactionName string, inputName string) *iotago.SignedTransaction {
	input := w.OutputData(inputName)
	inputAccount, ok := input.Output.(*iotago.AccountOutput)
	if !ok {
		panic(fmt.Sprintf("output with alias %s is not *iotago.AccountOutput", inputName))
	}

	destructionOutputs := iotago.Outputs[iotago.Output]{&iotago.BasicOutput{
		Amount: input.Output.BaseTokenAmount(),
		Mana:   input.Output.StoredMana(),
		UnlockConditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: w.Address()},
		},
		Features: iotago.BasicOutputFeatures{},
	}}

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: inputAccount.AccountID,
		}),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.GetNewBlockIssuanceResponse().LatestCommitment.MustID(),
		}),
		WithAccountInput(input),
		WithOutputs(destructionOutputs...),
	)

	return signedTransaction
}

// CreateImplicitAccountFromInput creates an implicit account output.
func (w *Wallet) CreateImplicitAccountFromInput(transactionName string, inputName string, recipientWallet *Wallet) *iotago.SignedTransaction {
	input := w.OutputData(inputName)

	implicitAccountOutput := &iotago.BasicOutput{
		Amount: MinIssuerAccountAmount(w.Client.CommittedAPI().ProtocolParameters()),
		Mana:   AccountConversionManaCost(w.Client.CommittedAPI().ProtocolParameters()),
		UnlockConditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: recipientWallet.ImplicitAccountCreationAddress()},
		},
		Features: iotago.BasicOutputFeatures{},
	}

	remainderBasicOutput := &iotago.BasicOutput{
		Amount: input.Output.BaseTokenAmount() - MinIssuerAccountAmount(w.Client.CommittedAPI().ProtocolParameters()),
		Mana:   input.Output.StoredMana() - AccountConversionManaCost(w.Client.CommittedAPI().ProtocolParameters()),
		UnlockConditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: input.Output.UnlockConditionSet().Address().Address},
		},
		Features: iotago.BasicOutputFeatures{},
	}

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithInputs(input),
		WithOutputs(implicitAccountOutput, remainderBasicOutput),
	)

	// register the outputs in the recipient wallet (so wallet doesn't have to scan for outputs on its addresses)
	recipientWallet.registerOutputs(transactionName, signedTransaction.Transaction)

	// register the implicit account as a block issuer in the wallet
	implicitAccountID := iotago.AccountIDFromOutputID(recipientWallet.OutputData(fmt.Sprintf("%s:0", transactionName)).ID)
	recipientWallet.SetBlockIssuer(&AccountData{ID: implicitAccountID})

	return signedTransaction
}

// CreateImplicitAccountAndBasicOutputFromInput creates an implicit account output and a remainder basic output from a basic output.
func (w *Wallet) CreateImplicitAccountAndBasicOutputFromInput(transactionName string, inputName string, recipientWallet *Wallet) *iotago.SignedTransaction {
	input := w.OutputData(inputName)

	implicitAccountOutput := &iotago.BasicOutput{
		Amount: MinIssuerAccountAmount(w.Client.CommittedAPI().ProtocolParameters()),
		Mana:   AccountConversionManaCost(w.Client.CommittedAPI().ProtocolParameters()),
		UnlockConditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: recipientWallet.ImplicitAccountCreationAddress()},
		},
		Features: iotago.BasicOutputFeatures{},
	}

	remainderBasicOutput := &iotago.BasicOutput{
		Amount: input.Output.BaseTokenAmount() - implicitAccountOutput.Amount,
		Mana:   input.Output.StoredMana() - implicitAccountOutput.Mana,
		UnlockConditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: recipientWallet.Address()},
		},
		Features: iotago.BasicOutputFeatures{},
	}

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithInputs(input),
		WithOutputs(implicitAccountOutput, remainderBasicOutput),
	)

	// register the outputs in the recipient wallet (so wallet doesn't have to scan for outputs on its addresses)
	recipientWallet.registerOutputs(transactionName, signedTransaction.Transaction)

	// register the implicit account as a block issuer in the wallet
	implicitAccountID := iotago.AccountIDFromOutputID(recipientWallet.OutputData(fmt.Sprintf("%s:0", transactionName)).ID)
	recipientWallet.SetBlockIssuer(&AccountData{ID: implicitAccountID})

	return signedTransaction
}

func (w *Wallet) TransitionImplicitAccountToAccountOutput(transactionName string, inputs []*OutputData, opts ...options.Option[builder.AccountOutputBuilder]) *iotago.SignedTransaction {
	var implicitAccountOutput *OutputData
	var baseTokenAmount iotago.BaseToken
	for _, input := range inputs {
		basicOutput, isBasic := input.Output.(*iotago.BasicOutput)
		if !isBasic {
			panic("input is not *iotago.BasicOutput")
		}
		if basicOutput.UnlockConditionSet().Address().Address.Type() == iotago.AddressImplicitAccountCreation {
			if implicitAccountOutput != nil {
				panic("multiple implicit account outputs found")
			}
			implicitAccountOutput = input
		}
		baseTokenAmount += input.Output.BaseTokenAmount()
	}
	if implicitAccountOutput == nil {
		panic("no implicit account output found")
	}
	implicitAccountID := iotago.AccountIDFromOutputID(implicitAccountOutput.ID)

	accountOutput := options.Apply(builder.NewAccountOutputBuilder(w.Address(), baseTokenAmount).
		AccountID(implicitAccountID),
		opts).MustBuild()

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: implicitAccountID,
		}),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.GetNewBlockIssuanceResponse().LatestCommitment.MustID(),
		}),
		WithInputs(inputs...),
		WithOutputs(accountOutput),
		WithAllotAllManaToAccount(w.CurrentSlot(), implicitAccountID),
		WithTaggedDataPayload(&iotago.TaggedData{Tag: []byte("account")}),
	)

	return signedTransaction
}

func (w *Wallet) CreateFoundryAndNativeTokensFromInput(input *OutputData, mintedAmount iotago.BaseToken, maxSupply iotago.BaseToken) *iotago.SignedTransaction {
	issuer := w.BlockIssuer.AccountData
	currentSlot := w.Client.LatestAPI().TimeProvider().CurrentSlot()
	apiForSlot := w.Client.APIForSlot(currentSlot)

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
		AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: issuer.ID}).
		AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: w.GetNewBlockIssuanceResponse().LatestCommitment.MustID()}).
		AddTaggedDataPayload(&iotago.TaggedData{Tag: []byte("foundry")}).
		AllotAllMana(currentSlot, issuer.ID, 0).
		Build()
	require.NoError(w.Testing, err)

	foundryOutputID := iotago.OutputIDFromTransactionIDAndIndex(signedTx.Transaction.MustID(), 1)
	w.AddOutput("", &OutputData{
		ID:      foundryOutputID,
		Output:  foundryOutput,
		Address: issuer.Address,
	})

	//nolint:forcetypeassert
	w.BlockIssuer.AccountData = &AccountData{
		ID:           issuer.ID,
		Address:      issuer.Address,
		AddressIndex: issuer.AddressIndex,
		Output:       signedTx.Transaction.Outputs[0].(*iotago.AccountOutput),
		OutputID:     iotago.OutputIDFromTransactionIDAndIndex(signedTx.Transaction.MustID(), 0),
	}

	return signedTx
}

func (w *Wallet) CreateFoundryAndNativeTokensOnOutputsFromInput(transactionName string, input *OutputData, accountName string, addressIndexes ...uint32) *iotago.SignedTransaction {
	nNativeTokens := len(addressIndexes)
	if nNativeTokens > iotago.MaxOutputsCount-2 {
		panic("too many address indexes provided")
	}
	outputStates := make(iotago.Outputs[iotago.Output], 0, nNativeTokens+2)

	inputAccountState := w.AccountOutputData(accountName)
	inputAccount, isAccount := inputAccountState.Output.(*iotago.AccountOutput)
	if !isAccount {
		panic(fmt.Sprintf("output with alias %s is not *iotago.AccountOutput", accountName))
	}
	accountAddr, isAccountAddress := inputAccount.AccountID.ToAddress().(*iotago.AccountAddress)
	if !isAccountAddress {
		panic(fmt.Sprintf("account address of output with alias %s is not *iotago.AccountAddress", accountName))
	}
	serialNumber := inputAccount.FoundryCounter + 1

	totalIn := input.Output.BaseTokenAmount()
	outputAmount := totalIn / iotago.BaseToken(nNativeTokens+1)
	remainder := totalIn - outputAmount*iotago.BaseToken(nNativeTokens+1)

	tokenScheme := &iotago.SimpleTokenScheme{
		MintedTokens:  big.NewInt(int64(nNativeTokens)),
		MeltedTokens:  big.NewInt(0),
		MaximumSupply: big.NewInt(1000),
	}
	nativeTokenFeature := &iotago.NativeTokenFeature{
		ID:     lo.PanicOnErr(iotago.FoundryIDFromAddressAndSerialNumberAndTokenScheme(accountAddr, serialNumber, iotago.TokenSchemeSimple)),
		Amount: big.NewInt(1),
	}

	outputStates = append(outputStates,
		builder.NewFoundryOutputBuilder(accountAddr, outputAmount+remainder, serialNumber, tokenScheme).
			MustBuild(),
	)

	accountOutput := builder.NewAccountOutputBuilderFromPrevious(inputAccount).FoundriesToGenerate(1).MustBuild()
	outputStates = append(outputStates, accountOutput)

	for _, index := range addressIndexes {
		outputStates = append(outputStates, &iotago.BasicOutput{
			Amount: outputAmount,
			Mana:   0,
			UnlockConditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: w.Address(index)},
			},
			Features: iotago.BasicOutputFeatures{
				nativeTokenFeature,
			},
		})
	}

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithInputs(input, inputAccountState),
		WithOutputs(outputStates...),
		WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: accountOutput.AccountID,
		}),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.GetNewBlockIssuanceResponse().LatestCommitment.MustID(),
		}),
	)

	return signedTransaction
}

// TransitionFoundry transitions a FoundryOutput by increasing the native token amount on the output by one.
func (w *Wallet) TransitionFoundry(transactionName string, foundryInput *OutputData, accountInput *OutputData) *iotago.SignedTransaction {
	inputFoundry, isFoundry := foundryInput.Output.(*iotago.FoundryOutput)
	if !isFoundry {
		panic("foundry input is not *iotago.FoundryOutput")
	}
	inputAccount, isAccount := foundryInput.Output.(*iotago.AccountOutput)
	if !isAccount {
		panic("account input is not *iotago.AccountOutput")
	}
	nativeTokenAmount := inputFoundry.FeatureSet().NativeToken().Amount
	previousTokenScheme, isSimple := inputFoundry.TokenScheme.(*iotago.SimpleTokenScheme)
	if !isSimple {
		panic("only simple token schemes supported")
	}
	tokenScheme := &iotago.SimpleTokenScheme{
		MaximumSupply: previousTokenScheme.MaximumSupply,
		MeltedTokens:  previousTokenScheme.MeltedTokens,
		MintedTokens:  previousTokenScheme.MintedTokens.Add(previousTokenScheme.MintedTokens, big.NewInt(1)),
	}

	if tokenScheme.MintedTokens.Cmp(tokenScheme.MaximumSupply) > 0 {
		panic("Can't transition foundry, maximum native token supply reached")
	}

	outputFoundry := builder.NewFoundryOutputBuilderFromPrevious(inputFoundry).
		NativeToken(&iotago.NativeTokenFeature{
			ID:     inputFoundry.MustFoundryID(),
			Amount: nativeTokenAmount.Add(nativeTokenAmount, big.NewInt(1)),
		}).
		TokenScheme(tokenScheme).
		MustBuild()
	outputAccount := builder.NewAccountOutputBuilderFromPrevious(inputAccount).
		MustBuild()

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithInputs(accountInput, foundryInput),
		WithOutputs(outputAccount, outputFoundry),
		WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: outputAccount.AccountID,
		}),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.GetNewBlockIssuanceResponse().LatestCommitment.MustID(),
		}),
	)

	return signedTransaction
}

func (w *Wallet) AllotManaFromBasicOutput(transactionName string, input *OutputData, manaToAllot iotago.Mana, accountIDs ...iotago.AccountID) *iotago.SignedTransaction {
	if _, isBasic := input.Output.(*iotago.BasicOutput); !isBasic {
		panic("input is not *iotago.BasicOutput")
	}

	apiForSlot := w.Client.APIForSlot(w.CurrentSlot())
	manaDecayProvider := apiForSlot.ManaDecayProvider()
	storageScoreStructure := apiForSlot.StorageScoreStructure()

	totalInputMana := lo.PanicOnErr(vm.TotalManaIn(manaDecayProvider, storageScoreStructure, w.CurrentSlot(), vm.InputSet{input.ID: input.Output}, vm.RewardsInputSet{}))
	if manaToAllot > totalInputMana {
		panic("not enough mana to allot")
	}
	manaPerOutput := manaToAllot / iotago.Mana(len(accountIDs))
	remainderMana := manaToAllot - manaPerOutput*iotago.Mana(len(accountIDs))
	output := &iotago.BasicOutput{
		Amount: input.Output.BaseTokenAmount(),
		Mana:   totalInputMana - manaToAllot,
		UnlockConditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: w.Address()},
		},
		Features: iotago.BasicOutputFeatures{},
	}

	var allotments iotago.Allotments
	for i, accountID := range accountIDs {
		if i+1 == len(accountIDs) {
			manaPerOutput += remainderMana
		}
		allotments = append(allotments, &iotago.Allotment{
			AccountID: accountID,
			Mana:      manaPerOutput,
		})
	}

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithInputs(input),
		WithOutputs(output),
		WithAllotments(allotments),
	)

	return signedTransaction
}

func (w *Wallet) CreateBasicOutputsEquallyFromInput(transactionName string, outputCount int, inputName string) *iotago.SignedTransaction {
	apiForSlot := w.Client.APIForSlot(w.CurrentSlot())
	manaDecayProvider := apiForSlot.ManaDecayProvider()
	storageScoreStructure := apiForSlot.StorageScoreStructure()

	inputState := w.OutputData(inputName)
	inputAmount := inputState.Output.BaseTokenAmount()

	totalInputMana := lo.PanicOnErr(vm.TotalManaIn(manaDecayProvider, storageScoreStructure, w.CurrentSlot(), vm.InputSet{inputState.ID: inputState.Output}, vm.RewardsInputSet{}))

	manaAmount := totalInputMana / iotago.Mana(outputCount)
	remainderMana := totalInputMana - manaAmount*iotago.Mana(outputCount)

	tokenAmount := inputAmount / iotago.BaseToken(outputCount)
	remainderFunds := inputAmount - tokenAmount*iotago.BaseToken(outputCount)

	outputStates := make(iotago.Outputs[iotago.Output], 0, outputCount)
	for i := range outputCount {
		if i+1 == outputCount {
			tokenAmount += remainderFunds
			manaAmount += remainderMana
		}

		outputStates = append(outputStates, &iotago.BasicOutput{
			Amount: tokenAmount,
			Mana:   manaAmount,
			UnlockConditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: w.Address()},
			},
			Features: iotago.BasicOutputFeatures{},
		})
	}

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithInputs(inputState),
		WithOutputs(outputStates...),
	)

	return signedTransaction
}

func (w *Wallet) CreateBasicOutputsAtAddressesFromInput(transactionName string, addressIndexes []uint32, inputName string) *iotago.SignedTransaction {
	outputCount := len(addressIndexes)
	apiForSlot := w.Client.APIForSlot(w.CurrentSlot())
	manaDecayProvider := apiForSlot.ManaDecayProvider()
	storageScoreStructure := apiForSlot.StorageScoreStructure()

	inputState := w.OutputData(inputName)
	inputAmount := inputState.Output.BaseTokenAmount()

	totalInputMana := lo.PanicOnErr(vm.TotalManaIn(manaDecayProvider, storageScoreStructure, w.CurrentSlot(), vm.InputSet{inputState.ID: inputState.Output}, vm.RewardsInputSet{}))

	manaAmount := totalInputMana / iotago.Mana(outputCount)
	remainderMana := totalInputMana - manaAmount*iotago.Mana(outputCount)

	tokenAmount := inputAmount / iotago.BaseToken(outputCount)
	remainderFunds := inputAmount - tokenAmount*iotago.BaseToken(outputCount)

	outputStates := make(iotago.Outputs[iotago.Output], 0, outputCount)
	for i, index := range addressIndexes {
		if i+1 == outputCount {
			tokenAmount += remainderFunds
			manaAmount += remainderMana
		}

		outputStates = append(outputStates, &iotago.BasicOutput{
			Amount: tokenAmount,
			Mana:   manaAmount,
			UnlockConditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: w.Address(index)},
			},
			Features: iotago.BasicOutputFeatures{},
		})
	}

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		addressIndexes,
		WithInputs(inputState),
		WithOutputs(outputStates...),
	)

	return signedTransaction
}

func (w *Wallet) CreateBasicOutputsEquallyFromInputs(transactionName string, inputNames []string, inputAddressIndexes []uint32, outputsCount int) *iotago.SignedTransaction {
	apiForSlot := w.Client.APIForSlot(w.CurrentSlot())
	manaDecayProvider := apiForSlot.ManaDecayProvider()
	storageScoreStructure := apiForSlot.StorageScoreStructure()

	inputStates := make([]*OutputData, 0, len(inputNames))
	var totalInputMana iotago.Mana
	var totalInputBaseToken iotago.BaseToken
	for _, inputName := range inputNames {
		inputState := w.OutputData(inputName)
		inputStates = append(inputStates, inputState)
		inputAmount := inputState.Output.BaseTokenAmount()

		totalInputMana += lo.PanicOnErr(vm.TotalManaIn(manaDecayProvider, storageScoreStructure, w.CurrentSlot(), vm.InputSet{inputState.ID: inputState.Output}, vm.RewardsInputSet{}))
		totalInputBaseToken += inputAmount
	}

	outputStates := make(iotago.Outputs[iotago.Output], 0, outputsCount)
	outputAmount := totalInputBaseToken / iotago.BaseToken(outputsCount)
	remainderAmount := totalInputBaseToken - outputAmount*iotago.BaseToken(outputsCount)
	outputMana := totalInputMana / iotago.Mana(outputsCount)
	remainderMana := totalInputMana - outputMana*iotago.Mana(outputsCount)

	for i := range outputsCount {
		if i+1 == outputsCount {
			outputAmount += remainderAmount
			outputMana += remainderMana
		}
		outputStates = append(outputStates, &iotago.BasicOutput{
			Amount: outputAmount,
			Mana:   outputMana,
			UnlockConditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: w.Address()},
			},
			Features: iotago.BasicOutputFeatures{},
		})
	}

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		inputAddressIndexes,
		WithInputs(inputStates...),
		WithOutputs(outputStates...),
	)

	return signedTransaction
}

func (w *Wallet) RemoveFeatureFromAccount(featureType iotago.FeatureType, transactionName string, inputName string) *iotago.SignedTransaction {
	input := w.OutputData(inputName)
	inputAccount, ok := input.Output.(*iotago.AccountOutput)
	if !ok {
		panic(fmt.Sprintf("output with alias %s is not *iotago.AccountOutput", inputName))
	}

	// clone the output but remove the feature of the specified type.
	accountOutput := builder.NewAccountOutputBuilderFromPrevious(inputAccount).RemoveFeature(featureType).MustBuild()

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithAccountInput(input),
		WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: accountOutput.AccountID,
		}),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.GetNewBlockIssuanceResponse().LatestCommitment.MustID(),
		}),
		WithOutputs(accountOutput),
	)

	return signedTransaction
}

func (w *Wallet) SendFundsToWallet(transactionName string, receiverWallet *Wallet, inputNames ...string) *iotago.SignedTransaction {
	inputStates := make([]*OutputData, 0, len(inputNames))
	totalInputAmounts := iotago.BaseToken(0)
	totalInputStoredMana := iotago.Mana(0)
	for _, inputName := range inputNames {
		output := w.OutputData(inputName)
		inputStates = append(inputStates, output)
		totalInputAmounts += output.Output.BaseTokenAmount()
		totalInputStoredMana += output.Output.StoredMana()
	}

	targetOutput := &iotago.BasicOutput{
		Amount: totalInputAmounts,
		Mana:   totalInputStoredMana,
		UnlockConditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: receiverWallet.Address()},
		},
		Features: iotago.BasicOutputFeatures{},
	}

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithInputs(inputStates...),
		WithOutputs(targetOutput),
	)

	receiverWallet.registerOutputs(transactionName, signedTransaction.Transaction)
	fmt.Println(lo.Keys(w.outputs))

	return signedTransaction
}

func (w *Wallet) SendFundsToAccount(transactionName string, accountID iotago.AccountID, inputNames ...string) *iotago.SignedTransaction {
	inputStates := make([]*OutputData, 0, len(inputNames))
	totalInputAmounts := iotago.BaseToken(0)
	totalInputStoredMana := iotago.Mana(0)
	for _, inputName := range inputNames {
		output := w.OutputData(inputName)
		inputStates = append(inputStates, output)
		totalInputAmounts += output.Output.BaseTokenAmount()
		totalInputStoredMana += output.Output.StoredMana()
	}

	targetOutput := &iotago.BasicOutput{
		Amount: totalInputAmounts,
		Mana:   totalInputStoredMana,
		UnlockConditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: accountID.ToAddress()},
		},
		Features: iotago.BasicOutputFeatures{},
	}

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithInputs(inputStates...),
		WithOutputs(targetOutput),
	)

	w.registerOutputs(transactionName, signedTransaction.Transaction)
	fmt.Println(lo.Keys(w.outputs))

	return signedTransaction
}

func (w *Wallet) SendFundsFromAccount(transactionName string, accountOutputName string, commitmentID iotago.CommitmentID, inputNames ...string) *iotago.SignedTransaction {
	inputStates := make([]*OutputData, 0, len(inputNames))
	totalInputAmounts := iotago.BaseToken(0)
	totalInputStoredMana := iotago.Mana(0)

	sourceOutput := w.AccountOutputData(accountOutputName)
	inputStates = append(inputStates, sourceOutput)

	for _, inputName := range inputNames {
		output := w.OutputData(inputName)
		inputStates = append(inputStates, output)
		totalInputAmounts += output.Output.BaseTokenAmount()
		totalInputStoredMana += output.Output.StoredMana()
	}

	accountOutput, ok := sourceOutput.Output.(*iotago.AccountOutput)
	if !ok {
		panic("accountOutputName is not an AccountOutput type")
	}

	targetOutputs := iotago.Outputs[iotago.Output]{accountOutput, &iotago.BasicOutput{
		Amount: totalInputAmounts,
		Mana:   totalInputStoredMana,
		UnlockConditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: w.Address()},
		},
		Features: iotago.BasicOutputFeatures{},
	}}
	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithInputs(inputStates...),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: commitmentID,
		}),
		WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: accountOutput.AccountID,
		}),
		WithOutputs(targetOutputs...),
	)

	w.registerOutputs(transactionName, signedTransaction.Transaction)

	return signedTransaction
}

func (w *Wallet) ClaimValidatorRewards(transactionName string, inputName string) *iotago.SignedTransaction {
	input := w.OutputData(inputName)
	inputAccount, ok := input.Output.(*iotago.AccountOutput)
	if !ok {
		panic(fmt.Sprintf("output with alias %s is not *iotago.AccountOutput", inputName))
	}

	apiForSlot := w.Client.APIForSlot(w.CurrentSlot())

	rewardResp, err := w.Client.Rewards(context.Background(), input.ID)
	require.NoError(w.Testing, err)
	potentialMana := w.PotentialMana(apiForSlot, input)
	storedMana := w.StoredMana(apiForSlot, input)

	accountOutput := builder.NewAccountOutputBuilderFromPrevious(inputAccount).
		RemoveFeature(iotago.FeatureStaking).
		Mana(potentialMana + storedMana + rewardResp.Rewards).
		MustBuild()

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithAccountInput(input),
		WithRewardInput(
			&iotago.RewardInput{Index: 0},
			rewardResp.Rewards,
		),
		WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: accountOutput.AccountID,
		}),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.GetNewBlockIssuanceResponse().LatestCommitment.MustID(),
		}),
		WithOutputs(accountOutput),
	)

	return signedTransaction
}

func (w *Wallet) AllotManaFromInputs(transactionName string, allotments iotago.Allotments, inputNames ...string) *iotago.SignedTransaction {
	inputStates := make([]*OutputData, 0, len(inputNames))
	outputStates := make(iotago.Outputs[iotago.Output], 0, len(inputNames))
	manaToAllot := iotago.Mana(0)
	for _, allotment := range allotments {
		manaToAllot += allotment.Mana
	}

	for _, inputName := range inputNames {
		output := w.OutputData(inputName)
		inputStates = append(inputStates, output)
		basicOutput, ok := output.Output.(*iotago.BasicOutput)
		if !ok {
			panic("allotting is only supported from BasicOutputs")
		}

		// Subtract stored mana from source outputs to fund Allotment.
		outputBuilder := builder.NewBasicOutputBuilderFromPrevious(basicOutput)
		if manaToAllot > 0 {
			if manaToAllot >= basicOutput.StoredMana() {
				outputBuilder.Mana(0)
			} else {
				outputBuilder.Mana(basicOutput.StoredMana() - manaToAllot)
			}
			manaToAllot -= basicOutput.StoredMana()
		}

		outputStates = append(outputStates, outputBuilder.MustBuild())
	}

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithAllotments(allotments),
		WithInputs(inputStates...),
		WithOutputs(outputStates...),
	)

	w.registerOutputs(transactionName, signedTransaction.Transaction)

	return signedTransaction
}

func (w *Wallet) ClaimDelegatorRewards(transactionName string, inputName string) *iotago.SignedTransaction {
	input := w.OutputData(inputName)

	apiForSlot := w.Client.APIForSlot(w.CurrentSlot())
	potentialMana := w.PotentialMana(apiForSlot, input)

	rewardsResp, err := w.Client.Rewards(context.Background(), input.ID)
	require.NoError(w.Testing, err)

	// Create Basic Output where the reward will be put.
	outputStates := iotago.Outputs[iotago.Output]{&iotago.BasicOutput{
		Amount: input.Output.BaseTokenAmount(),
		Mana:   rewardsResp.Rewards + potentialMana,
		UnlockConditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: w.Address()},
		},
		Features: iotago.BasicOutputFeatures{},
	}}

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithInputs(input),
		WithRewardInput(
			&iotago.RewardInput{Index: 0},
			rewardsResp.Rewards,
		),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.GetNewBlockIssuanceResponse().LatestCommitment.MustID(),
		}),
		WithOutputs(outputStates...),
	)

	return signedTransaction
}

// Computes the Potential Mana that the output generates until the current slot.
func (w *Wallet) PotentialMana(api iotago.API, input *OutputData) iotago.Mana {
	return lo.PanicOnErr(iotago.PotentialMana(api.ManaDecayProvider(), api.StorageScoreStructure(), input.Output, input.ID.CreationSlot(), w.CurrentSlot()))
}

// Computes the decay on stored mana that the output holds until the current slot.
func (w *Wallet) StoredMana(api iotago.API, input *OutputData) iotago.Mana {
	return lo.PanicOnErr(api.ManaDecayProvider().DecayManaBySlots(input.Output.StoredMana(), input.ID.CreationSlot(), w.CurrentSlot()))
}

func (w *Wallet) AllotManaToWallet(transactionName string, inputName string, recipientWallet *Wallet) *iotago.SignedTransaction {
	input := w.OutputData(inputName)

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithInputs(input),
		WithAllotAllManaToAccount(w.CurrentSlot(), recipientWallet.BlockIssuer.AccountData.ID),
	)

	return signedTransaction
}

func (w *Wallet) CreateNFTFromInput(transactionName string, input *OutputData, opts ...options.Option[builder.NFTOutputBuilder]) *iotago.SignedTransaction {
	nftOutputBuilder := builder.NewNFTOutputBuilder(w.Address(), input.Output.BaseTokenAmount())
	options.Apply(nftOutputBuilder, opts)
	nftOutput := nftOutputBuilder.MustBuild()

	return w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithInputs(input),
		WithOutputs(nftOutput),
		WithAllotAllManaToAccount(w.CurrentSlot(), w.BlockIssuer.AccountData.ID),
		WithTaggedDataPayload(&iotago.TaggedData{Tag: []byte("nft")}),
	)
}

//nolint:forcetypeassert
func (w *Wallet) CreateNativeTokenFromInput(transactionName string, inputName string, accountOutputName string, mintedAmount iotago.BaseToken, maxSupply iotago.BaseToken) *iotago.SignedTransaction {
	if mintedAmount > maxSupply {
		panic("minted amount cannot be greater than max supply")
	}

	input := w.OutputData(inputName)
	accountOutput := w.AccountOutputData(accountOutputName)

	// transition account output, increase foundry counter by 1, the amount of account stays the same
	accID := accountOutput.Output.(*iotago.AccountOutput).AccountID
	accAddr := accID.ToAddress().(*iotago.AccountAddress)
	accTransitionOutput := builder.NewAccountOutputBuilderFromPrevious(accountOutput.Output.(*iotago.AccountOutput)).
		FoundriesToGenerate(1).MustBuild()

	// build foundry output
	foundryID, _ := iotago.FoundryIDFromAddressAndSerialNumberAndTokenScheme(accAddr, accTransitionOutput.FoundryCounter, iotago.TokenSchemeSimple)
	tokenScheme := &iotago.SimpleTokenScheme{
		MintedTokens:  big.NewInt(int64(mintedAmount)),
		MaximumSupply: big.NewInt(int64(maxSupply)),
		MeltedTokens:  big.NewInt(0),
	}

	foundryOutput := builder.NewFoundryOutputBuilder(accAddr, input.Output.BaseTokenAmount(), accTransitionOutput.FoundryCounter, tokenScheme).
		NativeToken(&iotago.NativeTokenFeature{
			ID:     foundryID,
			Amount: big.NewInt(int64(mintedAmount)),
		}).MustBuild()

	return w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		WithInputs(accountOutput, input),
		WithOutputs(accTransitionOutput, foundryOutput),
		WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: accID,
		}),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.GetNewBlockIssuanceResponse().LatestCommitment.MustID(),
		}),
		WithAllotAllManaToAccount(w.CurrentSlot(), accID),
	)
}

func (w *Wallet) TransitionNFTWithTransactionOpts(transactionName string, inputName string, opts ...options.Option[builder.TransactionBuilder]) *iotago.SignedTransaction {
	input, exists := w.outputs[inputName]
	if !exists {
		panic(fmt.Sprintf("NFT with alias %s does not exist", inputName))
	}

	nftOutput, ok := input.Output.Clone().(*iotago.NFTOutput)
	if !ok {
		panic(fmt.Sprintf("output with alias %s is not *iotago.NFTOutput", inputName))
	}

	builder.NewNFTOutputBuilderFromPrevious(nftOutput).NFTID(iotago.NFTIDFromOutputID(input.ID)).MustBuild()

	return w.createSignedTransactionWithOptions(
		transactionName,
		[]uint32{0},
		append(opts,
			WithInputs(input),
			WithOutputs(nftOutput),
			WithAllotAllManaToAccount(w.CurrentSlot(), w.BlockIssuer.AccountData.ID))...,
	)
}

func (w *Wallet) createSignedTransactionWithOptions(transactionName string, addressIndexes []uint32, opts ...options.Option[builder.TransactionBuilder]) *iotago.SignedTransaction {
	currentAPI := w.Client.CommittedAPI()

	addressSigner := w.AddressSigner(addressIndexes...)

	txBuilder := builder.NewTransactionBuilder(currentAPI, addressSigner)
	// Use the wallet's current slot as creation slot by default.
	txBuilder.SetCreationSlot(w.CurrentSlot())
	// Set the transaction capabilities to be able to do anything.
	txBuilder.WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything()))
	// Always add a random payload to randomize transaction ID.
	randomPayload := tpkg.Rand12ByteArray()
	txBuilder.AddTaggedDataPayload(&iotago.TaggedData{Tag: randomPayload[:], Data: randomPayload[:]})

	signedTransaction := lo.PanicOnErr(options.Apply(txBuilder, opts).Build())

	// register the outputs in the wallet
	w.registerOutputs(transactionName, signedTransaction.Transaction, addressIndexes...)

	return signedTransaction
}

func (w *Wallet) registerOutputs(transactionName string, transaction *iotago.Transaction, addressIndexes ...uint32) {
	if len(addressIndexes) == 0 {
		addressIndexes = []uint32{0}
	}
	transaction.MustID().RegisterAlias(transactionName)
	w.transactions[transactionName] = transaction

	for outputID, output := range lo.PanicOnErr(transaction.OutputsSet()) {
		// register the output if it belongs to this wallet
		addressUC := output.UnlockConditionSet().Address()
		stateControllerUC := output.UnlockConditionSet().StateControllerAddress()
		immutableAccountUC := output.UnlockConditionSet().ImmutableAccount()
		for _, index := range addressIndexes {
			if addressUC != nil && (w.HasAddress(addressUC.Address, index) ||
				addressUC.Address.Type() == iotago.AddressAccount && addressUC.Address.String() == w.BlockIssuer.AccountData.ID.ToAddress().String()) ||
				immutableAccountUC != nil && immutableAccountUC.Address.AccountID() == w.BlockIssuer.AccountData.ID ||
				stateControllerUC != nil && w.HasAddress(stateControllerUC.Address, index) {
				clonedOutput := output.Clone()
				actualOutputID := iotago.OutputIDFromTransactionIDAndIndex(transaction.MustID(), outputID.Index())
				if clonedOutput.Type() == iotago.OutputAccount {
					accountOutput, ok := clonedOutput.(*iotago.AccountOutput)
					if ok && accountOutput.AccountID == iotago.EmptyAccountID {
						accountOutput.AccountID = iotago.AccountIDFromOutputID(actualOutputID)
					}
					//nolint:forcetypeassert
					w.accounts[accountOutput.AccountID] = &AccountData{
						ID:           accountOutput.AccountID,
						Address:      accountOutput.AccountID.ToAddress().(*iotago.AccountAddress),
						AddressIndex: index,
						Output:       clonedOutput.(*iotago.AccountOutput),
						OutputID:     actualOutputID,
					}
				}
				// register the output by both name and ID
				var address iotago.Address
				if addressUC != nil {
					address = addressUC.Address
				}
				w.outputs[fmt.Sprintf("%s:%d", transactionName, outputID.Index())] = &OutputData{
					ID:      actualOutputID,
					Output:  clonedOutput,
					Address: address,
				}
				w.outputsByID[actualOutputID] = &OutputData{
					ID:      actualOutputID,
					Output:  clonedOutput,
					Address: address,
				}

				break
			}
		}
	}
}

func (w *Wallet) CreateBasicOutputFromInput(input *OutputData) *iotago.SignedTransaction {
	currentSlot := w.CurrentSlot()
	apiForSlot := w.Client.APIForSlot(currentSlot)
	ed25519Addr := w.Address()
	basicOutput := builder.NewBasicOutputBuilder(ed25519Addr, input.Output.BaseTokenAmount()).MustBuild()
	signedTx, err := builder.NewTransactionBuilder(apiForSlot, w.AddressSigner(input.AddressIndex)).
		AddInput(&builder.TxInput{
			UnlockTarget: input.Address,
			InputID:      input.ID,
			Input:        input.Output,
		}).
		AddOutput(basicOutput).
		SetCreationSlot(currentSlot).
		AllotAllMana(currentSlot, w.BlockIssuer.AccountData.ID, 0).
		AddTaggedDataPayload(&iotago.TaggedData{Tag: []byte("basic")}).
		Build()
	require.NoError(w.Testing, err)

	return signedTx
}
