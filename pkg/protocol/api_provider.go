package protocol

import iotago "github.com/iotaledger/iota.go/v4"

type APIProvider struct {
	protocol *Protocol
}

func newAPIProvider(protocol *Protocol) *APIProvider {
	return &APIProvider{
		protocol: protocol,
	}
}

// APIForVersion returns the API for the given version.
func (a *APIProvider) APIForVersion(version iotago.Version) (api iotago.API, err error) {
	return a.protocol.MainEngine().APIForVersion(version)
}

// APIForSlot returns the API for the given slot.
func (a *APIProvider) APIForSlot(slot iotago.SlotIndex) iotago.API {
	return a.protocol.MainEngine().APIForSlot(slot)
}

func (a *APIProvider) APIForEpoch(epoch iotago.EpochIndex) iotago.API {
	return a.protocol.MainEngine().APIForEpoch(epoch)
}

func (a *APIProvider) CurrentAPI() iotago.API {
	return a.protocol.MainEngine().CurrentAPI()
}

func (a *APIProvider) LatestAPI() iotago.API {
	return a.protocol.MainEngine().LatestAPI()
}
