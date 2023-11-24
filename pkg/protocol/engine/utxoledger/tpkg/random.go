package tpkg

import (
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func RandLedgerStateOutput() *utxoledger.Output {
	return RandLedgerStateOutputWithType(utils.RandOutputType())
}

func RandLedgerStateOutputWithOutput(output iotago.Output) *utxoledger.Output {
	outputs := iotago.TxEssenceOutputs{output}
	txID := utils.RandTransactionID()
	proof := lo.PanicOnErr(iotago.NewOutputIDProof(tpkg.TestAPI, txID.Identifier(), txID.Slot(), outputs, 0))

	return utxoledger.CreateOutput(iotago.SingleVersionProvider(tpkg.TestAPI), utils.RandOutputID(), utils.RandBlockID(), utils.RandSlotIndex(), outputs[0], proof)
}

func RandLedgerStateOutputWithType(outputType iotago.OutputType) *utxoledger.Output {
	return RandLedgerStateOutputWithOutput(utils.RandOutput(outputType))
}

func RandLedgerStateOutputOnAddress(outputType iotago.OutputType, address iotago.Address) *utxoledger.Output {
	return RandLedgerStateOutputWithOutput(utils.RandOutputOnAddress(outputType, address))
}

func RandLedgerStateOutputOnAddressWithAmount(outputType iotago.OutputType, address iotago.Address, amount iotago.BaseToken) *utxoledger.Output {
	return RandLedgerStateOutputWithOutput(utils.RandOutputOnAddressWithAmount(outputType, address, amount))
}

func RandLedgerStateSpent(indexSpent iotago.SlotIndex) *utxoledger.Spent {
	return utxoledger.NewSpent(RandLedgerStateOutput(), utils.RandTransactionID(), indexSpent)
}

func RandLedgerStateSpentWithOutput(output *utxoledger.Output, indexSpent iotago.SlotIndex) *utxoledger.Spent {
	return utxoledger.NewSpent(output, utils.RandTransactionID(), indexSpent)
}
