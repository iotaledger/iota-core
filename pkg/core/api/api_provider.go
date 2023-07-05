package api

import iotago "github.com/iotaledger/iota.go/v4"

type Provider interface {
	APIForVersion(byte) iotago.API
	APIForSlot(iotago.SlotIndex) iotago.API
	APIForEpoch(iotago.EpochIndex) iotago.API

	LatestAPI() iotago.API
}
