package testsuite

import (
	"fmt"
	"testing"
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type TransactionFramework struct {
	apiProvider iotago.APIProvider

	GenesisWallet      *mock.Wallet
	states             map[string]*utxoledger.Output
	signedTransactions map[string]*iotago.SignedTransaction
	transactions       map[string]*iotago.Transaction
}

func NewTransactionFramework(t *testing.T, node *mock.Node, genesisSeed []byte) *TransactionFramework {
	genesisWallet := mock.NewWallet(t, "genesis", node, genesisSeed)
	genesisWallet.AddBlockIssuer(iotago.EmptyAccountID)

	tf := &TransactionFramework{
		apiProvider:        node.Protocol,
		states:             make(map[string]*utxoledger.Output),
		signedTransactions: make(map[string]*iotago.SignedTransaction),
		transactions:       make(map[string]*iotago.Transaction),

		GenesisWallet: genesisWallet,
	}

	if err := node.Protocol.MainEngineInstance().Ledger.ForEachUnspentOutput(func(output *utxoledger.Output) bool {
		tf.states[fmt.Sprintf("Genesis:%d", output.OutputID().Index())] = output
		tf.GenesisWallet.AddOutput(fmt.Sprintf("Genesis:%d", output.OutputID().Index()), output)
		return true
	}); err != nil {
		panic(err)
	}

	if len(tf.states) == 0 {
		panic("no genesis outputs found")
	}

	return tf
}

func (t *TransactionFramework) RegisterTransaction(alias string, transaction *iotago.Transaction) {
	currentAPI := t.apiProvider.CommittedAPI()
	(lo.PanicOnErr(transaction.ID())).RegisterAlias(alias)

	t.transactions[alias] = transaction

	for outputID, output := range lo.PanicOnErr(transaction.OutputsSet()) {
		clonedOutput := output.Clone()
		actualOutputID := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(transaction.ID()), outputID.Index())
		if clonedOutput.Type() == iotago.OutputAccount {
			if accountOutput, ok := clonedOutput.(*iotago.AccountOutput); ok && accountOutput.AccountID == iotago.EmptyAccountID {
				accountOutput.AccountID = iotago.AccountIDFromOutputID(actualOutputID)
			}
		}

		t.states[fmt.Sprintf("%s:%d", alias, outputID.Index())] = utxoledger.CreateOutput(t.apiProvider, actualOutputID, iotago.EmptyBlockID, currentAPI.TimeProvider().SlotFromTime(time.Now()), clonedOutput, lo.PanicOnErr(iotago.OutputIDProofFromTransaction(transaction, outputID.Index())))
	}
}

func (t *TransactionFramework) RegisterSignedTransaction(alias string, signedTransaction *iotago.SignedTransaction) {
	(lo.PanicOnErr(signedTransaction.ID())).RegisterAlias(alias)

	t.signedTransactions[alias] = signedTransaction
}

func (t *TransactionFramework) CreateSignedTransactionWithOptions(alias string, signingWallet *mock.Wallet, opts ...options.Option[builder.TransactionBuilder]) (*iotago.SignedTransaction, error) {
	currentAPI := t.apiProvider.CommittedAPI()

	txBuilder := builder.NewTransactionBuilder(currentAPI)
	txBuilder.WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything()))
	// Always add a random payload to randomize transaction ID.
	randomPayload := tpkg.Rand12ByteArray()
	txBuilder.AddTaggedDataPayload(&iotago.TaggedData{Tag: randomPayload[:], Data: randomPayload[:]})

	signedTransaction, err := options.Apply(txBuilder, opts).Build(signingWallet.AddressSigner())
	if err == nil {
		t.RegisterSignedTransaction(alias, signedTransaction)
		t.RegisterTransaction(alias, signedTransaction.Transaction)
	}

	return signedTransaction, err
}

func (t *TransactionFramework) CreateSimpleTransaction(alias string, outputCount int, inputAliases ...string) (*iotago.SignedTransaction, error) {
	inputStates, outputStates := t.CreateBasicOutputsEquallyFromGenesis(outputCount, inputAliases...)

	return t.CreateSignedTransactionWithOptions(alias, t.GenesisWallet, WithInputs(inputStates), WithOutputs(outputStates))
}

func (t *TransactionFramework) CreateBasicOutputsEquallyFromGenesis(outputCount int, inputAliases ...string) (consumedInputs utxoledger.Outputs, outputs iotago.Outputs[iotago.Output]) {
	inputStates := make([]*utxoledger.Output, 0, len(inputAliases))
	totalInputAmounts := iotago.BaseToken(0)
	totalInputStoredMana := iotago.Mana(0)

	for _, inputAlias := range inputAliases {
		output := t.Output(inputAlias)
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
				&iotago.AddressUnlockCondition{Address: t.GenesisWallet.Address()},
			},
			Features: iotago.BasicOutputFeatures{},
		})
	}

	return inputStates, outputStates
}

