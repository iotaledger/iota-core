package p2p

import (
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/iotaledger/hive.go/ds/onchangemap"
	"github.com/iotaledger/hive.go/lo"
)

// ConfigManager handles the list of peers that are stored in the peering config.
// It calls a function if the list changed.
type ConfigManager struct {
	onChangeMap *onchangemap.OnChangeMap[string, *ComparablePeerID, *PeerConfigItem]
}

// NewConfigManager creates a new config manager.
func NewConfigManager(storeCallback func([]*PeerConfigItem) error) *ConfigManager {
	return &ConfigManager{
		onChangeMap: onchangemap.NewOnChangeMap(
			onchangemap.WithChangedCallback[string, *ComparablePeerID](storeCallback),
		),
	}
}

// Peers returns all known peers.
func (pm *ConfigManager) Peers() []*PeerConfigItem {
	return lo.Values(pm.onChangeMap.All())
}

// AddPeer adds a peer to the config manager.
func (pm *ConfigManager) AddPeer(multiAddress multiaddr.Multiaddr, alias string) error {
	comparablePeerConfig, err := NewPeerConfigItem(&PeerConfig{
		MultiAddress: multiAddress.String(),
		Alias:        alias,
	})
	if err != nil {
		return err
	}

	if err := pm.onChangeMap.Add(comparablePeerConfig); err != nil {
		// already exists, modify the existing
		_, err := pm.onChangeMap.Modify(comparablePeerConfig.ID(), func(item *PeerConfigItem) bool {
			*item = *comparablePeerConfig
			return true
		})

		return err
	}

	return nil
}

// RemovePeer removes a peer from the config manager.
func (pm *ConfigManager) RemovePeer(peerID peer.ID) error {
	return pm.onChangeMap.Delete(NewComparablePeerID(peerID))
}

// StoreOnChange sets whether storing changes to the config is active or not.
func (pm *ConfigManager) StoreOnChange(enabled bool) {
	pm.onChangeMap.CallbacksEnabled(enabled)
}

// Store calls the storeCallback if storeOnChange is active.
func (pm *ConfigManager) Store() error {
	return pm.onChangeMap.ExecuteChangedCallback()
}
