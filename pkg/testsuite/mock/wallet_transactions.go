package mock

import (
	"fmt"
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

// Functionality for creating transactions in the mock wallet.

func (w *Wallet) CreateAccountFromInput(transactionName string, inputName string, recipientWallet *Wallet, creationSlot iotago.SlotIndex, opts ...options.Option[builder.AccountOutputBuilder]) *iotago.SignedTransaction {
	input := w.Output(inputName)

	accountOutput := options.Apply(builder.NewAccountOutputBuilder(recipientWallet.Address(), input.BaseTokenAmount()).
		Mana(input.StoredMana()),
		opts).MustBuild()

	outputStates := iotago.Outputs[iotago.Output]{accountOutput}

	// if amount was set by options, a remainder output needs to be created
	if accountOutput.Amount != input.BaseTokenAmount() {
		remainderOutput := &iotago.BasicOutput{
			Amount: input.BaseTokenAmount() - accountOutput.Amount,
			Mana:   input.StoredMana() - accountOutput.Mana,
			Conditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: recipientWallet.Address()},
			},
			Features: iotago.BasicOutputFeatures{},
		}
		outputStates = append(outputStates, remainderOutput)
	}

	signedTransaction := lo.PanicOnErr(w.createSignedTransactionWithOptions(
		transactionName,
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.Node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}),
		WithInputs(utxoledger.Outputs{input}),
		WithOutputs(outputStates),
		WithSlotCreated(creationSlot),
	))

	// register the outputs in the recipient wallet (so wallet doesn't have to scan for outputs on its addresses)
	recipientWallet.registerOutputs(transactionName, signedTransaction.Transaction)

	return signedTransaction
}

// CreateDelegationFromInput creates a new DelegationOutput with given options from an input. If the remainder Output
// is not created, then StoredMana from the input is not passed and can potentially be burned.
// In order not to burn it, it needs to be assigned manually in another output in the transaction.
func (w *Wallet) CreateDelegationFromInput(transactionName string, inputName string, creationSlot iotago.SlotIndex, opts ...options.Option[builder.DelegationOutputBuilder]) *iotago.SignedTransaction {
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
			Conditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: w.Address()},
			},
			Features: iotago.BasicOutputFeatures{},
		})
	}

	// create the signed transaction
	signedTransaction := lo.PanicOnErr(w.createSignedTransactionWithOptions(
		transactionName,
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.Node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}),
		WithInputs(utxoledger.Outputs{input}),
		WithOutputs(outputStates),
		WithSlotCreated(creationSlot),
	))

	return signedTransaction
}

// DelayedClaimingTransition transitions DelegationOutput into delayed claiming state by setting DelegationID and EndEpoch.
func (w *Wallet) DelayedClaimingTransition(transactionName string, inputName string, creationSlot iotago.SlotIndex, delegationEndEpoch iotago.EpochIndex) *iotago.SignedTransaction {
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

	signedTransaction := lo.PanicOnErr(w.createSignedTransactionWithOptions(
		transactionName,
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.Node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}),
		WithInputs(utxoledger.Outputs{input}),
		WithOutputs(iotago.Outputs[iotago.Output]{delegationOutput}),
		WithSlotCreated(creationSlot),
	))

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

	signedTransaction := lo.PanicOnErr(w.createSignedTransactionWithOptions(
		transactionName,
		WithAccountInput(input),
		WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: accountOutput.AccountID,
		}),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.Node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}),
		WithOutputs(iotago.Outputs[iotago.Output]{accountOutput}),
	))

	return signedTransaction
}

func (w *Wallet) DestroyAccount(transactionName string, inputName string, creationSlot iotago.SlotIndex) *iotago.SignedTransaction {
	input := w.Output(inputName)
	inputAccount, ok := input.Output().(*iotago.AccountOutput)
	if !ok {
		panic(fmt.Sprintf("output with alias %s is not *iotago.AccountOutput", inputName))
	}

	destructionOutputs := iotago.Outputs[iotago.Output]{&iotago.BasicOutput{
		Amount: input.BaseTokenAmount(),
		Mana:   input.StoredMana(),
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: w.Address()},
		},
		Features: iotago.BasicOutputFeatures{},
	}}

	signedTransaction := lo.PanicOnErr(w.createSignedTransactionWithOptions(
		transactionName,
		WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: inputAccount.AccountID,
		}),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.Node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}),
		WithAccountInput(input),
		WithOutputs(destructionOutputs),
		WithSlotCreated(creationSlot),
	))

	return signedTransaction
}

