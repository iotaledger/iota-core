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

	outputs map[string]*utxoledger.Output
}

func NewWallet(t *testing.T, name string, node *Node, seed ...[]byte) *Wallet {
	if len(seed) == 0 {
		randomSeed := tpkg.RandEd25519Seed()
		seed = append(seed, randomSeed[:])
	}
	keyManager := NewKeyManager(seed[0], 0)

	return &Wallet{
		Testing:    t,
		Name:       name,
		node:       node,
		outputs:    make(map[string]*utxoledger.Output),
		keyManager: keyManager,
	}
}

func (w *Wallet) AddBlockIssuer(accountID iotago.AccountID) {
	w.BlockIssuer = NewBlockIssuer(w.Testing, w.Name, w.keyManager, accountID, false)
}

// func (w *Wallet) BookSpents(spentOutputs []*utxoledger.Output) {
// 	for _, spent := range spentOutputs {
// 		w.BookSpent(spent)
// 	}
// }
//
// func (w *Wallet) BookSpent(spentOutput *utxoledger.Output) {
// 	newOutputs := make([]*utxoledger.Output, 0)
// 	for _, output := range w.outputs {
// 		if output.OutputID() == spentOutput.OutputID() {
// 			fmt.Printf("%s spent %s\n", w.Name, output.OutputID().ToHex())

// 			continue
// 		}
// 		newOutputs = append(newOutputs, output)
// 	}
// 	w.outputs = newOutputs
// }

func (w *Wallet) Balance() iotago.BaseToken {
	var balance iotago.BaseToken
	for _, output := range w.outputs {
		balance += output.BaseTokenAmount()
	}

	return balance
}

// func (w *Wallet) BookOutput(output *utxoledger.Output) {
// 	if output != nil {
// 		fmt.Printf("%s book %s\n", w.Name, output.OutputID().ToHex())
// 		w.outputs = append(w.outputs, output)
// 	}
// }

func (w *Wallet) Output(outputName string) *utxoledger.Output {
	output, exists := w.outputs[outputName]
	if !exists {
		panic(ierrors.Errorf("output %s not registered in wallet %s", outputName, w.Name))
	}

	return output
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

func (w *Wallet) CreateAccountFromInput(transactionName string, recipientWallet *Wallet, inputName string, creationSlot iotago.SlotIndex, opts ...options.Option[builder.AccountOutputBuilder]) *iotago.SignedTransaction {
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

func (w *Wallet) registerOutputs(transactionName string, transaction *iotago.Transaction) {
	currentAPI := w.node.Protocol.CommittedAPI()
	(lo.PanicOnErr(transaction.ID())).RegisterAlias(transactionName)

	for outputID, output := range lo.PanicOnErr(transaction.OutputsSet()) {
		// register the output if it belongs to this wallet
		if w.HasAddress(output.UnlockConditionSet().Address().Address) {
			clonedOutput := output.Clone()
			actualOutputID := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(transaction.ID()), outputID.Index())
			if clonedOutput.Type() == iotago.OutputAccount {
				if accountOutput, ok := clonedOutput.(*iotago.AccountOutput); ok && accountOutput.AccountID == iotago.EmptyAccountID {
					accountOutput.AccountID = iotago.AccountIDFromOutputID(actualOutputID)
				}
			}
			w.outputs[fmt.Sprintf("%s:%s:%d", transactionName, w.Name, outputID.Index())] = utxoledger.CreateOutput(w.node.Protocol, actualOutputID, iotago.EmptyBlockID, currentAPI.TimeProvider().SlotFromTime(time.Now()), clonedOutput, lo.PanicOnErr(iotago.OutputIDProofFromTransaction(transaction, outputID.Index())))
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
