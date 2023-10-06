package protocol

import iotago "github.com/iotaledger/iota.go/v4"

// ApiProvider is a protocol component that exposes the methods to comply with the iotago.APIProvider interface.
type ApiProvider struct {
	// Protocol is the protocol instance.
	*Protocol
}

// newApiProvider creates a new ApiProvider.
func newApiProvider(protocol *Protocol) *ApiProvider {
	return &ApiProvider{Protocol: protocol}
}

// APIForVersion returns the API for the given version.
func (p *ApiProvider) APIForVersion(version iotago.Version) (api iotago.API, err error) {
	return p.MainEngineInstance().APIForVersion(version)
}

// APIForSlot returns the API for the given slot.
func (p *ApiProvider) APIForSlot(slot iotago.SlotIndex) iotago.API {
	return p.MainEngineInstance().APIForSlot(slot)
}

// APIForEpoch returns the API for the given epoch.
func (p *ApiProvider) APIForEpoch(epoch iotago.EpochIndex) iotago.API {
	return p.MainEngineInstance().APIForEpoch(epoch)
}

// CurrentAPI returns the current API.
func (p *ApiProvider) CurrentAPI() iotago.API {
	return p.MainEngineInstance().CurrentAPI()
}

// LatestAPI returns the latest API.
func (p *ApiProvider) LatestAPI() iotago.API {
	return p.MainEngineInstance().LatestAPI()
}
