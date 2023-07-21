package api

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	iotago "github.com/iotaledger/iota.go/v4"
)

type DynamicMockAPIProvider struct {
	mutex                       syncutils.RWMutex
	protocolParametersByVersion map[iotago.Version]iotago.ProtocolParameters
	protocolVersions            *ProtocolEpochVersions

	latestVersionMutex syncutils.RWMutex
	latestVersion      iotago.Version
}

func NewDynamicMockAPIProvider() *DynamicMockAPIProvider {
	return &DynamicMockAPIProvider{
		protocolParametersByVersion: make(map[iotago.Version]iotago.ProtocolParameters),
		protocolVersions:            NewProtocolEpochVersions(),
	}
}

func (d *DynamicMockAPIProvider) AddProtocolParameters(epoch iotago.EpochIndex, protocolParameters iotago.ProtocolParameters) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.protocolParametersByVersion[protocolParameters.Version()] = protocolParameters
	d.protocolVersions.Add(protocolParameters.Version(), epoch)

	d.latestVersionMutex.Lock()
	defer d.latestVersionMutex.Unlock()

	if d.latestVersion < protocolParameters.Version() {
		d.latestVersion = protocolParameters.Version()
	}
}

func (d *DynamicMockAPIProvider) APIForVersion(version iotago.Version) (iotago.API, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	protocolParams, exists := d.protocolParametersByVersion[version]
	if !exists {
		return nil, ierrors.Errorf("protocol parameters for version %d are not set", version)
	}

	return NewAnyAPI(protocolParams), nil
}

func (d *DynamicMockAPIProvider) APIForSlot(slot iotago.SlotIndex) iotago.API {
	epoch := d.LatestAPI().TimeProvider().EpochFromSlot(slot)
	return lo.PanicOnErr(d.APIForVersion(d.protocolVersions.VersionForEpoch(epoch)))
}

func (d *DynamicMockAPIProvider) APIForEpoch(epoch iotago.EpochIndex) iotago.API {
	return lo.PanicOnErr(d.APIForVersion(d.protocolVersions.VersionForEpoch(epoch)))
}

func (d *DynamicMockAPIProvider) LatestAPI() iotago.API {
	d.latestVersionMutex.RLock()
	defer d.latestVersionMutex.RUnlock()

	return lo.PanicOnErr(d.APIForVersion(d.latestVersion))
}

func (d *DynamicMockAPIProvider) CurrentAPI() iotago.API {
	return d.LatestAPI()
}
