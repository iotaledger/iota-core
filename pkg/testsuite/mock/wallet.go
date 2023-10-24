package mock

import (
	"crypto/ed25519"
	"fmt"
	"testing"
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

// Wallet is an object representing a wallet (similar to a FireFly wallet) capable of the following:
// - hierarchical deterministic key management
// - signing transactions
// - signing blocks
// - keeping track of unspent outputs.
type Wallet struct {
	Testing *testing.T

	Name string

	node *Node

	keyManager *KeyManager

	BlockIssuer *BlockIssuer

	outputs      map[string]*utxoledger.Output
	transactions map[string]*iotago.Transaction
}

func NewWallet(t *testing.T, name string, node *Node, seed ...[]byte) *Wallet {
	if len(seed) == 0 {
		randomSeed := tpkg.RandEd25519Seed()
		seed = append(seed, randomSeed[:])
	}
	keyManager := NewKeyManager(seed[0], 0)

	return &Wallet{
		Testing:      t,
		Name:         name,
		node:         node,
		outputs:      make(map[string]*utxoledger.Output),
		transactions: make(map[string]*iotago.Transaction),
		keyManager:   keyManager,
	}
}

func (w *Wallet) AddBlockIssuer(accountID iotago.AccountID) {
	w.BlockIssuer = NewBlockIssuer(w.Testing, w.Name, w.keyManager, accountID, false)
}

func (w *Wallet) Balance() iotago.BaseToken {
	var balance iotago.BaseToken
	for _, output := range w.outputs {
		balance += output.BaseTokenAmount()
	}

	return balance
}

func (w *Wallet) Output(outputName string) *utxoledger.Output {
	output, exists := w.outputs[outputName]
	if !exists {
		panic(ierrors.Errorf("output %s not registered in wallet %s", outputName, w.Name))
	}

	return output
}

func (w *Wallet) AccountOutput(outputName string) *utxoledger.Output {
	output, exists := w.outputs[outputName]
	if !exists {
		panic(ierrors.Errorf("output %s not registered in wallet %s", outputName, w.Name))
	}

	if _, ok := output.Output().(*iotago.AccountOutput); !ok {
		panic(ierrors.Errorf("output %s is not an account output", outputName))
	}

	return output
}

func (w *Wallet) Transaction(alias string) *iotago.Transaction {
	transaction, exists := w.transactions[alias]
	if !exists {
		panic(ierrors.Errorf("transaction with given alias does not exist %s", alias))
	}

	return transaction
}

func (w *Wallet) Transactions(transactionNames ...string) []*iotago.Transaction {
	return lo.Map(transactionNames, w.Transaction)
}

func (w *Wallet) TransactionID(alias string) iotago.TransactionID {
	return lo.PanicOnErr(w.Transaction(alias).ID())
}

func (w *Wallet) AddOutput(outputName string, output *utxoledger.Output) {
	w.outputs[outputName] = output
}

func (w *Wallet) PrintStatus() {
	var status string
	status += fmt.Sprintf("Name: %s\n", w.Name)
	status += fmt.Sprintf("Address: %s\n", w.keyManager.Address().Bech32(iotago.PrefixTestnet))
	status += fmt.Sprintf("Balance: %d\n", w.Balance())
	status += "Outputs: \n"
	for _, u := range w.outputs {
		nativeTokenDescription := ""
		nativeTokenFeature := u.Output().FeatureSet().NativeToken()
		if nativeTokenFeature != nil {
			nativeTokenDescription += fmt.Sprintf("%s: %s, ", nativeTokenFeature.ID.ToHex(), nativeTokenFeature.Amount)
		}
		status += fmt.Sprintf("\t%s [%s] = %d %v\n", u.OutputID().ToHex(), u.OutputType(), u.BaseTokenAmount(), nativeTokenDescription)
	}
	fmt.Printf("%s\n", status)
}

func (w *Wallet) Address(addressType ...iotago.AddressType) iotago.DirectUnlockableAddress {
	return w.keyManager.Address(addressType...)
}

func (w *Wallet) ImplicitAccountCreationAddress() *iotago.ImplicitAccountCreationAddress {
	address := w.keyManager.Address(iotago.AddressImplicitAccountCreation)
	//nolint:forcetypeassert
	return address.(*iotago.ImplicitAccountCreationAddress)
}

func (w *Wallet) HasAddress(address iotago.Address) bool {
	return address.Equal(w.Address()) || address.Equal(w.ImplicitAccountCreationAddress())
}

func (w *Wallet) KeyPair() (ed25519.PrivateKey, ed25519.PublicKey) {
	return w.keyManager.KeyPair()
}

func (w *Wallet) AddressSigner() iotago.AddressSigner {
	return w.keyManager.AddressSigner()
}

func (w *Wallet) CreateAccountFromInput(transactionName string, inputName string, recipientWallet *Wallet, creationSlot iotago.SlotIndex, opts ...options.Option[builder.AccountOutputBuilder]) *iotago.SignedTransaction {
	input := w.Output(inputName)

	accountOutput := options.Apply(builder.NewAccountOutputBuilder(recipientWallet.Address(), recipientWallet.Address(), input.BaseTokenAmount()).
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
		WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.CommitmentInput{
				CommitmentID: w.node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
			},
		}),
		WithInputs(utxoledger.Outputs{input}),
		WithOutputs(outputStates),
		WithSlotCreated(creationSlot),
	))

	// register the outputs in each wallet
	w.registerOutputs(transactionName, signedTransaction.Transaction)
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
		WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.CommitmentInput{
				CommitmentID: w.node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
			},
		}),
		WithInputs(utxoledger.Outputs{input}),
		WithOutputs(outputStates),
		WithSlotCreated(creationSlot),
	))

	// register the outputs in the wallet
	w.registerOutputs(transactionName, signedTransaction.Transaction)

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
		WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.CommitmentInput{
				CommitmentID: w.node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
			},
		}),
		WithInputs(utxoledger.Outputs{input}),
		WithOutputs(iotago.Outputs[iotago.Output]{delegationOutput}),
		WithSlotCreated(creationSlot),
	))

	// register the outputs in the wallet
	w.registerOutputs(transactionName, signedTransaction.Transaction)

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
		WithAccountInput(input, true),
		WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.BlockIssuanceCreditInput{
				AccountID: accountOutput.AccountID,
			},
			&iotago.CommitmentInput{
				CommitmentID: w.node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
			},
		}),
		WithOutputs(iotago.Outputs[iotago.Output]{accountOutput}),
	))

	// register the outputs in the wallet
	w.registerOutputs(transactionName, signedTransaction.Transaction)

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
		WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.BlockIssuanceCreditInput{
				AccountID: inputAccount.AccountID,
			},
			&iotago.CommitmentInput{
				CommitmentID: w.node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
			},
		}),
		WithAccountInput(input, true),
		WithOutputs(destructionOutputs),
		WithSlotCreated(creationSlot),
	))

	// register the outputs in the wallet
	w.registerOutputs(transactionName, signedTransaction.Transaction)

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
		WithInputs(utxoledger.Outputs{input}),
		WithOutputs(iotago.Outputs[iotago.Output]{implicitAccountOutput, remainderBasicOutput}),
	))

	// register the outputs in the wallet
	w.registerOutputs(transactionName, signedTransaction.Transaction)
	recipientWallet.registerOutputs(transactionName, signedTransaction.Transaction)

	// register the implicit account as a block issuer in the wallet
	implicitAccountID := iotago.AccountIDFromOutputID(recipientWallet.Output(fmt.Sprintf("%s:0", transactionName)).OutputID())
	recipientWallet.AddBlockIssuer(implicitAccountID)

	return signedTransaction
}

