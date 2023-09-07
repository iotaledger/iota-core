package manualpeering

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/network/p2p"
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
	p2pm              *p2p.Manager
	log               *logger.Logger
	startOnce         sync.Once
	isStarted         atomic.Bool
	stopOnce          sync.Once
	stopMutex         syncutils.RWMutex
	isStopped         bool
	reconnectInterval time.Duration
	knownPeersMutex   syncutils.RWMutex
	knownPeers        map[peer.ID]*network.Peer
	workerPool        *workerpool.WorkerPool

	onGossipNeighborRemovedHook *event.Hook[func(*p2p.Neighbor)]
	onGossipNeighborAddedHook   *event.Hook[func(*p2p.Neighbor)]
}

// NewManager initializes a new Manager instance.
func NewManager(p2pm *p2p.Manager, workerPool *workerpool.WorkerPool, log *logger.Logger) *Manager {
	m := &Manager{
		p2pm:              p2pm,
		log:               log,
		reconnectInterval: network.DefaultReconnectInterval,
		knownPeers:        make(map[peer.ID]*network.Peer),
		workerPool:        workerPool,
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
	if err := m.p2pm.DropNeighbor(peerID); err != nil && !ierrors.Is(err, p2p.ErrUnknownNeighbor) {
		return ierrors.Wrapf(err, "failed to drop known peer %s in the gossip layer", peerID)
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
		m.workerPool.Start()
		m.onGossipNeighborRemovedHook = m.p2pm.Events.NeighborRemoved.Hook(func(neighbor *p2p.Neighbor) {
			m.onGossipNeighborRemoved(neighbor)
		}, event.WithWorkerPool(m.workerPool))
		m.onGossipNeighborAddedHook = m.p2pm.Events.NeighborAdded.Hook(func(neighbor *p2p.Neighbor) {
			m.onGossipNeighborAdded(neighbor)
		}, event.WithWorkerPool(m.workerPool))
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
	if p.ID == m.p2pm.P2PHost().ID() {
		return ierrors.New("not adding self to the list of known peers")
	}

	if _, exists := m.knownPeers[p.ID]; exists {
		return nil
	}
	m.log.Infof("Adding new peer to the list of known peers in manual peering %s", p)
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
			m.log.Infow("Peer is disconnected, calling gossip layer to establish the connection", "peer", peer.ID)

			var err error
			if err = m.p2pm.DialPeer(ctx, peer); err != nil && !ierrors.Is(err, p2p.ErrDuplicateNeighbor) && !ierrors.Is(err, context.Canceled) {
				m.log.Errorw("Failed to connect a neighbor in the gossip layer", "peerID", peer.ID, "err", err)
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

func (m *Manager) onGossipNeighborRemoved(neighbor *p2p.Neighbor) {
	m.changeNeighborStatus(neighbor, network.ConnStatusDisconnected)
}

func (m *Manager) onGossipNeighborAdded(neighbor *p2p.Neighbor) {
	m.changeNeighborStatus(neighbor, network.ConnStatusConnected)
	m.log.Infow("Gossip layer successfully connected with the peer", "peer", neighbor.Peer)
}

func (m *Manager) changeNeighborStatus(neighbor *p2p.Neighbor, connStatus network.ConnectionStatus) {
	m.knownPeersMutex.RLock()
	defer m.knownPeersMutex.RUnlock()

	kp, exists := m.knownPeers[neighbor.ID]
	if !exists {
		return
	}
	kp.SetConnStatus(connStatus)
}
