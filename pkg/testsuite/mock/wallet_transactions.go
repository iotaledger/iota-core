package mock

import (
	"fmt"
	"time"

	"github.com/iotaledger/hive.go/core/safemath"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
	"github.com/iotaledger/iota.go/v4/vm"
)

// Functionality for creating transactions in the mock wallet.

func (w *Wallet) CreateAccountFromInput(transactionName string, inputName string, recipientWallet *Wallet, opts ...options.Option[builder.AccountOutputBuilder]) *iotago.SignedTransaction {
	input := w.Output(inputName)

	accountOutput := options.Apply(builder.NewAccountOutputBuilder(recipientWallet.Address(), input.BaseTokenAmount()),
		opts).MustBuild()

	outputStates := iotago.Outputs[iotago.Output]{accountOutput}

	// if amount was set by options, a remainder output needs to be created
	remainderBaseToken := lo.PanicOnErr(safemath.SafeSub(input.BaseTokenAmount(), accountOutput.Amount))
	remainderMana := lo.PanicOnErr(safemath.SafeSub(input.StoredMana(), accountOutput.Mana))

	if accountOutput.Amount != input.BaseTokenAmount() {
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
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.Node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}),
		WithInputs(utxoledger.Outputs{input}),
		WithOutputs(outputStates),
	)

	// register the outputs in the recipient wallet (so wallet doesn't have to scan for outputs on its addresses)
	recipientWallet.registerOutputs(transactionName, signedTransaction.Transaction)

	return signedTransaction
}

// CreateDelegationFromInput creates a new DelegationOutput with given options from an input. If the remainder Output
// is not created, then StoredMana from the input is not passed and can potentially be burned.
// In order not to burn it, it needs to be assigned manually in another output in the transaction.
func (w *Wallet) CreateDelegationFromInput(transactionName string, inputName string, opts ...options.Option[builder.DelegationOutputBuilder]) *iotago.SignedTransaction {
	input := w.Output(inputName)

	delegationOutput := options.Apply(builder.NewDelegationOutputBuilder(&iotago.AccountAddress{}, w.Address(), input.BaseTokenAmount()).
		DelegatedAmount(input.BaseTokenAmount()),
		opts).MustBuild()

	if delegationOutput.ValidatorAddress.AccountID() == iotago.EmptyAccountID ||
		delegationOutput.DelegatedAmount == 0 ||
		delegationOutput.StartEpoch == 0 {
		panic(fmt.Sprintf("delegation output created incorrectly %+v", delegationOutput))
	}

	outputStates := iotago.Outputs[iotago.Output]{delegationOutput}

	// if options set an Amount, a remainder output needs to be created
	if delegationOutput.Amount != input.BaseTokenAmount() {
		outputStates = append(outputStates, &iotago.BasicOutput{
			Amount: input.BaseTokenAmount() - delegationOutput.Amount,
			Mana:   input.StoredMana(),
			UnlockConditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: w.Address()},
			},
			Features: iotago.BasicOutputFeatures{},
		})
	}

	// create the signed transaction
	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.Node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}),
		WithInputs(utxoledger.Outputs{input}),
		WithOutputs(outputStates),
		WithAllotAllManaToAccount(w.currentSlot, w.BlockIssuer.AccountID),
	)

	return signedTransaction
}

func (w *Wallet) DelegationStartFromSlot(slot iotago.SlotIndex) iotago.EpochIndex {
	latestCommitment := w.Node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment()
	apiForSlot := w.Node.Protocol.APIForSlot(slot)

	pastBoundedSlotIndex := latestCommitment.Slot() + apiForSlot.ProtocolParameters().MaxCommittableAge()
	pastBoundedEpochIndex := apiForSlot.TimeProvider().EpochFromSlot(pastBoundedSlotIndex)

	registrationSlot := w.registrationSlot(slot)

	if pastBoundedSlotIndex <= registrationSlot {
		return pastBoundedEpochIndex + 1
	}

	return pastBoundedEpochIndex + 2
}