func (t *TransactionFramework) CreateBasicOutputs(signingWallet *mock.Wallet, amountDistribution []iotago.BaseToken, manaDistribution []iotago.Mana, inputAliases ...string) (consumedInputs utxoledger.Outputs, outputs iotago.Outputs[iotago.Output]) {
	if len(amountDistribution) != len(manaDistribution) {
		panic("amount and mana distributions should have the same length")
	}

	inputStates := make([]*utxoledger.Output, 0, len(inputAliases))
	totalInputAmounts := iotago.BaseToken(0)
	totalInputStoredMana := iotago.Mana(0)

	for _, inputAlias := range inputAliases {
		output := t.Output(inputAlias)
		inputStates = append(inputStates, output)
		totalInputAmounts += output.BaseTokenAmount()
		totalInputStoredMana += output.StoredMana()
	}

	if lo.Sum(amountDistribution...) != totalInputAmounts {
		panic("amount on input and output side must be equal")
	}

	outputStates := make(iotago.Outputs[iotago.Output], 0, len(amountDistribution))
	for idx, outputAmount := range amountDistribution {
		outputStates = append(outputStates, &iotago.BasicOutput{
			Amount: outputAmount,
			Mana:   manaDistribution[idx],
			Conditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: signingWallet.Address()},
			},
			Features: iotago.BasicOutputFeatures{},
		})
	}

	return inputStates, outputStates
}


func (t *TransactionFramework) Output(alias string) *utxoledger.Output {
	output, exists := t.states[alias]
	if !exists {
		panic(ierrors.Errorf("output with given alias does not exist %s", alias))
	}

	return output
}

func (t *TransactionFramework) OutputID(alias string) iotago.OutputID {
	return t.Output(alias).OutputID()
}

func (t *TransactionFramework) SignedTransaction(alias string) *iotago.SignedTransaction {
	transaction, exists := t.signedTransactions[alias]
	if !exists {
		panic(ierrors.Errorf("transaction with given alias does not exist %s", alias))
	}

	return transaction
}

func (t *TransactionFramework) SignedTransactionID(alias string) iotago.SignedTransactionID {
	return lo.PanicOnErr(t.SignedTransaction(alias).ID())
}

func (t *TransactionFramework) SignedTransactions(aliases ...string) []*iotago.SignedTransaction {
	return lo.Map(aliases, t.SignedTransaction)
}

func (t *TransactionFramework) SignedTransactionIDs(aliases ...string) []iotago.SignedTransactionID {
	return lo.Map(aliases, t.SignedTransactionID)
}

func (t *TransactionFramework) Transaction(alias string) *iotago.Transaction {
	transaction, exists := t.transactions[alias]
	if !exists {
		panic(ierrors.Errorf("transaction with given alias does not exist %s", alias))
	}

	return transaction
}

func (t *TransactionFramework) TransactionID(alias string) iotago.TransactionID {
	return lo.PanicOnErr(t.Transaction(alias).ID())
}

func (t *TransactionFramework) Transactions(aliases ...string) []*iotago.Transaction {
	return lo.Map(aliases, t.Transaction)
}


// DelegationOutput options

func WithDelegatedAmount(delegatedAmount iotago.BaseToken) options.Option[builder.DelegationOutputBuilder] {
	return func(delegationBuilder *builder.DelegationOutputBuilder) {
		delegationBuilder.DelegatedAmount(delegatedAmount)
	}
}

func WithDelegatedValidatorAddress(validatorAddress *iotago.AccountAddress) options.Option[builder.DelegationOutputBuilder] {
	return func(delegationBuilder *builder.DelegationOutputBuilder) {
		delegationBuilder.ValidatorAddress(validatorAddress)
	}
}

func WithDelegationStartEpoch(startEpoch iotago.EpochIndex) options.Option[builder.DelegationOutputBuilder] {
	return func(delegationBuilder *builder.DelegationOutputBuilder) {
		delegationBuilder.StartEpoch(startEpoch)
	}
}

func WithDelegationEndEpoch(endEpoch iotago.EpochIndex) options.Option[builder.DelegationOutputBuilder] {
	return func(delegationBuilder *builder.DelegationOutputBuilder) {
		delegationBuilder.EndEpoch(endEpoch)
	}
}

func WithDelegationConditions(delegationConditions iotago.DelegationOutputUnlockConditions) options.Option[builder.DelegationOutputBuilder] {
	return func(delegationBuilder *builder.DelegationOutputBuilder) {
		delegationBuilder.Address(delegationConditions.MustSet().Address().Address)
	}
}

func WithDelegationAmount(amount iotago.BaseToken) options.Option[builder.DelegationOutputBuilder] {
	return func(delegationBuilder *builder.DelegationOutputBuilder) {
		delegationBuilder.Amount(amount)
	}
}

// BlockIssuer options

func WithBlockIssuerFeature(keys iotago.BlockIssuerKeys, expirySlot iotago.SlotIndex) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		accountBuilder.BlockIssuer(keys, expirySlot)
	}
}

