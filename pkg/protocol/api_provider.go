package protocol

import (
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
)

// APIProvider is a protocol component that exposes the methods to comply with the iotago.APIProvider interface.
type APIProvider struct {
	// Protocol is the protocol instance.
	*Protocol
}

// NewAPIProvider creates a new APIProvider.
func NewAPIProvider(protocol *Protocol) *APIProvider {
	return &APIProvider{Protocol: protocol}
}

// APIForVersion returns the API for the given version.
func (a *APIProvider) APIForVersion(version iotago.Version) (api iotago.API, err error) {
	if mainEngineInstance := a.Engines.Main.Get(); mainEngineInstance != nil {
		return mainEngineInstance.APIForVersion(version)
	}

	return nil, ierrors.New("no engine instance available")
}

// APIForSlot returns the API for the given slot.
func (a *APIProvider) APIForSlot(slot iotago.SlotIndex) iotago.API {
	return a.Engines.Main.Get().APIForSlot(slot)
}

// APIForEpoch returns the API for the given epoch.
func (a *APIProvider) APIForEpoch(epoch iotago.EpochIndex) iotago.API {
	return a.Engines.Main.Get().APIForEpoch(epoch)
}

// APIForTime returns the API for the given time.
func (a *APIProvider) APIForTime(t time.Time) iotago.API {
	return a.Engines.Main.Get().APIForTime(t)
}

// CommittedAPI returns the API for the committed state.
func (a *APIProvider) CommittedAPI() iotago.API {
	return a.Engines.Main.Get().CommittedAPI()
}

// LatestAPI returns the latest API.
func (a *APIProvider) LatestAPI() iotago.API {
	return a.Engines.Main.Get().LatestAPI()
}
