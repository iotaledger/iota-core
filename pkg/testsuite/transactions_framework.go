package testsuite

import (
	"fmt"
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type TransactionFramework struct {
	apiProvider iotago.APIProvider

	wallet             *mock.HDWallet
	states             map[string]*utxoledger.Output
	signedTransactions map[string]*iotago.SignedTransaction
	transactions       map[string]*iotago.Transaction
}

func NewTransactionFramework(protocol *protocol.Protocol, genesisSeed []byte) *TransactionFramework {
	tf := &TransactionFramework{
		apiProvider:        protocol,
		states:             make(map[string]*utxoledger.Output),
		signedTransactions: make(map[string]*iotago.SignedTransaction),
		transactions:       make(map[string]*iotago.Transaction),

		wallet: mock.NewHDWallet("genesis", genesisSeed, 0),
	}

	if err := protocol.MainEngineInstance().Ledger.ForEachUnspentOutput(func(output *utxoledger.Output) bool {
		tf.states[fmt.Sprintf("Genesis:%d", output.OutputID().Index())] = output
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

func (t *TransactionFramework) CreateSignedTransactionWithOptions(alias string, signingWallets []*mock.HDWallet, opts ...options.Option[builder.TransactionBuilder]) (*iotago.SignedTransaction, error) {
	currentAPI := t.apiProvider.CommittedAPI()

	walletKeys := make([]iotago.AddressKeys, 0, len(signingWallets)*2)
	for _, wallet := range signingWallets {
		inputPrivateKey, _ := wallet.KeyPair()
		// add address keys for both types of directly unlockable addresses to simplify the TransactionFramework
		//nolint:forcetypeassert
		walletKeys = append(walletKeys, iotago.NewAddressKeysForEd25519Address(wallet.Address(iotago.AddressEd25519).(*iotago.Ed25519Address), inputPrivateKey))
		//nolint:forcetypeassert
		walletKeys = append(walletKeys, iotago.NewAddressKeysForImplicitAccountCreationAddress(wallet.Address(iotago.AddressImplicitAccountCreation).(*iotago.ImplicitAccountCreationAddress), inputPrivateKey))
	}

	txBuilder := builder.NewTransactionBuilder(currentAPI)
	txBuilder.WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything()))
	// Always add a random payload to randomize transaction ID.
	randomPayload := tpkg.Rand12ByteArray()
	txBuilder.AddTaggedDataPayload(&iotago.TaggedData{Tag: randomPayload[:], Data: randomPayload[:]})

	signedTransaction, err := options.Apply(txBuilder, opts).Build(iotago.NewInMemoryAddressSigner(walletKeys...))
	if err == nil {
		t.RegisterSignedTransaction(alias, signedTransaction)
		t.RegisterTransaction(alias, signedTransaction.Transaction)
	}

	return signedTransaction, err
}

func (t *TransactionFramework) CreateSimpleTransaction(alias string, outputCount int, inputAliases ...string) (*iotago.SignedTransaction, error) {
	inputStates, outputStates, signingWallets := t.CreateBasicOutputsEqually(outputCount, inputAliases...)

	return t.CreateSignedTransactionWithOptions(alias, signingWallets, WithInputs(inputStates), WithOutputs(outputStates))
}

func (t *TransactionFramework) CreateBasicOutputsEqually(outputCount int, inputAliases ...string) (consumedInputs utxoledger.Outputs, outputs iotago.Outputs[iotago.Output], signingWallets []*mock.HDWallet) {
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
				&iotago.AddressUnlockCondition{Address: t.DefaultAddress()},
			},
			Features: iotago.BasicOutputFeatures{},
		})
	}

	return inputStates, outputStates, []*mock.HDWallet{t.wallet}
}