func WithAddBlockIssuerKey(key iotago.BlockIssuerKey) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		transition := accountBuilder.GovernanceTransition()
		transition.BlockIssuerTransition().AddKeys(key)
	}
}

func WithBlockIssuerKeys(keys iotago.BlockIssuerKeys) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		transition := accountBuilder.GovernanceTransition()
		transition.BlockIssuerTransition().Keys(keys)
	}
}

func WithBlockIssuerExpirySlot(expirySlot iotago.SlotIndex) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		transition := accountBuilder.GovernanceTransition()
		transition.BlockIssuerTransition().ExpirySlot(expirySlot)
	}
}

func WithStakingFeature(amount iotago.BaseToken, fixedCost iotago.Mana, startEpoch iotago.EpochIndex, optEndEpoch ...iotago.EpochIndex) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		accountBuilder.Staking(amount, fixedCost, startEpoch, optEndEpoch...)
	}
}

// Account options

func WithAccountMana(mana iotago.Mana) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		accountBuilder.Mana(mana)
	}
}

func WithAccountAmount(amount iotago.BaseToken) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		accountBuilder.Amount(amount)
	}
}

func WithAccountIncreasedFoundryCounter(diff uint32) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		accountBuilder.FoundriesToGenerate(diff)
	}
}

func WithAccountImmutableFeatures(features iotago.AccountOutputImmFeatures) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		for _, feature := range features.MustSet() {
			switch feature.Type() {
			case iotago.FeatureMetadata:
				//nolint:forcetypeassert
				accountBuilder.ImmutableMetadata(feature.(*iotago.MetadataFeature).Data)
			case iotago.FeatureSender:
				//nolint:forcetypeassert
				accountBuilder.ImmutableSender(feature.(*iotago.SenderFeature).Address)
			}
		}
	}
}

func WithAccountConditions(conditions iotago.AccountOutputUnlockConditions) options.Option[builder.AccountOutputBuilder] {
	return func(accountBuilder *builder.AccountOutputBuilder) {
		for _, condition := range conditions.MustSet() {
			switch condition.Type() {
			case iotago.UnlockConditionStateControllerAddress:
				//nolint:forcetypeassert
				accountBuilder.StateController(condition.(*iotago.StateControllerAddressUnlockCondition).Address)
			case iotago.UnlockConditionGovernorAddress:
				//nolint:forcetypeassert
				accountBuilder.Governor(condition.(*iotago.GovernorAddressUnlockCondition).Address)
			}
		}
	}
}

// TransactionBuilder options

func WithInputs(inputs utxoledger.Outputs) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		for _, input := range inputs {
			switch input.OutputType() {
			case iotago.OutputFoundry:
				// For foundries we need to unlock the account output
				txBuilder.AddInput(&builder.TxInput{
					UnlockTarget: input.Output().UnlockConditionSet().ImmutableAccount().Address,
					InputID:      input.OutputID(),
					Input:        input.Output(),
				})
			case iotago.OutputAccount:
				// For alias we need to unlock the state controller
				txBuilder.AddInput(&builder.TxInput{
					UnlockTarget: input.Output().UnlockConditionSet().StateControllerAddress().Address,
					InputID:      input.OutputID(),
					Input:        input.Output(),
				})
			default:
				txBuilder.AddInput(&builder.TxInput{
					UnlockTarget: input.Output().UnlockConditionSet().Address().Address,
					InputID:      input.OutputID(),
					Input:        input.Output(),
				})
			}
		}
	}
}

func WithAccountInput(input *utxoledger.Output, governorTransition bool) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		switch input.OutputType() {
		case iotago.OutputAccount:
			address := input.Output().UnlockConditionSet().StateControllerAddress().Address
			if governorTransition {
				address = input.Output().UnlockConditionSet().GovernorAddress().Address
			}
			txBuilder.AddInput(&builder.TxInput{
				UnlockTarget: address,
				InputID:      input.OutputID(),
				Input:        input.Output(),
			})
		default:
			panic("only OutputAccount can be added as account input")
		}
	}
}

func WithAllotments(allotments iotago.Allotments) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		for _, allotment := range allotments {
			txBuilder.IncreaseAllotment(allotment.AccountID, allotment.Value)
		}
	}
}

func WithSlotCreated(creationSlot iotago.SlotIndex) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		txBuilder.SetCreationSlot(creationSlot)
	}
}

func WithContextInputs(contextInputs iotago.TxEssenceContextInputs) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		for _, input := range contextInputs {
			txBuilder.AddContextInput(input)
		}
	}
}

func WithOutputs(outputs iotago.Outputs[iotago.Output]) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		for _, output := range outputs {
			txBuilder.AddOutput(output)
		}
	}
}

func WithTaggedDataPayload(payload *iotago.TaggedData) options.Option[builder.TransactionBuilder] {
	return func(txBuilder *builder.TransactionBuilder) {
		txBuilder.AddTaggedDataPayload(payload)
	}
}
