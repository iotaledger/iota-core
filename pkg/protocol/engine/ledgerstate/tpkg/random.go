package tpkg

import (
	"github.com/iotaledger/iota-core/pkg/utils"
	"time"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate"
	iotago "github.com/iotaledger/iota.go/v4"
)

var (
	protocolParams = &iotago.ProtocolParameters{
		Version:     3,
		NetworkName: utils.RandString(255),
		Bech32HRP:   iotago.NetworkPrefix(utils.RandString(3)),
		MinPoWScore: utils.RandUint32(50000),
		RentStructure: iotago.RentStructure{
			VByteCost:    100,
			VBFactorData: 1,
			VBFactorKey:  10,
		},
		TokenSupply:           utils.RandAmount(),
		GenesisUnixTimestamp:  uint32(time.Now().Unix()),
		SlotDurationInSeconds: 10,
	}
	api = iotago.LatestAPI(protocolParams)
)

func ProtocolParams() *iotago.ProtocolParameters {
	return protocolParams
}

func API() iotago.API {
	return api
}

func RandLedgerStateOutput() *ledgerstate.Output {
	return RandLedgerStateOutputWithType(utils.RandOutputType())
}

func RandLedgerStateOutputWithType(outputType iotago.OutputType) *ledgerstate.Output {
	return ledgerstate.CreateOutput(api, utils.RandOutputID(), utils.RandBlockID(), utils.RandSlotIndex(), utils.RandSlotIndex(), utils.RandOutput(outputType))
}

func RandLedgerStateOutputOnAddress(outputType iotago.OutputType, address iotago.Address) *ledgerstate.Output {
	return ledgerstate.CreateOutput(api, utils.RandOutputID(), utils.RandBlockID(), utils.RandSlotIndex(), utils.RandSlotIndex(), utils.RandOutputOnAddress(outputType, address))
}

func RandLedgerStateOutputOnAddressWithAmount(outputType iotago.OutputType, address iotago.Address, amount uint64) *ledgerstate.Output {
	return ledgerstate.CreateOutput(api, utils.RandOutputID(), utils.RandBlockID(), utils.RandSlotIndex(), utils.RandSlotIndex(), utils.RandOutputOnAddressWithAmount(outputType, address, amount))
}

func RandLedgerStateSpent(indexSpent iotago.SlotIndex, timestampSpent time.Time) *ledgerstate.Spent {
	return ledgerstate.NewSpent(RandLedgerStateOutput(), utils.RandTransactionID(), indexSpent)
}

func RandLedgerStateSpentWithOutput(output *ledgerstate.Output, indexSpent iotago.SlotIndex, timestampSpent time.Time) *ledgerstate.Spent {
	return ledgerstate.NewSpent(output, utils.RandTransactionID(), indexSpent)
}
