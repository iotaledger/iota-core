package testsuite

import (
	"fmt"
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type TransactionFramework struct {
	apiProvider api.Provider

	wallet       *mock.HDWallet
	states       map[string]*utxoledger.Output
	transactions map[string]*iotago.Transaction
}

func NewTransactionFramework(protocol *protocol.Protocol, genesisSeed []byte, accounts ...snapshotcreator.AccountDetails) *TransactionFramework {
	// The genesis output is on index 0 of the genesis TX
	genesisOutput, err := protocol.MainEngineInstance().Ledger.Output(iotago.OutputIDFromTransactionIDAndIndex(snapshotcreator.GenesisTransactionID, 0))
	if err != nil {
		panic(err)
	}

	tf := &TransactionFramework{
		apiProvider:  protocol,
		states:       map[string]*utxoledger.Output{"Genesis:0": genesisOutput},
		transactions: make(map[string]*iotago.Transaction),
		wallet:       mock.NewHDWallet("genesis", genesisSeed, 0),
	}

	for idx := range accounts {
		// Genesis TX
		outputID := iotago.OutputIDFromTransactionIDAndIndex(snapshotcreator.GenesisTransactionID, uint16(idx+1))

		if tf.states[fmt.Sprintf("Genesis:%d", idx+1)], err = protocol.MainEngineInstance().Ledger.Output(outputID); err != nil {
			panic(err)
		}
	}

	return tf
}

func (t *TransactionFramework) RegisterTransaction(alias string, transaction *iotago.Transaction) {
	currentAPI := t.apiProvider.CurrentAPI()
	(lo.PanicOnErr(transaction.ID(currentAPI))).RegisterAlias(alias)

	t.transactions[alias] = transaction

	for outputID, output := range lo.PanicOnErr(transaction.OutputsSet(currentAPI)) {
		clonedOutput := output.Clone()
		actualOutputID := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(transaction.ID(currentAPI)), outputID.Index())
		if clonedOutput.Type() == iotago.OutputAccount {
			if accountOutput, ok := clonedOutput.(*iotago.AccountOutput); ok && accountOutput.AccountID == iotago.EmptyAccountID() {
				accountOutput.AccountID = iotago.AccountIDFromOutputID(actualOutputID)
			}
		}

		t.states[fmt.Sprintf("%s:%d", alias, outputID.Index())] = utxoledger.CreateOutput(t.apiProvider, actualOutputID, iotago.EmptyBlockID(), 0, currentAPI.TimeProvider().SlotFromTime(time.Now()), clonedOutput)
	}
}

func (t *TransactionFramework) CreateTransactionWithOptions(alias string, signingWallets []*mock.HDWallet, opts ...options.Option[builder.TransactionBuilder]) (*iotago.Transaction, error) {
	currentAPI := t.apiProvider.CurrentAPI()

	walletKeys := make([]iotago.AddressKeys, len(signingWallets))
	for i, wallet := range signingWallets {
		inputPrivateKey, _ := wallet.KeyPair()
		walletKeys[i] = iotago.AddressKeys{Address: wallet.Address(), Keys: inputPrivateKey}
	}

	txBuilder := builder.NewTransactionBuilder(currentAPI)

	// Always add a random payload to randomize transaction ID.
	randomPayload := tpkg.Rand12ByteArray()
	txBuilder.AddTaggedDataPayload(&iotago.TaggedData{Tag: randomPayload[:], Data: randomPayload[:]})

	tx, err := options.Apply(txBuilder, opts).Build(iotago.NewInMemoryAddressSigner(walletKeys...))
	if err == nil {
		t.RegisterTransaction(alias, tx)
	}

	return tx, err
}

func (t *TransactionFramework) CreateSimpleTransaction(alias string, outputCount int, inputAliases ...string) (*iotago.Transaction, error) {
	inputStates, outputStates, signingWallets := t.CreateBasicOutputsEqually(outputCount, inputAliases...)

	return t.CreateTransactionWithOptions(alias, signingWallets, WithInputs(inputStates), WithOutputs(outputStates))
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
			Amount:       tokenAmount,
			Mana:         manaAmount,
			NativeTokens: iotago.NativeTokens{},
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
			Amount:       outputAmount,
			Mana:         manaDistribution[idx],
			NativeTokens: iotago.NativeTokens{},
			Conditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: t.DefaultAddress()},
			},
			Features: iotago.BasicOutputFeatures{},
		})
	}

	return inputStates, outputStates, []*mock.HDWallet{t.wallet}
}