func (t *TransactionFramework) CreateBasicOutputs(amountDistribution []iotago.BaseToken, manaDistribution []iotago.Mana, inputAliases ...string) (consumedInputs utxoledger.Outputs, outputs iotago.Outputs[iotago.Output], signingWallets []*mock.HDWallet) {
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
				&iotago.AddressUnlockCondition{Address: t.DefaultAddress()},
			},
			Features: iotago.BasicOutputFeatures{},
		})
	}

	return inputStates, outputStates, []*mock.HDWallet{t.wallet}
}

func (t *TransactionFramework) CreateAccountFromInput(inputAlias string, opts ...options.Option[builder.AccountOutputBuilder]) (utxoledger.Outputs, iotago.Outputs[iotago.Output], []*mock.HDWallet) {
	input := t.Output(inputAlias)

	accountOutput := options.Apply(builder.NewAccountOutputBuilder(t.DefaultAddress(), t.DefaultAddress(), input.BaseTokenAmount()).
		Mana(input.StoredMana()).
		StateController(t.DefaultAddress()).
		Governor(t.DefaultAddress()),
		opts).MustBuild()

	outputStates := iotago.Outputs[iotago.Output]{accountOutput}

	// if amount was set by options, a remainder output needs to be created
	if accountOutput.Amount != input.BaseTokenAmount() {
		outputStates = append(outputStates, &iotago.BasicOutput{
			Amount: input.BaseTokenAmount() - accountOutput.Amount,
			Mana:   input.StoredMana() - accountOutput.Mana,
			Conditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: t.DefaultAddress()},
			},
			Features: iotago.BasicOutputFeatures{},
		})
	}

	return utxoledger.Outputs{input}, outputStates, []*mock.HDWallet{t.wallet}
}

// CreateImplicitAccountFromInput creates an implicit account output.
func (t *TransactionFramework) CreateImplicitAccountFromInput(inputAlias string) (utxoledger.Outputs, iotago.Outputs[iotago.Output], *iotago.ImplicitAccountCreationAddress, []*mock.HDWallet) {
	input := t.Output(inputAlias)

	//nolint:forcetypeassert
	implicitAccountAddress := t.DefaultAddress(iotago.AddressImplicitAccountCreation).(*iotago.ImplicitAccountCreationAddress)

	basicOutput := &iotago.BasicOutput{
		Amount: input.BaseTokenAmount(),
		Mana:   input.StoredMana(),
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: implicitAccountAddress},
		},
		Features: iotago.BasicOutputFeatures{},
	}

	return utxoledger.Outputs{input}, iotago.Outputs[iotago.Output]{basicOutput}, implicitAccountAddress, []*mock.HDWallet{t.wallet}
}

// CreateDelegationFromInput creates a new DelegationOutput with given options from an input. If the remainder Output
// is not created, then StoredMana from the input is not passed and can potentially be burned.
// In order not to burn it, it needs to be assigned manually in another output in the transaction.
func (t *TransactionFramework) CreateDelegationFromInput(inputAlias string, opts ...options.Option[builder.DelegationOutputBuilder]) (utxoledger.Outputs, iotago.Outputs[iotago.Output], []*mock.HDWallet) {
	input := t.Output(inputAlias)

	delegationOutput := options.Apply(builder.NewDelegationOutputBuilder(&iotago.AccountAddress{}, t.DefaultAddress(), input.BaseTokenAmount()).
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
				&iotago.AddressUnlockCondition{Address: t.DefaultAddress()},
			},
			Features: iotago.BasicOutputFeatures{},
		})
	}

	return utxoledger.Outputs{input}, outputStates, []*mock.HDWallet{t.wallet}
}