func (w *Wallet) DelegationEndFromSlot(slot iotago.SlotIndex) iotago.EpochIndex {
	latestCommitment := w.Node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment()
	apiForSlot := w.Node.Protocol.APIForSlot(slot)

	futureBoundedSlotIndex := latestCommitment.Slot() + apiForSlot.ProtocolParameters().MinCommittableAge()
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
	apiForSlot := w.Node.Protocol.APIForSlot(slot)

	return apiForSlot.TimeProvider().EpochEnd(apiForSlot.TimeProvider().EpochFromSlot(slot)) - apiForSlot.ProtocolParameters().EpochNearingThreshold()
}

// DelayedClaimingTransition transitions DelegationOutput into delayed claiming state by setting DelegationID and EndEpoch.
func (w *Wallet) DelayedClaimingTransition(transactionName string, inputName string, delegationEndEpoch iotago.EpochIndex) *iotago.SignedTransaction {
	input := w.Output(inputName)
	if input.OutputType() != iotago.OutputDelegation {
		panic(ierrors.Errorf("%s is not a delegation output, cannot transition to delayed claiming state", inputName))
	}

	prevOutput, ok := input.Output().Clone().(*iotago.DelegationOutput)
	if !ok {
		panic(ierrors.Errorf("cloned output %s is not a delegation output, cannot transition to delayed claiming state", inputName))
	}

	delegationBuilder := builder.NewDelegationOutputBuilderFromPrevious(prevOutput).EndEpoch(delegationEndEpoch)
	if prevOutput.DelegationID == iotago.EmptyDelegationID() {
		delegationBuilder.DelegationID(iotago.DelegationIDFromOutputID(input.OutputID()))
	}

	delegationOutput := delegationBuilder.MustBuild()

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.Node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}),
		WithInputs(utxoledger.Outputs{input}),
		WithOutputs(iotago.Outputs[iotago.Output]{delegationOutput}),
	)

	return signedTransaction
}

func (w *Wallet) TransitionAccount(transactionName string, inputName string, opts ...options.Option[builder.AccountOutputBuilder]) *iotago.SignedTransaction {
	input, exists := w.outputs[inputName]
	if !exists {
		panic(fmt.Sprintf("account with alias %s does not exist", inputName))
	}

	accountOutput, ok := input.Output().Clone().(*iotago.AccountOutput)
	if !ok {
		panic(fmt.Sprintf("output with alias %s is not *iotago.AccountOutput", inputName))
	}

	accountBuilder := builder.NewAccountOutputBuilderFromPrevious(accountOutput)
	accountOutput = options.Apply(accountBuilder, opts).MustBuild()

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		WithAccountInput(input),
		WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: accountOutput.AccountID,
		}),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.Node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}),
		WithOutputs(iotago.Outputs[iotago.Output]{accountOutput}),
	)

	return signedTransaction
}

func (w *Wallet) DestroyAccount(transactionName string, inputName string) *iotago.SignedTransaction {
	input := w.Output(inputName)
	inputAccount, ok := input.Output().(*iotago.AccountOutput)
	if !ok {
		panic(fmt.Sprintf("output with alias %s is not *iotago.AccountOutput", inputName))
	}

	destructionOutputs := iotago.Outputs[iotago.Output]{&iotago.BasicOutput{
		Amount: input.BaseTokenAmount(),
		Mana:   input.StoredMana(),
		UnlockConditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: w.Address()},
		},
		Features: iotago.BasicOutputFeatures{},
	}}

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: inputAccount.AccountID,
		}),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.Node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}),
		WithAccountInput(input),
		WithOutputs(destructionOutputs),
	)

	return signedTransaction
}

