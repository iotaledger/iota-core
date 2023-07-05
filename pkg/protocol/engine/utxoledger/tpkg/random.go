package tpkg

import (
	"github.com/iotaledger/iota-core/pkg/core/api"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func API() iotago.API {
	return tpkg.TestAPI
}

func RandLedgerStateOutput() *utxoledger.Output {
	return RandLedgerStateOutputWithType(utils.RandOutputType())
}

func RandLedgerStateOutputWithType(outputType iotago.OutputType) *utxoledger.Output {
	return utxoledger.CreateOutput(api.TestAPIProvider, utils.RandOutputID(), utils.RandBlockID(), utils.RandSlotIndex(), utils.RandSlotIndex(), utils.RandOutput(outputType))
}

func RandLedgerStateOutputOnAddress(outputType iotago.OutputType, address iotago.Address) *utxoledger.Output {
	return utxoledger.CreateOutput(api.TestAPIProvider, utils.RandOutputID(), utils.RandBlockID(), utils.RandSlotIndex(), utils.RandSlotIndex(), utils.RandOutputOnAddress(outputType, address))
}

func RandLedgerStateOutputOnAddressWithAmount(outputType iotago.OutputType, address iotago.Address, amount iotago.BaseToken) *utxoledger.Output {
	return utxoledger.CreateOutput(api.TestAPIProvider, utils.RandOutputID(), utils.RandBlockID(), utils.RandSlotIndex(), utils.RandSlotIndex(), utils.RandOutputOnAddressWithAmount(outputType, address, amount))
}

func RandLedgerStateSpent(indexSpent iotago.SlotIndex) *utxoledger.Spent {
	return utxoledger.NewSpent(RandLedgerStateOutput(), utils.RandTransactionID(), indexSpent)
}

func RandLedgerStateSpentWithOutput(output *utxoledger.Output, indexSpent iotago.SlotIndex) *utxoledger.Spent {
	return utxoledger.NewSpent(output, utils.RandTransactionID(), indexSpent)
}