// DelayedClaimingTransition transitions DelegationOutput into delayed claiming state by setting DelegationID and EndEpoch.
func (t *TransactionFramework) DelayedClaimingTransition(inputAlias string, delegationEndEpoch iotago.EpochIndex) (utxoledger.Outputs, iotago.Outputs[iotago.Output], []*mock.HDWallet) {
	input := t.Output(inputAlias)
	if input.OutputType() != iotago.OutputDelegation {
		panic(ierrors.Errorf("%s is not a delegation output, cannot transition to delayed claiming state", inputAlias))
	}

	prevOutput, ok := input.Output().Clone().(*iotago.DelegationOutput)
	if !ok {
		panic(ierrors.Errorf("cloned output %s is not a delegation output, cannot transition to delayed claiming state", inputAlias))
	}

	delegationBuilder := builder.NewDelegationOutputBuilderFromPrevious(prevOutput).EndEpoch(delegationEndEpoch)
	if prevOutput.DelegationID == iotago.EmptyDelegationID() {
		delegationBuilder.DelegationID(iotago.DelegationIDFromOutputID(input.OutputID()))
	}

	return utxoledger.Outputs{input}, iotago.Outputs[iotago.Output]{delegationBuilder.MustBuild()}, []*mock.HDWallet{t.wallet}
}

func (t *TransactionFramework) DestroyAccount(alias string) (consumedInputs *utxoledger.Output, outputs iotago.Outputs[iotago.Output], signingWallets []*mock.HDWallet) {
	output := t.Output(alias)

	outputStates := iotago.Outputs[iotago.Output]{&iotago.BasicOutput{
		Amount: output.BaseTokenAmount(),
		Mana:   output.StoredMana(),
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: t.DefaultAddress()},
		},
		Features: iotago.BasicOutputFeatures{},
	}}

	return output, outputStates, []*mock.HDWallet{t.wallet}
}

func (t *TransactionFramework) TransitionAccount(alias string, opts ...options.Option[builder.AccountOutputBuilder]) (consumedInput *utxoledger.Output, outputs iotago.Outputs[iotago.Output], signingWallets []*mock.HDWallet) {
	output, exists := t.states[alias]
	if !exists {
		panic(fmt.Sprintf("account with alias %s does not exist", alias))
	}

	accountOutput, ok := output.Output().Clone().(*iotago.AccountOutput)
	if !ok {
		panic(fmt.Sprintf("output with alias %s is not *iotago.AccountOutput", alias))
	}

	accountBuilder := builder.NewAccountOutputBuilderFromPrevious(accountOutput)
	accountOutput = options.Apply(accountBuilder, opts).MustBuild()

	return output, iotago.Outputs[iotago.Output]{accountOutput}, []*mock.HDWallet{t.wallet}
}

func (t *TransactionFramework) TransitionImplicitAccountToAccountOutput(alias string, opts ...options.Option[builder.AccountOutputBuilder]) (consumedInput utxoledger.Outputs, outputs iotago.Outputs[iotago.Output], signingWallets []*mock.HDWallet) {
	input, exists := t.states[alias]
	if !exists {
		panic(fmt.Sprintf("output with alias %s does not exist", alias))
	}

	basicOutput, isBasic := input.Output().(*iotago.BasicOutput)
	if !isBasic {
		panic(fmt.Sprintf("output with alias %s is not *iotago.BasicOutput", alias))
	}
	if basicOutput.UnlockConditionSet().Address().Address.Type() != iotago.AddressImplicitAccountCreation {
		panic(fmt.Sprintf("output with alias %s is not an implicit account", alias))
	}

	accountOutput := options.Apply(builder.NewAccountOutputBuilder(t.DefaultAddress(), t.DefaultAddress(), input.BaseTokenAmount()).
		Mana(input.StoredMana()).
		AccountID(iotago.AccountIDFromOutputID(input.OutputID())),
		opts).MustBuild()

	return utxoledger.Outputs{input}, iotago.Outputs[iotago.Output]{accountOutput}, []*mock.HDWallet{t.wallet}
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

func (t *TransactionFramework) TransactionIDs(aliases ...string) []iotago.TransactionID {
	return lo.Map(aliases, t.TransactionID)
}

func (t *TransactionFramework) DefaultAddress(addressType ...iotago.AddressType) iotago.Address {
	return t.wallet.Address(addressType...)
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
				// For foundries we need to unlock the alias
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