func (w *Wallet) TransitionImplicitAccountToAccountOutput(transactioName string, inputName string, creationSlot iotago.SlotIndex, opts ...options.Option[builder.AccountOutputBuilder]) *iotago.SignedTransaction {
	input := w.Output(inputName)
	implicitAccountID := iotago.AccountIDFromOutputID(input.OutputID())

	basicOutput, isBasic := input.Output().(*iotago.BasicOutput)
	if !isBasic {
		panic(fmt.Sprintf("output with alias %s is not *iotago.BasicOutput", inputName))
	}
	if basicOutput.UnlockConditionSet().Address().Address.Type() != iotago.AddressImplicitAccountCreation {
		panic(fmt.Sprintf("output with alias %s is not an implicit account", inputName))
	}

	accountOutput := options.Apply(builder.NewAccountOutputBuilder(w.Address(), w.Address(), MinIssuerAccountAmount).
		AccountID(iotago.AccountIDFromOutputID(input.OutputID())),
		opts).MustBuild()

	signedTransaction := lo.PanicOnErr(w.createSignedTransactionWithAllotmentAndOptions(
		creationSlot,
		implicitAccountID,
		WithContextInputs(iotago.TxEssenceContextInputs{
			&iotago.BlockIssuanceCreditInput{
				AccountID: implicitAccountID,
			},
			&iotago.CommitmentInput{
				CommitmentID: w.node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID(),
			},
		}),
		WithInputs(utxoledger.Outputs{input}),
		WithOutputs(iotago.Outputs[iotago.Output]{accountOutput}),
		WithSlotCreated(creationSlot),
	))

	// register the outputs in the wallet
	w.registerOutputs(transactioName, signedTransaction.Transaction)

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
		WithInputs(inputStates),
		WithOutputs(outputStates),
	))

	// register the outputs in the wallet
	w.registerOutputs(transactionName, signedTransaction.Transaction)

	return signedTransaction
}