// CreateImplicitAccountFromInput creates an implicit account output.
func (w *Wallet) CreateImplicitAccountFromInput(transactionName string, inputName string, recipientWallet *Wallet) *iotago.SignedTransaction {
	input := w.Output(inputName)

	implicitAccountOutput := &iotago.BasicOutput{
		Amount: MinIssuerAccountAmount(w.Node.Protocol.CommittedAPI().ProtocolParameters()),
		Mana:   AccountConversionManaCost(w.Node.Protocol.CommittedAPI().ProtocolParameters()),
		UnlockConditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: recipientWallet.ImplicitAccountCreationAddress()},
		},
		Features: iotago.BasicOutputFeatures{},
	}

	remainderBasicOutput := &iotago.BasicOutput{
		Amount: input.BaseTokenAmount() - MinIssuerAccountAmount(w.Node.Protocol.CommittedAPI().ProtocolParameters()),
		Mana:   input.StoredMana() - AccountConversionManaCost(w.Node.Protocol.CommittedAPI().ProtocolParameters()),
		UnlockConditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: input.Output().UnlockConditionSet().Address().Address},
		},
		Features: iotago.BasicOutputFeatures{},
	}

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		WithInputs(utxoledger.Outputs{input}),
		WithOutputs(iotago.Outputs[iotago.Output]{implicitAccountOutput, remainderBasicOutput}),
	)

	// register the outputs in the recipient wallet (so wallet doesn't have to scan for outputs on its addresses)
	recipientWallet.registerOutputs(transactionName, signedTransaction.Transaction)

	// register the implicit account as a block issuer in the wallet
	implicitAccountID := iotago.AccountIDFromOutputID(recipientWallet.Output(fmt.Sprintf("%s:0", transactionName)).OutputID())
	recipientWallet.SetBlockIssuer(implicitAccountID)

	return signedTransaction
}

func (w *Wallet) TransitionImplicitAccountToAccountOutput(transactionName string, inputName string, opts ...options.Option[builder.AccountOutputBuilder]) *iotago.SignedTransaction {
	input := w.Output(inputName)
	implicitAccountID := iotago.AccountIDFromOutputID(input.OutputID())

	basicOutput, isBasic := input.Output().(*iotago.BasicOutput)
	if !isBasic {
		panic(fmt.Sprintf("output with alias %s is not *iotago.BasicOutput", inputName))
	}
	if basicOutput.UnlockConditionSet().Address().Address.Type() != iotago.AddressImplicitAccountCreation {
		panic(fmt.Sprintf("output with alias %s is not an implicit account", inputName))
	}

	accountOutput := options.Apply(builder.NewAccountOutputBuilder(w.Address(), MinIssuerAccountAmount(w.Node.Protocol.CommittedAPI().ProtocolParameters())).
		AccountID(iotago.AccountIDFromOutputID(input.OutputID())),
		opts).MustBuild()

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: implicitAccountID,
		}),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.Node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}),
		WithInputs(utxoledger.Outputs{input}),
		WithOutputs(iotago.Outputs[iotago.Output]{accountOutput}),
		WithAllotAllManaToAccount(w.currentSlot, implicitAccountID),
	)

	return signedTransaction
}

func (w *Wallet) CreateBasicOutputsEquallyFromInput(transactionName string, outputCount int, inputName string) *iotago.SignedTransaction {
	apiForSlot := w.Node.Protocol.Engines.Main.Get().APIForSlot(w.currentSlot)
	manaDecayProvider := apiForSlot.ManaDecayProvider()
	storageScoreStructure := apiForSlot.StorageScoreStructure()

	inputState := w.Output(inputName)
	inputAmount := inputState.BaseTokenAmount()

	totalInputMana := lo.PanicOnErr(vm.TotalManaIn(manaDecayProvider, storageScoreStructure, w.currentSlot, vm.InputSet{inputState.OutputID(): inputState.Output()}, vm.RewardsInputSet{}))

	manaAmount := totalInputMana / iotago.Mana(outputCount)
	remainderMana := totalInputMana

	tokenAmount := inputAmount / iotago.BaseToken(outputCount)
	remainderFunds := inputAmount

	outputStates := make(iotago.Outputs[iotago.Output], 0, outputCount)
	for i := 0; i < outputCount; i++ {
		if i+1 == outputCount {
			tokenAmount = remainderFunds
			manaAmount = remainderMana
		}
		remainderFunds -= tokenAmount
		remainderMana -= manaAmount

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
		WithInputs(utxoledger.Outputs{inputState}),
		WithOutputs(outputStates),
	)

	return signedTransaction
}

