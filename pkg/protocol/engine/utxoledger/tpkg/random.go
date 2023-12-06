package tpkg

import (
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func RandLedgerStateOutput() *utxoledger.Output {
	return RandLedgerStateOutputWithType(tpkg.RandOutputType())
}

func RandLedgerStateOutputWithOutput(output iotago.Output) *utxoledger.Output {
	outputs := iotago.TxEssenceOutputs{output}
	txID := tpkg.RandTransactionID()
	proof := lo.PanicOnErr(iotago.NewOutputIDProof(tpkg.ZeroCostTestAPI, txID.Identifier(), txID.Slot(), outputs, 0))

	return utxoledger.CreateOutput(iotago.SingleVersionProvider(tpkg.ZeroCostTestAPI), tpkg.RandOutputID(), tpkg.RandBlockID(), tpkg.RandSlot(), outputs[0], proof)
}

func RandLedgerStateOutputWithType(outputType iotago.OutputType) *utxoledger.Output {
	return RandLedgerStateOutputWithOutput(tpkg.RandOutput(outputType))
}

func RandLedgerStateOutputOnAddress(outputType iotago.OutputType, address iotago.Address) *utxoledger.Output {
	return RandLedgerStateOutputWithOutput(tpkg.RandOutputOnAddress(outputType, address))
}

func RandLedgerStateOutputOnAddressWithAmount(outputType iotago.OutputType, address iotago.Address, amount iotago.BaseToken) *utxoledger.Output {
	return RandLedgerStateOutputWithOutput(tpkg.RandOutputOnAddressWithAmount(outputType, address, amount))
}

func RandLedgerStateSpent(indexSpent iotago.SlotIndex) *utxoledger.Spent {
	return utxoledger.NewSpent(RandLedgerStateOutput(), tpkg.RandTransactionID(), indexSpent)
}

func RandLedgerStateSpentWithOutput(output *utxoledger.Output, indexSpent iotago.SlotIndex) *utxoledger.Spent {
	return utxoledger.NewSpent(output, tpkg.RandTransactionID(), indexSpent)
}
