package testsuite

import (
	"fmt"
	"time"

	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
)

type TransactionFramework struct {
	api         iotago.API
	protoParams *iotago.ProtocolParameters

	wallet *mock.HDWallet
	states map[string]*ledgerstate.Output
}

func NewTransactionFramework(protocol *protocol.Protocol, genesisSeed []byte) *TransactionFramework {
	genesisOutput, err := protocol.MainEngineInstance().Ledger.Output(iotago.OutputID{}.UTXOInput())
	if err != nil {
		panic(err)
	}

	return &TransactionFramework{
		api:         protocol.API(),
		protoParams: protocol.MainEngineInstance().Storage.Settings().ProtocolParameters(),
		states:      map[string]*ledgerstate.Output{"Genesis": genesisOutput},
		wallet:      mock.NewHDWallet("genesis", genesisSeed, 0),
	}
}

func (t *TransactionFramework) CreateTransaction(alias string, outputCount int, inputAliases ...string) (*iotago.Transaction, error) {
	inputStates := make([]*ledgerstate.Output, 0, len(inputAliases))
	totalInputDeposits := uint64(0)
	for _, inputAlias := range inputAliases {
		output := t.resolveOutputAlias(inputAlias)
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

	transaction, err := t.CreateTransactionWithInputsAndOutputs(inputStates, outputStates, []*mock.HDWallet{t.wallet})
	if err != nil {
		panic(err)
	}
	for idx, output := range outputStates {
		t.states[fmt.Sprintf("%s:%d", alias, idx)] = ledgerstate.CreateOutput(t.api, iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(transaction.ID()), uint16(idx)), iotago.EmptyBlockID(), 0, time.Now(), output)
	}

	return transaction, err
}

func (t *TransactionFramework) CreateTransactionWithInputsAndOutputs(consumedInputs ledgerstate.Outputs, outputs iotago.Outputs[iotago.Output], signingWallets []*mock.HDWallet) (*iotago.Transaction, error) {
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
			txBuilder.AddInput(&builder.TxInput{UnlockTarget: input.Output().UnlockConditionSet().ImmutableAlias().Address, InputID: input.OutputID(), Input: input.Output()})
		case iotago.OutputAlias:
			// For alias we need to unlock the state controller
			txBuilder.AddInput(&builder.TxInput{UnlockTarget: input.Output().UnlockConditionSet().StateControllerAddress().Address, InputID: input.OutputID(), Input: input.Output()})
		default:
			txBuilder.AddInput(&builder.TxInput{UnlockTarget: input.Output().UnlockConditionSet().Address().Address, InputID: input.OutputID(), Input: input.Output()})
		}
	}

	for _, output := range outputs {
		txBuilder.AddOutput(output)
	}

	return txBuilder.Build(t.protoParams, iotago.NewInMemoryAddressSigner(walletKeys...))
}

func (t *TransactionFramework) resolveOutputAlias(alias string) *ledgerstate.Output {
	output, exists := t.states[alias]
	if !exists {
		panic(xerrors.Errorf("given alias does not exist %s", alias))
	}

	return output
}