func (w *Wallet) RemoveFeatureFromAccount(featureType iotago.FeatureType, transactionName string, inputName string) *iotago.SignedTransaction {
	input := w.Output(inputName)
	inputAccount, ok := input.Output().(*iotago.AccountOutput)
	if !ok {
		panic(fmt.Sprintf("output with alias %s is not *iotago.AccountOutput", inputName))
	}

	// clone the output but remove the feature of the specified type.
	accountOutput := builder.NewAccountOutputBuilderFromPrevious(inputAccount).RemoveFeature(featureType).MustBuild()

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		WithAccountInput(input),
		WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: accountOutput.AccountID,
		}),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.Node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}),
		WithOutputs(iotago.Outputs[iotago.Output]{accountOutput}),
	)

	return signedTransaction
}
func (w *Wallet) SendFundsToAccount(transactionName string, accountID iotago.AccountID, inputNames ...string) *iotago.SignedTransaction {
	inputStates := make([]*utxoledger.Output, 0, len(inputNames))
	totalInputAmounts := iotago.BaseToken(0)
	totalInputStoredMana := iotago.Mana(0)
	for _, inputName := range inputNames {
		output := w.Output(inputName)
		inputStates = append(inputStates, output)
		totalInputAmounts += output.BaseTokenAmount()
		totalInputStoredMana += output.StoredMana()
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
		WithInputs(inputStates),
		WithOutputs(iotago.Outputs[iotago.Output]{targetOutput}),
	)

	w.registerOutputs(transactionName, signedTransaction.Transaction)
	fmt.Println(lo.Keys(w.outputs))

	return signedTransaction
}

func (w *Wallet) SendFundsFromAccount(transactionName string, accountOutputName string, commitmentID iotago.CommitmentID, inputNames ...string) *iotago.SignedTransaction {
	inputStates := make([]*utxoledger.Output, 0, len(inputNames))
	totalInputAmounts := iotago.BaseToken(0)
	totalInputStoredMana := iotago.Mana(0)

	sourceOutput := w.AccountOutput(accountOutputName)
	inputStates = append(inputStates, sourceOutput)

	for _, inputName := range inputNames {
		output := w.Output(inputName)
		inputStates = append(inputStates, output)
		totalInputAmounts += output.BaseTokenAmount()
		totalInputStoredMana += output.StoredMana()
	}

	accountOutput, ok := sourceOutput.Output().(*iotago.AccountOutput)
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
		WithInputs(inputStates),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: commitmentID,
		}),
		WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: accountOutput.AccountID,
		}),
		WithOutputs(targetOutputs),
	)

	w.registerOutputs(transactionName, signedTransaction.Transaction)

	return signedTransaction
}

func (w *Wallet) ClaimValidatorRewards(transactionName string, inputName string) *iotago.SignedTransaction {
	input := w.Output(inputName)
	inputAccount, ok := input.Output().(*iotago.AccountOutput)
	if !ok {
		panic(fmt.Sprintf("output with alias %s is not *iotago.AccountOutput", inputName))
	}

	apiForSlot := w.Node.Protocol.APIForSlot(w.currentSlot)
	latestCommittedSlot := w.Node.Protocol.Chains.Main.Get().LatestCommitment.Get().Slot()
	futureBoundedSlotIndex := latestCommittedSlot + apiForSlot.ProtocolParameters().MinCommittableAge()
	claimingEpoch := apiForSlot.TimeProvider().EpochFromSlot(futureBoundedSlotIndex)

	rewardMana, _, _, err := w.Node.Protocol.Engines.Main.Get().SybilProtection.ValidatorReward(
		inputAccount.AccountID,
		inputAccount.FeatureSet().Staking(),
		claimingEpoch,
		apiForSlot.ProtocolParameters().RewardsParameters().RetentionPeriod,
	)
	if err != nil {
		panic(fmt.Sprintf("failed to calculate reward for output %s: %s", inputName, err))
	}

	potentialMana := w.PotentialMana(apiForSlot, input)
	storedMana := w.StoredMana(apiForSlot, input)

	accountOutput := builder.NewAccountOutputBuilderFromPrevious(inputAccount).
		RemoveFeature(iotago.FeatureStaking).
		Mana(potentialMana + storedMana + rewardMana).
		MustBuild()

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		WithAccountInput(input),
		WithRewardInput(
			&iotago.RewardInput{Index: 0},
			rewardMana,
		),
		WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: accountOutput.AccountID,
		}),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.Node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}),
		WithOutputs(iotago.Outputs[iotago.Output]{accountOutput}),
	)

	return signedTransaction
}

