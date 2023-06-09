package testsuite

import (
	"crypto/ed25519"
	"encoding/binary"
	"fmt"
	"time"

	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type TransactionFramework struct {
	api         iotago.API
	protoParams *iotago.ProtocolParameters

	wallet       *mock.HDWallet
	states       map[string]*utxoledger.Output
	transactions map[string]*iotago.Transaction
}

func NewTransactionFramework(protocol *protocol.Protocol, genesisSeed []byte, accounts ...snapshotcreator.AccountDetails) *TransactionFramework {
	// The genesis output is on index 0 of the genesis TX
	genesisOutput, err := protocol.MainEngineInstance().Ledger.Output(iotago.OutputID{}.UTXOInput())
	if err != nil {
		panic(err)
	}

	tf := &TransactionFramework{
		api:          protocol.API(),
		protoParams:  protocol.MainEngineInstance().Storage.Settings().ProtocolParameters(),
		states:       map[string]*utxoledger.Output{"Genesis": genesisOutput},
		transactions: make(map[string]*iotago.Transaction),
		wallet:       mock.NewHDWallet("genesis", genesisSeed, 0),
	}

	for idx, account := range accounts {
		// Genesis TX
		outputID := iotago.OutputID{}
		// Accounts start from index 1 of the genesis TX
		binary.LittleEndian.PutUint16(outputID[iotago.TransactionIDLength:], uint16(idx+1))
		if tf.states[account.Alias], err = protocol.MainEngineInstance().Ledger.Output(outputID.UTXOInput()); err != nil {
			panic(err)
		}
	}

	return tf
}

func (t *TransactionFramework) CreateOrTransitionAccount(alias string, deposit uint64, keys ...ed25519.PublicKey) *utxoledger.Output {
	if len(keys) == 0 {
		keys = []ed25519.PublicKey{lo.Return2(t.wallet.KeyPair())}
	}

	accountID := utils.RandAccountID()
	if output, exists := t.states[alias]; exists {
		accountID = output.Output().(*iotago.AccountOutput).AccountID
	}

	accountOutput := &iotago.AccountOutput{
		Amount:    deposit,
		AccountID: accountID,
		Conditions: iotago.AccountOutputUnlockConditions{
			&iotago.StateControllerAddressUnlockCondition{
				Address: t.wallet.Address(),
			},
			&iotago.GovernorAddressUnlockCondition{
				Address: t.wallet.Address(),
			},
		},
		Features: iotago.AccountOutputFeatures{
			&iotago.BlockIssuerFeature{
				BlockIssuerKeys: keys,
			},
		},
	}

	output := utxoledger.CreateOutput(t.api, iotago.OutputIDFromTransactionIDAndIndex(utils.RandTransactionID(), 0), iotago.EmptyBlockID(), t.api.SlotTimeProvider().IndexFromTime(time.Now()), t.api.SlotTimeProvider().IndexFromTime(time.Now()), accountOutput)

	t.states[alias] = output

	return output
}

func (t *TransactionFramework) CreateTransaction(alias string, outputCount int, inputAliases ...string) (*iotago.Transaction, error) {
	inputStates, outputStates, signingWallets := t.PrepareTransaction(outputCount, inputAliases...)
	transaction, err := t.CreateTransactionWithInputsAndOutputs(inputStates, outputStates, signingWallets)
	if err != nil {
		return nil, err
	}

	t.RegisterTransaction(alias, transaction)

	return transaction, nil
}

func (t *TransactionFramework) RegisterTransaction(alias string, transaction *iotago.Transaction) {
	(lo.PanicOnErr(transaction.ID())).RegisterAlias(alias)

	t.transactions[alias] = transaction

	for outputID, output := range lo.PanicOnErr(transaction.OutputsSet()) {
		t.states[fmt.Sprintf("%s:%d", alias, outputID.Index())] = utxoledger.CreateOutput(t.api, iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(transaction.ID()), uint16(outputID.Index())), iotago.EmptyBlockID(), 0, t.api.SlotTimeProvider().IndexFromTime(time.Now()), output)
	}
}