// CreateImplicitAccountFromInput creates an implicit account output.
func (w *Wallet) CreateImplicitAccountFromInput(transactionName string, inputName string, recipientWallet *Wallet) *iotago.SignedTransaction {
	input := w.Output(inputName)

	implicitAccountOutput := &iotago.BasicOutput{
		Amount: MinIssuerAccountAmount,
		Mana:   AccountConversionManaCost,
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: recipientWallet.ImplicitAccountCreationAddress()},
		},
		Features: iotago.BasicOutputFeatures{},
	}

	remainderBasicOutput := &iotago.BasicOutput{
		Amount: input.BaseTokenAmount() - MinIssuerAccountAmount,
		Mana:   input.StoredMana() - AccountConversionManaCost,
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: input.Output().UnlockConditionSet().Address().Address},
		},
		Features: iotago.BasicOutputFeatures{},
	}

	signedTransaction := lo.PanicOnErr(w.createSignedTransactionWithOptions(
		transactionName,
		WithInputs(utxoledger.Outputs{input}),
		WithOutputs(iotago.Outputs[iotago.Output]{implicitAccountOutput, remainderBasicOutput}),
	))

	// register the outputs in the recipient wallet (so wallet doesn't have to scan for outputs on its addresses)
	recipientWallet.registerOutputs(transactionName, signedTransaction.Transaction)

	// register the implicit account as a block issuer in the wallet
	implicitAccountID := iotago.AccountIDFromOutputID(recipientWallet.Output(fmt.Sprintf("%s:0", transactionName)).OutputID())
	recipientWallet.SetBlockIssuer(implicitAccountID)

	return signedTransaction
}

func (w *Wallet) TransitionImplicitAccountToAccountOutput(transactionName string, inputName string, creationSlot iotago.SlotIndex, opts ...options.Option[builder.AccountOutputBuilder]) *iotago.SignedTransaction {
	input := w.Output(inputName)
	implicitAccountID := iotago.AccountIDFromOutputID(input.OutputID())

	basicOutput, isBasic := input.Output().(*iotago.BasicOutput)
	if !isBasic {
		panic(fmt.Sprintf("output with alias %s is not *iotago.BasicOutput", inputName))
	}
	if basicOutput.UnlockConditionSet().Address().Address.Type() != iotago.AddressImplicitAccountCreation {
		panic(fmt.Sprintf("output with alias %s is not an implicit account", inputName))
	}

	accountOutput := options.Apply(builder.NewAccountOutputBuilder(w.Address(), MinIssuerAccountAmount).
		AccountID(iotago.AccountIDFromOutputID(input.OutputID())),
		opts).MustBuild()

	signedTransaction := lo.PanicOnErr(w.createSignedTransactionWithOptions(
		transactionName,
		WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: implicitAccountID,
		}),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.Node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}),
		WithInputs(utxoledger.Outputs{input}),
		WithOutputs(iotago.Outputs[iotago.Output]{accountOutput}),
		WithSlotCreated(creationSlot),
		func(txBuilder *builder.TransactionBuilder) {
			txBuilder.AllotAllMana(creationSlot, implicitAccountID)
		},
	))

	return signedTransaction
}

func (w *Wallet) CreateBasicOutputsEquallyFromInputs(transactionName string, outputCount int, inputNames ...string) *iotago.SignedTransaction {
	inputStates := make([]*utxoledger.Output, 0, len(inputNames))
	totalInputAmounts := iotago.BaseToken(0)
	totalInputStoredMana := iotago.Mana(0)

	for _, inputName := range inputNames {
		output := w.Output(inputName)
		inputStates = append(inputStates, output)
		totalInputAmounts += output.BaseTokenAmount()
		totalInputStoredMana += output.StoredMana()
	}

	manaAmount := totalInputStoredMana / iotago.Mana(outputCount)
	remainderMana := totalInputStoredMana

	tokenAmount := totalInputAmounts / iotago.BaseToken(outputCount)
	remainderFunds := totalInputAmounts

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
			Conditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: w.Address()},
			},
			Features: iotago.BasicOutputFeatures{},
		})
	}

	signedTransaction := lo.PanicOnErr(w.createSignedTransactionWithOptions(
		transactionName,
		WithInputs(inputStates),
		WithOutputs(outputStates),
	))

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

	signedTransaction := lo.PanicOnErr(w.createSignedTransactionWithOptions(
		transactionName,
		WithAccountInput(input),
		WithBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{
			AccountID: accountOutput.AccountID,
		}),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.Node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}),
		WithOutputs(iotago.Outputs[iotago.Output]{accountOutput}),
	))

	return signedTransaction
}

