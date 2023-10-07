package protocol

import iotago "github.com/iotaledger/iota.go/v4"

// APIProvider is a protocol component that exposes the methods to comply with the iotago.APIProvider interface.
type APIProvider struct {
	// Protocol is the protocol instance.
	*Protocol
}

// NewApiProvider creates a new APIProvider.
func NewApiProvider(protocol *Protocol) *APIProvider {
	return &APIProvider{Protocol: protocol}
}

// APIForVersion returns the API for the given version.
func (a *APIProvider) APIForVersion(version iotago.Version) (api iotago.API, err error) {
	return a.MainEngineInstance().APIForVersion(version)
}

// APIForSlot returns the API for the given slot.
func (a *APIProvider) APIForSlot(slot iotago.SlotIndex) iotago.API {
	return a.MainEngineInstance().APIForSlot(slot)
}

// APIForEpoch returns the API for the given epoch.
func (a *APIProvider) APIForEpoch(epoch iotago.EpochIndex) iotago.API {
	return a.MainEngineInstance().APIForEpoch(epoch)
}

// CurrentAPI returns the current API.
func (a *APIProvider) CurrentAPI() iotago.API {
	return a.MainEngineInstance().CurrentAPI()
}

// LatestAPI returns the latest API.
func (a *APIProvider) LatestAPI() iotago.API {
	return a.MainEngineInstance().LatestAPI()
}