func (w *Wallet) AllotManaFromInputs(transactionName string, allotments iotago.Allotments, inputNames ...string) *iotago.SignedTransaction {
	inputStates := make([]*utxoledger.Output, 0, len(inputNames))
	outputStates := make(iotago.Outputs[iotago.Output], 0, len(inputNames))
	manaToAllot := iotago.Mana(0)
	for _, allotment := range allotments {
		manaToAllot += allotment.Mana
	}

	for _, inputName := range inputNames {
		output := w.Output(inputName)
		inputStates = append(inputStates, output)
		basicOutput, ok := output.Output().(*iotago.BasicOutput)
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
		WithAllotments(allotments),
		WithInputs(inputStates),
		WithOutputs(outputStates),
	)

	w.registerOutputs(transactionName, signedTransaction.Transaction)

	return signedTransaction
}

func (w *Wallet) ClaimDelegatorRewards(transactionName string, inputName string) *iotago.SignedTransaction {
	input := w.Output(inputName)
	inputDelegation, ok := input.Output().(*iotago.DelegationOutput)
	if !ok {
		panic(fmt.Sprintf("output with alias %s is not *iotago.AccountOutput", inputName))
	}

	apiForSlot := w.Node.Protocol.APIForSlot(w.currentSlot)
	futureBoundedSlotIndex := w.currentSlot + apiForSlot.ProtocolParameters().MinCommittableAge()
	claimingEpoch := apiForSlot.TimeProvider().EpochFromSlot(futureBoundedSlotIndex)

	delegationEnd := inputDelegation.EndEpoch
	// If Delegation ID is zeroed, the output is in delegating state, which means its End Epoch is not set and we must use the
	// "last epoch" for the rewards calculation.
	if inputDelegation.DelegationID.Empty() {
		delegationEnd = claimingEpoch - iotago.EpochIndex(1)
	}

	rewardMana, _, _, err := w.Node.Protocol.Engines.Main.Get().SybilProtection.DelegatorReward(
		inputDelegation.ValidatorAddress.AccountID(),
		inputDelegation.DelegatedAmount,
		inputDelegation.StartEpoch,
		delegationEnd,
		claimingEpoch,
		apiForSlot.ProtocolParameters().RewardsParameters().RetentionPeriod,
	)

	if err != nil {
		panic(fmt.Sprintf("failed to calculate reward for output %s: %s", inputName, err))
	}

	potentialMana := w.PotentialMana(apiForSlot, input)

	// Create Basic Output where the reward will be put.
	outputStates := iotago.Outputs[iotago.Output]{&iotago.BasicOutput{
		Amount: input.BaseTokenAmount(),
		Mana:   rewardMana + potentialMana,
		UnlockConditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: w.Address()},
		},
		Features: iotago.BasicOutputFeatures{},
	}}

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		WithInputs(utxoledger.Outputs{input}),
		WithRewardInput(
			&iotago.RewardInput{Index: 0},
			rewardMana,
		),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.Node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}),
		WithOutputs(outputStates),
	)

	return signedTransaction
}

// Computes the Potential Mana that the output generates until the current slot.
func (w *Wallet) PotentialMana(api iotago.API, input *utxoledger.Output) iotago.Mana {
	return lo.PanicOnErr(iotago.PotentialMana(api.ManaDecayProvider(), api.StorageScoreStructure(), input.Output(), input.SlotCreated(), w.currentSlot))
}

// Computes the decay on stored mana that the output holds until the current slot.
func (w *Wallet) StoredMana(api iotago.API, input *utxoledger.Output) iotago.Mana {
	return lo.PanicOnErr(api.ManaDecayProvider().DecayManaBySlots(input.StoredMana(), input.SlotCreated(), w.currentSlot))
}

func (w *Wallet) AllotManaToWallet(transactionName string, inputName string, recipientWallet *Wallet) *iotago.SignedTransaction {
	input := w.Output(inputName)

	signedTransaction := w.createSignedTransactionWithOptions(
		transactionName,
		WithInputs(utxoledger.Outputs{input}),
		WithAllotAllManaToAccount(w.currentSlot, recipientWallet.BlockIssuer.AccountID),
	)

	return signedTransaction
}

