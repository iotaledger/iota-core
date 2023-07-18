package api

import (
	iotago "github.com/iotaledger/iota.go/v4"
)

func NewStaticProvider(api iotago.API) Provider {
	return &staticAPIProvider{api: api}
}

type staticAPIProvider struct {
	api iotago.API
}

func (t *staticAPIProvider) APIForVersion(iotago.Version) (iotago.API, error) {
	return t.api, nil
}

func (t *staticAPIProvider) APIForSlot(iotago.SlotIndex) iotago.API {
	return t.api
}

func (t *staticAPIProvider) APIForEpoch(iotago.EpochIndex) iotago.API {
	return t.api
}

func (t *staticAPIProvider) LatestAPI() iotago.API {
	return t.api
}

func (t *staticAPIProvider) CurrentAPI() iotago.API {
	return t.api
}
