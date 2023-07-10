package api

import (
	"fmt"
	"sync"

	iotago "github.com/iotaledger/iota.go/v4"
)

type DynamicMockAPIProvider struct {
	mutex                       sync.RWMutex
	protocolParametersByVersion map[iotago.Version]iotago.ProtocolParameters
	protocolVersions            *ProtocolEpochVersions

	latestVersionMutex sync.RWMutex
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

func (d *DynamicMockAPIProvider) APIForVersion(version iotago.Version) iotago.API {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	protocolParams, exists := d.protocolParametersByVersion[version]
	if !exists {
		panic(fmt.Sprintf("protocol parameters for version %d are not set", version))
	}

	return NewAnyAPI(protocolParams)
}

func (d *DynamicMockAPIProvider) APIForSlot(slot iotago.SlotIndex) iotago.API {
	epoch := d.LatestAPI().TimeProvider().EpochFromSlot(slot)
	return d.APIForVersion(d.protocolVersions.VersionForEpoch(epoch))
}

func (d *DynamicMockAPIProvider) APIForEpoch(epoch iotago.EpochIndex) iotago.API {
	return d.APIForVersion(d.protocolVersions.VersionForEpoch(epoch))
}

func (d *DynamicMockAPIProvider) LatestAPI() iotago.API {
	d.latestVersionMutex.RLock()
	defer d.latestVersionMutex.RUnlock()

	return d.APIForVersion(d.latestVersion)
}