func (t *TransactionFramework) PrepareTransaction(outputCount int, inputAliases ...string) (consumedInputs utxoledger.Outputs, outputs iotago.Outputs[iotago.Output], signingWallets []*mock.HDWallet) {
	inputStates := make([]*utxoledger.Output, 0, len(inputAliases))
	totalInputDeposits := uint64(0)
	for _, inputAlias := range inputAliases {
		output := t.Output(inputAlias)
		inputStates = append(inputStates, output)
		totalInputDeposits += output.Deposit()
	}

	tokenAmount := totalInputDeposits / uint64(outputCount)
	remainderFunds := totalInputDeposits

	outputStates := make(iotago.Outputs[iotago.Output], 0, outputCount)
	for i := 0; i < outputCount; i++ {
		if i+1 == outputCount {
			tokenAmount = remainderFunds
		}
		remainderFunds -= tokenAmount

		outputStates = append(outputStates, &iotago.BasicOutput{
			Amount: tokenAmount,
			Conditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: t.wallet.Address()},
			},
		})
	}

	return inputStates, outputStates, []*mock.HDWallet{t.wallet}
}

func (t *TransactionFramework) CreateTransactionWithInputsAndOutputs(consumedInputs utxoledger.Outputs, outputs iotago.Outputs[iotago.Output], signingWallets []*mock.HDWallet) (*iotago.Transaction, error) {
	walletKeys := make([]iotago.AddressKeys, len(signingWallets))
	for i, wallet := range signingWallets {
		inputPrivateKey, _ := wallet.KeyPair()
		walletKeys[i] = iotago.AddressKeys{Address: wallet.Address(), Keys: inputPrivateKey}
	}

	txBuilder := builder.NewTransactionBuilder(t.protoParams.NetworkID())
	for _, input := range consumedInputs {
		switch input.OutputType() {
		case iotago.OutputFoundry:
			// For foundries we need to unlock the alias
			txBuilder.AddInput(&builder.TxInput{
				UnlockTarget: input.Output().UnlockConditionSet().ImmutableAccount().Address,
				InputID:      input.OutputID(),
				Input: iotago.OutputWithCreationTime{
					Output:       input.Output(),
					CreationTime: input.CreationTime(),
				},
			})
		case iotago.OutputAccount:
			// For alias we need to unlock the state controller
			txBuilder.AddInput(&builder.TxInput{
				UnlockTarget: input.Output().UnlockConditionSet().StateControllerAddress().Address,
				InputID:      input.OutputID(),
				Input: iotago.OutputWithCreationTime{
					Output:       input.Output(),
					CreationTime: input.CreationTime(),
				},
			})
		default:
			txBuilder.AddInput(&builder.TxInput{
				UnlockTarget: input.Output().UnlockConditionSet().Address().Address,
				InputID:      input.OutputID(),
				Input: iotago.OutputWithCreationTime{
					Output:       input.Output(),
					CreationTime: input.CreationTime(),
				},
			})
		}
	}

	for _, output := range outputs {
		txBuilder.AddOutput(output)
	}
	randomPayload := tpkg.Rand12ByteArray()
	txBuilder.AddTaggedDataPayload(&iotago.TaggedData{Tag: randomPayload[:], Data: randomPayload[:]})

	return txBuilder.Build(t.protoParams, iotago.NewInMemoryAddressSigner(walletKeys...))
}

func (t *TransactionFramework) Output(alias string) *utxoledger.Output {
	output, exists := t.states[alias]
	if !exists {
		panic(xerrors.Errorf("output with given alias does not exist %s", alias))
	}

	return output
}

func (t *TransactionFramework) OutputID(alias string) iotago.OutputID {
	return t.Output(alias).OutputID()
}

func (t *TransactionFramework) Transaction(alias string) *iotago.Transaction {
	transaction, exists := t.transactions[alias]
	if !exists {
		panic(xerrors.Errorf("transaction with given alias does not exist %s", alias))
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
