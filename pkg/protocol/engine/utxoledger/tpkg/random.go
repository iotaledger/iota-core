package tpkg

import (
	"math"
	"time"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
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
		TokenSupply:           tpkg.RandBaseToken(math.MaxUint64),
		GenesisUnixTimestamp:  time.Now().Unix(),
		SlotDurationInSeconds: 10,
		EvictionAge:           6,
		LivenessThreshold:     3,
	}
	api = iotago.LatestAPI(protocolParams)
)

func ProtocolParams() *iotago.ProtocolParameters {
	return protocolParams
}

func API() iotago.API {
	return api
}

func RandLedgerStateOutput() *utxoledger.Output {
	return RandLedgerStateOutputWithType(utils.RandOutputType())
}

func RandLedgerStateOutputWithType(outputType iotago.OutputType) *utxoledger.Output {
	return utxoledger.CreateOutput(api, utils.RandOutputID(), utils.RandBlockID(), utils.RandSlotIndex(), utils.RandSlotIndex(), utils.RandOutput(outputType))
}

func RandLedgerStateOutputOnAddress(outputType iotago.OutputType, address iotago.Address) *utxoledger.Output {
	return utxoledger.CreateOutput(api, utils.RandOutputID(), utils.RandBlockID(), utils.RandSlotIndex(), utils.RandSlotIndex(), utils.RandOutputOnAddress(outputType, address))
}

func RandLedgerStateOutputOnAddressWithAmount(outputType iotago.OutputType, address iotago.Address, amount iotago.BaseToken) *utxoledger.Output {
	return utxoledger.CreateOutput(api, utils.RandOutputID(), utils.RandBlockID(), utils.RandSlotIndex(), utils.RandSlotIndex(), utils.RandOutputOnAddressWithAmount(outputType, address, amount))
}

func RandLedgerStateSpent(indexSpent iotago.SlotIndex) *utxoledger.Spent {
	return utxoledger.NewSpent(RandLedgerStateOutput(), utils.RandTransactionID(), indexSpent)
}

func RandLedgerStateSpentWithOutput(output *utxoledger.Output, indexSpent iotago.SlotIndex) *utxoledger.Spent {
	return utxoledger.NewSpent(output, utils.RandTransactionID(), indexSpent)
}