func (t *TransactionFramework) CreateAccountFromInput(inputAlias string, opts ...options.Option[iotago.AccountOutput]) (utxoledger.Outputs, iotago.Outputs[iotago.Output], []*mock.HDWallet) {
	input := t.Output(inputAlias)

	accountOutput := options.Apply(&iotago.AccountOutput{
		Amount:       input.BaseTokenAmount(),
		Mana:         input.StoredMana(),
		NativeTokens: iotago.NativeTokens{},
		Conditions: iotago.AccountOutputUnlockConditions{
			&iotago.StateControllerAddressUnlockCondition{Address: t.DefaultAddress()},
			&iotago.GovernorAddressUnlockCondition{Address: t.DefaultAddress()},
		},
		Features:          iotago.AccountOutputFeatures{},
		ImmutableFeatures: iotago.AccountOutputImmFeatures{},
	}, opts)

	outputStates := iotago.Outputs[iotago.Output]{accountOutput}

	// if amount was set by options, a remainder output needs to be created
	if accountOutput.Amount != input.BaseTokenAmount() {
		outputStates = append(outputStates, &iotago.BasicOutput{
			Amount:       input.BaseTokenAmount() - accountOutput.Amount,
			Mana:         input.StoredMana() - accountOutput.Mana,
			NativeTokens: iotago.NativeTokens{},
			Conditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: t.DefaultAddress()},
			},
			Features: iotago.BasicOutputFeatures{},
		})
	}

	return utxoledger.Outputs{input}, outputStates, []*mock.HDWallet{t.wallet}
}

// CreateDelegationFromInput creates a new DelegationOutput with given options from an input. If the remainder Output
// is not created, then StoredMana from the input is not passed and can potentially be burned.
// In order not to burn it, it needs to be assigned manually in another output in the transaction.
func (t *TransactionFramework) CreateDelegationFromInput(inputAlias string, opts ...options.Option[iotago.DelegationOutput]) (utxoledger.Outputs, iotago.Outputs[iotago.Output], []*mock.HDWallet) {
	input := t.Output(inputAlias)

	delegationOutput := options.Apply(&iotago.DelegationOutput{
		Amount:          input.BaseTokenAmount(),
		DelegatedAmount: input.BaseTokenAmount(),
		DelegationID:    iotago.DelegationID{},
		ValidatorID:     iotago.AccountID{},
		StartEpoch:      0,
		EndEpoch:        0,
		Conditions: iotago.DelegationOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: t.DefaultAddress()},
		},
	}, opts)

	if delegationOutput.ValidatorID == iotago.EmptyAccountID() ||
		delegationOutput.DelegatedAmount == 0 ||
		delegationOutput.StartEpoch == 0 {
		panic(fmt.Sprintf("delegation output created incorrectly %+v", delegationOutput))
	}

	outputStates := iotago.Outputs[iotago.Output]{delegationOutput}

	// if options set an Amount, a remainder output needs to be created
	if delegationOutput.Amount != input.BaseTokenAmount() {
		outputStates = append(outputStates, &iotago.BasicOutput{
			Amount:       input.BaseTokenAmount() - delegationOutput.Amount,
			Mana:         input.StoredMana(),
			NativeTokens: iotago.NativeTokens{},
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

	delegationOutput, ok := input.Output().Clone().(*iotago.DelegationOutput)
	if !ok {
		panic(ierrors.Errorf("cloned output %s is not a delegation output, cannot transition to delayed claiming state", inputAlias))
	}

	if delegationOutput.DelegationID == iotago.EmptyDelegationID() {
		delegationOutput.DelegationID = iotago.DelegationIDFromOutputID(input.OutputID())
	}
	delegationOutput.EndEpoch = delegationEndEpoch

	return utxoledger.Outputs{input}, iotago.Outputs[iotago.Output]{delegationOutput}, []*mock.HDWallet{t.wallet}
}

func (t *TransactionFramework) DestroyAccount(alias string) (consumedInputs *utxoledger.Output, outputs iotago.Outputs[iotago.Output], signingWallets []*mock.HDWallet) {
	output := t.Output(alias)

	outputStates := iotago.Outputs[iotago.Output]{&iotago.BasicOutput{
		Amount:       output.BaseTokenAmount(),
		Mana:         output.StoredMana(),
		NativeTokens: iotago.NativeTokens{},
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: t.DefaultAddress()},
		},
		Features: iotago.BasicOutputFeatures{},
	}}

	return output, outputStates, []*mock.HDWallet{t.wallet}
}

