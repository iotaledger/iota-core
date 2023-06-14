package tpkg

import (
	"time"

	"github.com/iotaledger/iota-core/pkg/utils"
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
		MaxCommitableAge:      10,
	}
	api = iotago.LatestAPI(protocolParams)
)

func ProtocolParams() *iotago.ProtocolParameters {
	return protocolParams
}

func API() iotago.API {
	return api
}
