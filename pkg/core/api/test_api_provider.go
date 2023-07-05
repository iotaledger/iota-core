package api

import (
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type testAPIProvider struct{}

var TestAPIProvider Provider = &testAPIProvider{}

func (t *testAPIProvider) APIForVersion(byte) iotago.API {
	return tpkg.TestAPI
}

func (t *testAPIProvider) APIForSlot(iotago.SlotIndex) iotago.API {
	return tpkg.TestAPI
}

func (t *testAPIProvider) APIForEpoch(iotago.EpochIndex) iotago.API {
	return tpkg.TestAPI
}

func (t *testAPIProvider) LatestAPI() iotago.API {
	return tpkg.TestAPI
}