func (w *Wallet) CreateNFTFromInput(transactionName string, inputName string, opts ...options.Option[builder.NFTOutputBuilder]) *iotago.SignedTransaction {
	input := w.Output(inputName)

	nftOutputBuilder := builder.NewNFTOutputBuilder(w.Address(), input.BaseTokenAmount())
	options.Apply(nftOutputBuilder, opts)
	nftOutput := nftOutputBuilder.MustBuild()

	return w.createSignedTransactionWithOptions(
		transactionName,
		WithInputs(utxoledger.Outputs{input}),
		WithOutputs(iotago.Outputs[iotago.Output]{nftOutput}),
		WithAllotAllManaToAccount(w.currentSlot, w.BlockIssuer.AccountID),
	)
}

func (w *Wallet) TransitionNFTWithTransactionOpts(transactionName string, inputName string, opts ...options.Option[builder.TransactionBuilder]) *iotago.SignedTransaction {
	input, exists := w.outputs[inputName]
	if !exists {
		panic(fmt.Sprintf("NFT with alias %s does not exist", inputName))
	}

	nftOutput, ok := input.Output().Clone().(*iotago.NFTOutput)
	if !ok {
		panic(fmt.Sprintf("output with alias %s is not *iotago.NFTOutput", inputName))
	}

	builder.NewNFTOutputBuilderFromPrevious(nftOutput).NFTID(iotago.NFTIDFromOutputID(input.OutputID())).MustBuild()

	return w.createSignedTransactionWithOptions(
		transactionName,
		append(opts, WithInputs(utxoledger.Outputs{input}),
			WithOutputs(iotago.Outputs[iotago.Output]{nftOutput}),
			WithAllotAllManaToAccount(w.currentSlot, w.BlockIssuer.AccountID))...,
	)
}

func (w *Wallet) createSignedTransactionWithOptions(transactionName string, opts ...options.Option[builder.TransactionBuilder]) *iotago.SignedTransaction {
	currentAPI := w.Node.Protocol.CommittedAPI()

	txBuilder := builder.NewTransactionBuilder(currentAPI)
	// Use the wallet's current slot as creation slot by default.
	txBuilder.SetCreationSlot(w.currentSlot)
	// Set the transaction capabilities to be able to do anything.
	txBuilder.WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything()))
	// Always add a random payload to randomize transaction ID.
	randomPayload := tpkg.Rand12ByteArray()
	txBuilder.AddTaggedDataPayload(&iotago.TaggedData{Tag: randomPayload[:], Data: randomPayload[:]})

	signedTransaction := lo.PanicOnErr(options.Apply(txBuilder, opts).Build(w.AddressSigner()))

	// register the outputs in the wallet
	w.registerOutputs(transactionName, signedTransaction.Transaction)

	return signedTransaction
}

func (w *Wallet) registerOutputs(transactionName string, transaction *iotago.Transaction) {
	currentAPI := w.Node.Protocol.CommittedAPI()
	(lo.PanicOnErr(transaction.ID())).RegisterAlias(transactionName)
	w.transactions[transactionName] = transaction

	for outputID, output := range lo.PanicOnErr(transaction.OutputsSet()) {
		// register the output if it belongs to this wallet
		addressUC := output.UnlockConditionSet().Address()
		stateControllerUC := output.UnlockConditionSet().StateControllerAddress()
		if addressUC != nil && (w.HasAddress(addressUC.Address) || addressUC.Address.Type() == iotago.AddressAccount && addressUC.Address.String() == w.BlockIssuer.AccountID.ToAddress().String()) || stateControllerUC != nil && w.HasAddress(stateControllerUC.Address) {
			clonedOutput := output.Clone()
			actualOutputID := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(transaction.ID()), outputID.Index())
			if clonedOutput.Type() == iotago.OutputAccount {
				if accountOutput, ok := clonedOutput.(*iotago.AccountOutput); ok && accountOutput.AccountID == iotago.EmptyAccountID {
					accountOutput.AccountID = iotago.AccountIDFromOutputID(actualOutputID)
				}
			}
			w.outputs[fmt.Sprintf("%s:%d", transactionName, outputID.Index())] = utxoledger.CreateOutput(w.Node.Protocol, actualOutputID, iotago.EmptyBlockID, currentAPI.TimeProvider().SlotFromTime(time.Now()), clonedOutput, lo.PanicOnErr(iotago.OutputIDProofFromTransaction(transaction, outputID.Index())))
		}
	}
}