func (t *TransactionFramework) TransitionAccount(alias string, opts ...options.Option[iotago.AccountOutput]) (consumedInput *utxoledger.Output, outputs iotago.Outputs[iotago.Output], signingWallets []*mock.HDWallet) {
	output, exists := t.states[alias]
	if !exists {
		panic(fmt.Sprintf("account with alias %s does not exist", alias))
	}

	accountOutput, ok := output.Output().Clone().(*iotago.AccountOutput)
	if !ok {
		panic(fmt.Sprintf("output with alias %s is not *iotago.AccountOutput", alias))
	}

	accountOutput = options.Apply(accountOutput, opts)

	return output, iotago.Outputs[iotago.Output]{accountOutput}, []*mock.HDWallet{t.wallet}
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

func (t *TransactionFramework) Transaction(alias string) *iotago.Transaction {
	transaction, exists := t.transactions[alias]
	if !exists {
		panic(ierrors.Errorf("transaction with given alias does not exist %s", alias))
	}

	return transaction
}

func (t *TransactionFramework) TransactionID(alias string) iotago.TransactionID {
	return lo.PanicOnErr(t.Transaction(alias).ID(t.apiProvider.CurrentAPI()))
}

func (t *TransactionFramework) Transactions(aliases ...string) []*iotago.Transaction {
	return lo.Map(aliases, t.Transaction)
}

func (t *TransactionFramework) TransactionIDs(aliases ...string) []iotago.TransactionID {
	return lo.Map(aliases, t.TransactionID)
}

func (t *TransactionFramework) DefaultAddress() iotago.Address {
	return t.wallet.Address()
}

// DelegationOutput options

func WithDelegatedAmount(delegatedAmount iotago.BaseToken) options.Option[iotago.DelegationOutput] {
	return func(delegationOutput *iotago.DelegationOutput) {
		delegationOutput.DelegatedAmount = delegatedAmount
	}
}

func WithDelegatedValidatorID(validatorID iotago.AccountID) options.Option[iotago.DelegationOutput] {
	return func(delegationOutput *iotago.DelegationOutput) {
		delegationOutput.ValidatorID = validatorID
	}
}

func WithDelegationStartEpoch(startEpoch iotago.EpochIndex) options.Option[iotago.DelegationOutput] {
	return func(delegationOutput *iotago.DelegationOutput) {
		delegationOutput.StartEpoch = startEpoch
	}
}

func WithDelegationEndEpoch(endEpoch iotago.EpochIndex) options.Option[iotago.DelegationOutput] {
	return func(delegationOutput *iotago.DelegationOutput) {
		delegationOutput.EndEpoch = endEpoch
	}
}

func WithDelegationConditions(delegationConditions iotago.DelegationOutputUnlockConditions) options.Option[iotago.DelegationOutput] {
	return func(delegationOutput *iotago.DelegationOutput) {
		delegationOutput.Conditions = delegationConditions
	}
}

func WithDelegationAmount(amount iotago.BaseToken) options.Option[iotago.DelegationOutput] {
	return func(delegationOutput *iotago.DelegationOutput) {
		delegationOutput.Amount = amount
	}
}

// BlockIssuer options

func WithBlockIssuerFeature(blockIssuerFeature *iotago.BlockIssuerFeature) options.Option[iotago.AccountOutput] {
	return func(accountOutput *iotago.AccountOutput) {
		for idx, feature := range accountOutput.Features {
			if feature.Type() == iotago.FeatureBlockIssuer {
				accountOutput.Features[idx] = blockIssuerFeature
				return
			}
		}

		accountOutput.Features = append(accountOutput.Features, blockIssuerFeature)
	}
}

func AddBlockIssuerKey(key iotago.BlockIssuerKey) options.Option[iotago.AccountOutput] {
	return func(accountOutput *iotago.AccountOutput) {
		blockIssuer := accountOutput.FeatureSet().BlockIssuer()
		if blockIssuer == nil {
			panic("cannot add block issuer key to account without BlockIssuer feature")
		}
		blockIssuer.BlockIssuerKeys = append(blockIssuer.BlockIssuerKeys, key)

		blockIssuer.BlockIssuerKeys.Sort()
	}
}

func WithBlockIssuerKeys(keys iotago.BlockIssuerKeys) options.Option[iotago.AccountOutput] {
	return func(accountOutput *iotago.AccountOutput) {
		blockIssuer := accountOutput.FeatureSet().BlockIssuer()
		if blockIssuer == nil {
			panic("cannot set block issuer keys to account without BlockIssuer feature")
		}
		blockIssuer.BlockIssuerKeys = keys
	}
}

func WithBlockIssuerExpirySlot(expirySlot iotago.SlotIndex) options.Option[iotago.AccountOutput] {
	return func(accountOutput *iotago.AccountOutput) {
		blockIssuer := accountOutput.FeatureSet().BlockIssuer()
		if blockIssuer == nil {
			panic("cannot set block issuer expiry slot to account without BlockIssuer feature")
		}
		blockIssuer.ExpirySlot = expirySlot
	}
}

func WithStakingFeature(stakingFeature *iotago.StakingFeature) options.Option[iotago.AccountOutput] {
	return func(accountOutput *iotago.AccountOutput) {
		for idx, feature := range accountOutput.Features {
			if feature.Type() == iotago.FeatureStaking {
				accountOutput.Features[idx] = stakingFeature
				return
			}
		}

		accountOutput.Features = append(accountOutput.Features, stakingFeature)
	}
}

func WithStakingEndEpoch(endEpoch iotago.EpochIndex) options.Option[iotago.AccountOutput] {
	return func(accountOutput *iotago.AccountOutput) {
		staking := accountOutput.FeatureSet().Staking()
		if staking == nil {
			panic("cannot update staking end epoch on account without Staking feature")
		}
		staking.EndEpoch = endEpoch
	}
}

// Account options

func WithAccountMana(mana iotago.Mana) options.Option[iotago.AccountOutput] {
	return func(accountOutput *iotago.AccountOutput) {
		accountOutput.Mana = mana
	}
}

func WithAccountAmount(amount iotago.BaseToken) options.Option[iotago.AccountOutput] {
	return func(accountOutput *iotago.AccountOutput) {
		accountOutput.Amount = amount
	}
}

func WithAccountIncreasedStateIndex() options.Option[iotago.AccountOutput] {
	return func(accountOutput *iotago.AccountOutput) {
		accountOutput.StateIndex++
	}
}

func WithAccountIncreasedFoundryCounter(diff uint32) options.Option[iotago.AccountOutput] {
	return func(accountOutput *iotago.AccountOutput) {
		accountOutput.FoundryCounter += diff
	}
}

func WithAccountFeatures(features iotago.AccountOutputFeatures) options.Option[iotago.AccountOutput] {
	return func(accountOutput *iotago.AccountOutput) {
		accountOutput.Features = features
	}
}

func WithAccountImmutableFeatures(features iotago.AccountOutputImmFeatures) options.Option[iotago.AccountOutput] {
	return func(accountOutput *iotago.AccountOutput) {
		accountOutput.ImmutableFeatures = features
	}
}

func WithAccountConditions(conditions iotago.AccountOutputUnlockConditions) options.Option[iotago.AccountOutput] {
	return func(accountOutput *iotago.AccountOutput) {
		accountOutput.Conditions = conditions
	}
}

func WithAccountNativeTokens(nativeTokens iotago.NativeTokens) options.Option[iotago.AccountOutput] {
	return func(accountOutput *iotago.AccountOutput) {
		accountOutput.NativeTokens = nativeTokens
	}
}

// Transaction options

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
			txBuilder.AddAllotment(allotment)
		}
	}
}

func WithCreationSlot(creationSlot iotago.SlotIndex) options.Option[builder.TransactionBuilder] {
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