func (w *Wallet) ClaimValidatorRewards(transactionName string, inputName string) *iotago.SignedTransaction {
	input := w.Output(inputName)
	inputAccount, ok := input.Output().(*iotago.AccountOutput)
	if !ok {
		panic(fmt.Sprintf("output with alias %s is not *iotago.AccountOutput", inputName))
	}

	rewardMana, _, _, err := w.Node.Protocol.MainEngineInstance().SybilProtection.ValidatorReward(
		inputAccount.AccountID,
		inputAccount.FeatureSet().Staking().StakedAmount,
		inputAccount.FeatureSet().Staking().StartEpoch,
		inputAccount.FeatureSet().Staking().EndEpoch,
	)
	if err != nil {
		panic(fmt.Sprintf("failed to calculate reward for output %s: %s", inputName, err))
	}

	signedTransaction := lo.PanicOnErr(w.createSignedTransactionWithOptions(
		transactionName,
		WithAccountInput(input),
		WithRewardInput(
			&iotago.RewardInput{Index: 1},
			rewardMana,
		),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.Node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}),
		// TODO: add account as output with extra Mana from rewards
	))

	return signedTransaction
}

func (w *Wallet) ClaimDelegatorRewards(transactionName string, inputName string) *iotago.SignedTransaction {
	input := w.Output(inputName)
	inputDelegation, ok := input.Output().(*iotago.DelegationOutput)
	if !ok {
		panic(fmt.Sprintf("output with alias %s is not *iotago.AccountOutput", inputName))
	}

	rewardMana, _, _, err := w.Node.Protocol.MainEngineInstance().SybilProtection.DelegatorReward(
		inputDelegation.ValidatorAddress.AccountID(),
		inputDelegation.DelegatedAmount,
		inputDelegation.StartEpoch,
		inputDelegation.EndEpoch,
	)
	if err != nil {
		panic(fmt.Sprintf("failed to calculate reward for output %s: %s", inputName, err))
	}

	signedTransaction := lo.PanicOnErr(w.createSignedTransactionWithOptions(
		transactionName,
		WithInputs(utxoledger.Outputs{input}),
		WithRewardInput(
			&iotago.RewardInput{Index: 1},
			rewardMana,
		),
		WithCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: w.Node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
		}),
	))

	// TODO: add basic output with extra Mana from rewards

	return signedTransaction
}

func (w *Wallet) createSignedTransactionWithOptions(transactionName string, opts ...options.Option[builder.TransactionBuilder]) (*iotago.SignedTransaction, error) {
	currentAPI := w.Node.Protocol.CommittedAPI()

	txBuilder := builder.NewTransactionBuilder(currentAPI)
	txBuilder.WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything()))
	// Always add a random payload to randomize transaction ID.
	randomPayload := tpkg.Rand12ByteArray()
	txBuilder.AddTaggedDataPayload(&iotago.TaggedData{Tag: randomPayload[:], Data: randomPayload[:]})

	addrSigner := w.AddressSigner()
	signedTransaction, err := options.Apply(txBuilder, opts).Build(addrSigner)

	// register the outputs in the wallet
	w.registerOutputs(transactionName, signedTransaction.Transaction)

	return signedTransaction, err
}

func (w *Wallet) registerOutputs(transactionName string, transaction *iotago.Transaction) {
	currentAPI := w.Node.Protocol.CommittedAPI()
	(lo.PanicOnErr(transaction.ID())).RegisterAlias(transactionName)
	w.transactions[transactionName] = transaction

	for outputID, output := range lo.PanicOnErr(transaction.OutputsSet()) {
		// register the output if it belongs to this wallet
		addressUC := output.UnlockConditionSet().Address()
		stateControllerUC := output.UnlockConditionSet().StateControllerAddress()
		if addressUC != nil && w.HasAddress(addressUC.Address) || stateControllerUC != nil && w.HasAddress(stateControllerUC.Address) {
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
