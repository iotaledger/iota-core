package manualpeering

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/network"
)

const (
	manualPeerProtectionTag = "manual-peering"
)

// Manager is the core entity in the manual peering package.
// It holds a list of known peers and constantly provisions it to the gossip layer.
// Its job is to keep in sync the list of known peers
// and the list of current manual neighbors connected in the gossip layer.
// If a new peer is added to known peers, manager will forward it to gossip and make sure it establishes a connection.
// And vice versa, if a peer is being removed from the list of known peers,
// manager will make sure gossip drops that connection.
// Manager also subscribes to the gossip events and in case the connection with a manual peer fails it will reconnect.
type Manager struct {
	networkManager    network.Manager
	logger            log.Logger
	startOnce         sync.Once
	isStarted         atomic.Bool
	stopOnce          sync.Once
	stopMutex         syncutils.RWMutex
	isStopped         bool
	reconnectInterval time.Duration
	knownPeersMutex   syncutils.RWMutex
	knownPeers        map[peer.ID]*network.Peer

	onGossipNeighborRemovedHook *event.Hook[func(network.Neighbor)]
	onGossipNeighborAddedHook   *event.Hook[func(network.Neighbor)]
}

// NewManager initializes a new Manager instance.
func NewManager(networkManager network.Manager, logger log.Logger) *Manager {
	m := &Manager{
		networkManager:    networkManager,
		logger:            logger.NewChildLogger("ManualPeering"),
		reconnectInterval: network.DefaultReconnectInterval,
		knownPeers:        make(map[peer.ID]*network.Peer),
	}

	return m
}

// AddPeers adds multiple peers to the list of known peers.
func (m *Manager) AddPeers(peerAddrs ...multiaddr.Multiaddr) error {
	var resultErr error
	for _, peerAddr := range peerAddrs {
		if err := m.addPeer(peerAddr); err != nil {
			resultErr = err
		}
	}

	return resultErr
}

// RemovePeer removes a peer from the list of known peers.
func (m *Manager) RemovePeer(peerID peer.ID) error {
	m.knownPeersMutex.Lock()

	kp, exists := m.knownPeers[peerID]
	if !exists {
		m.knownPeersMutex.Unlock()

		return nil
	}
	delete(m.knownPeers, peerID)
	close(kp.RemoveCh)

	m.knownPeersMutex.Unlock()

	<-kp.DoneCh

	m.networkManager.P2PHost().ConnManager().Unprotect(peerID, manualPeerProtectionTag)

	if err := m.networkManager.DropNeighbor(peerID); err != nil && !ierrors.Is(err, network.ErrUnknownPeer) {
		return ierrors.Wrapf(err, "failed to drop known peer %s in the gossip layer", peerID.String())
	}

	return nil
}

// GetPeersConfig holds optional parameters for the GetPeers method.
type GetPeersConfig struct {
	// If true, GetPeers returns peers that have actual connection established in the gossip layer.
	OnlyConnected bool `json:"onlyConnected"`
}

// GetPeersOption defines a single option for GetPeers method.
type GetPeersOption func(conf *GetPeersConfig)

// BuildGetPeersConfig builds GetPeersConfig struct from a list of options.
func BuildGetPeersConfig(opts []GetPeersOption) *GetPeersConfig {
	conf := &GetPeersConfig{}
	for _, o := range opts {
		o(conf)
	}

	return conf
}

// ToOptions translates config struct to a list of corresponding options.
func (c *GetPeersConfig) ToOptions() (opts []GetPeersOption) {
	if c.OnlyConnected {
		opts = append(opts, WithOnlyConnectedPeers())
	}

	return opts
}

// WithOnlyConnectedPeers returns a GetPeersOption that sets OnlyConnected field to true.
func WithOnlyConnectedPeers() GetPeersOption {
	return func(conf *GetPeersConfig) {
		conf.OnlyConnected = true
	}
}

// GetPeers returns the list of known peers.
func (m *Manager) GetPeers(opts ...GetPeersOption) []*network.PeerDescriptor {
	conf := BuildGetPeersConfig(opts)
	m.knownPeersMutex.RLock()
	defer m.knownPeersMutex.RUnlock()

	peers := make([]*network.PeerDescriptor, 0, len(m.knownPeers))
	for _, kp := range m.knownPeers {
		connStatus := kp.GetConnStatus()
		if !conf.OnlyConnected || connStatus == network.ConnStatusConnected {
			peers = append(peers, &network.PeerDescriptor{
				Addresses: kp.PeerAddresses,
			})
		}
	}

	return peers
}

