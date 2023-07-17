package api

import iotago "github.com/iotaledger/iota.go/v4"

type Provider interface {
	APIForVersion(iotago.Version) (iotago.API, error)
	APIForSlot(iotago.SlotIndex) iotago.API
	APIForEpoch(iotago.EpochIndex) iotago.API

	LatestAPI() iotago.API
}