func (w *Wallet) createSignedTransactionWithOptions(opts ...options.Option[builder.TransactionBuilder]) (*iotago.SignedTransaction, error) {
	currentAPI := w.node.Protocol.CommittedAPI()

	txBuilder := builder.NewTransactionBuilder(currentAPI)
	txBuilder.WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything()))
	// Always add a random payload to randomize transaction ID.
	randomPayload := tpkg.Rand12ByteArray()
	txBuilder.AddTaggedDataPayload(&iotago.TaggedData{Tag: randomPayload[:], Data: randomPayload[:]})

	signedTransaction, err := options.Apply(txBuilder, opts).Build(w.AddressSigner())

	return signedTransaction, err
}

func (w *Wallet) createSignedTransactionWithAllotmentAndOptions(creationSlot iotago.SlotIndex, blockIssuerAccountID iotago.AccountID, opts ...options.Option[builder.TransactionBuilder]) (*iotago.SignedTransaction, error) {
	currentAPI := w.node.Protocol.CommittedAPI()

	txBuilder := builder.NewTransactionBuilder(currentAPI)
	txBuilder.WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything()))
	// Always add a random payload to randomize transaction ID.
	randomPayload := tpkg.Rand12ByteArray()
	txBuilder.AddTaggedDataPayload(&iotago.TaggedData{Tag: randomPayload[:], Data: randomPayload[:]})
	options.Apply(txBuilder, opts)
	txBuilder.AllotAllMana(creationSlot, blockIssuerAccountID)

	signedTransaction, err := txBuilder.Build(w.AddressSigner())

	return signedTransaction, err
}

func (w *Wallet) registerOutputs(transactionName string, transaction *iotago.Transaction) {
	currentAPI := w.node.Protocol.CommittedAPI()
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
			w.outputs[fmt.Sprintf("%s:%d", transactionName, outputID.Index())] = utxoledger.CreateOutput(w.node.Protocol, actualOutputID, iotago.EmptyBlockID, currentAPI.TimeProvider().SlotFromTime(time.Now()), clonedOutput, lo.PanicOnErr(iotago.OutputIDProofFromTransaction(transaction, outputID.Index())))
		}
	}
}