// Start subscribes to the gossip layer events and starts internal background workers.
// Calling multiple times has no effect.
func (m *Manager) Start() {
	m.startOnce.Do(func() {
		m.onGossipNeighborRemovedHook = m.networkManager.OnNeighborRemoved(func(neighbor network.Neighbor) {
			m.onGossipNeighborRemoved(neighbor)
		})
		m.onGossipNeighborAddedHook = m.networkManager.OnNeighborAdded(func(neighbor network.Neighbor) {
			m.onGossipNeighborAdded(neighbor)
		})
		m.isStarted.Store(true)
	})
}

// Stop terminates internal background workers. Calling multiple times has no effect.
func (m *Manager) Stop() (err error) {
	if !m.isStarted.Load() {
		return ierrors.New("can't stop the manager: it hasn't been started yet")
	}
	m.stopOnce.Do(func() {
		m.stopMutex.Lock()
		defer m.stopMutex.Unlock()

		m.isStopped = true
		err = ierrors.WithStack(m.removeAllKnownPeers())
		m.onGossipNeighborRemovedHook.Unhook()
		m.onGossipNeighborAddedHook.Unhook()
	})

	return err
}

func (m *Manager) IsPeerKnown(id peer.ID) bool {
	m.knownPeersMutex.RLock()
	defer m.knownPeersMutex.RUnlock()

	_, exists := m.knownPeers[id]

	return exists
}

func (m *Manager) addPeer(peerAddr multiaddr.Multiaddr) error {
	if !m.isStarted.Load() {
		return ierrors.New("manual peering manager hasn't been started yet")
	}

	if m.isStopped {
		return ierrors.New("manual peering manager was stopped")
	}

	m.knownPeersMutex.Lock()
	defer m.knownPeersMutex.Unlock()

	p, err := network.NewPeerFromMultiAddr(peerAddr)
	if err != nil {
		return ierrors.WithStack(err)
	}

	// Do not add self
	if p.ID == m.networkManager.P2PHost().ID() {
		return ierrors.New("not adding self to the list of known peers")
	}

	if _, exists := m.knownPeers[p.ID]; exists {
		return nil
	}
	m.logger.LogInfof("Adding new peer to the list of known peers in manual peering %s", p)
	m.knownPeers[p.ID] = p
	go func() {
		defer close(p.DoneCh)
		m.keepPeerConnected(p)
	}()

	return nil
}

func (m *Manager) removeAllKnownPeers() error {
	var resultErr error
	for peerID := range m.knownPeers {
		if err := m.RemovePeer(peerID); err != nil {
			resultErr = err
		}
	}

	return resultErr
}

func (m *Manager) keepPeerConnected(peer *network.Peer) {
	ctx, ctxCancel := context.WithCancel(context.Background())
	cancelContextOnRemove := func() {
		<-peer.RemoveCh
		ctxCancel()
	}
	go cancelContextOnRemove()

	ticker := time.NewTicker(m.reconnectInterval)
	defer ticker.Stop()

	for {
		if peer.GetConnStatus() == network.ConnStatusDisconnected {
			m.logger.LogInfof("Peer is disconnected, calling gossip layer to establish the connection, peerID: %s", peer.ID)

			var err error
			if err = m.networkManager.DialPeer(ctx, peer); err != nil && !ierrors.Is(err, network.ErrDuplicatePeer) && !ierrors.Is(err, context.Canceled) {
				m.logger.LogErrorf("Failed to connect a neighbor in the gossip layer, peerID: %s, error: %s", peer.ID.String(), err.Error())
			}
		}
		select {
		case <-ticker.C:
		case <-peer.RemoveCh:
			<-ctx.Done()
			return
		}
	}
}

func (m *Manager) onGossipNeighborRemoved(neighbor network.Neighbor) {
	m.changeNeighborStatus(neighbor)
}

func (m *Manager) onGossipNeighborAdded(neighbor network.Neighbor) {
	m.changeNeighborStatus(neighbor)
	m.logger.LogInfof("Gossip layer successfully connected with the peer %s", neighbor.Peer())
}

func (m *Manager) changeNeighborStatus(neighbor network.Neighbor) {
	m.knownPeersMutex.RLock()
	defer m.knownPeersMutex.RUnlock()

	kp, exists := m.knownPeers[neighbor.Peer().ID]
	if !exists {
		return
	}
	kp.SetConnStatus(neighbor.Peer().GetConnStatus())
	m.networkManager.P2PHost().ConnManager().Protect(neighbor.Peer().ID, manualPeerProtectionTag)
}
