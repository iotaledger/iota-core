package autopeering

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/discovery/util"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/iota-core/pkg/network"
)

type Manager struct {
	namespace        string
	maxPeers         int
	networkManager   network.Manager
	logger           log.Logger
	host             host.Host
	peerDB           *network.DB
	startOnce        sync.Once
	isStarted        atomic.Bool
	stopOnce         sync.Once
	ctx              context.Context
	stopFunc         context.CancelFunc
	routingDiscovery *routing.RoutingDiscovery

	advertiseLock   sync.Mutex
	advertiseCtx    context.Context
	advertiseCancel context.CancelFunc
}

// NewManager creates a new autopeering manager.
func NewManager(maxPeers int, networkManager network.Manager, host host.Host, peerDB *network.DB, logger log.Logger) *Manager {
	return &Manager{
		maxPeers:       maxPeers,
		networkManager: networkManager,
		host:           host,
		peerDB:         peerDB,
		logger:         logger.NewChildLogger("Autopeering"),
	}
}

func (m *Manager) MaxNeighbors() int {
	return m.maxPeers
}

// Start starts the autopeering manager.
func (m *Manager) Start(ctx context.Context, networkID string) (err error) {
	//nolint:contextcheck
	m.startOnce.Do(func() {
		// We will use /iota/networkID/kad/1.0.0 for the DHT protocol.
		// And /iota/networkID/iota-core/1.0.0 for the peer discovery.
		prefix := protocol.ID("/iota")
		extension := protocol.ID(fmt.Sprintf("/%s", networkID))
		m.namespace = fmt.Sprintf("%s%s/%s", prefix, extension, network.CoreProtocolID)
		dhtCtx, dhtCancel := context.WithCancel(ctx)
		kademliaDHT, innerErr := dht.New(dhtCtx, m.host, dht.Mode(dht.ModeServer), dht.ProtocolPrefix(prefix), dht.ProtocolExtension(extension))
		if innerErr != nil {
			err = innerErr
			dhtCancel()

			return
		}

		// Bootstrap the DHT. In the default configuration, this spawns a Background worker that will keep the
		// node connected to the bootstrap peers and will disconnect from peers that are not useful.
		if innerErr = kademliaDHT.Bootstrap(dhtCtx); innerErr != nil {
			err = innerErr
			dhtCancel()

			return
		}

		m.ctx = dhtCtx

		for _, seedPeer := range m.peerDB.SeedPeers() {
			addrInfo := seedPeer.ToAddrInfo()
			if innerErr := m.host.Connect(ctx, *addrInfo); innerErr != nil {
				m.logger.LogInfof("Failed to connect to bootstrap node, peer: %s, error: %s", seedPeer, innerErr)
				continue
			}

			if _, innerErr := kademliaDHT.RoutingTable().TryAddPeer(addrInfo.ID, true, true); innerErr != nil {
				m.logger.LogWarnf("Failed to add bootstrap node to routing table, error: %s", innerErr)
				continue
			}

			m.logger.LogDebugf("Connected to bootstrap node, peer: %s", seedPeer)
		}

		m.routingDiscovery = routing.NewRoutingDiscovery(kademliaDHT)
		m.startAdvertisingIfNeeded()
		go m.discoveryLoop()

		onGossipNeighborRemovedHook := m.networkManager.OnNeighborRemoved(func(_ network.Neighbor) {
			m.startAdvertisingIfNeeded()
		})
		onGossipNeighborAddedHook := m.networkManager.OnNeighborAdded(func(_ network.Neighbor) {
			m.stopAdvertisingItNotNeeded()
		})

		m.stopFunc = func() {
			dhtCancel()
			onGossipNeighborRemovedHook.Unhook()
			onGossipNeighborAddedHook.Unhook()
			m.stopAdvertising()
		}

		m.isStarted.Store(true)
	})

	return err
}

// Stop terminates internal background workers. Calling multiple times has no effect.
func (m *Manager) Stop() error {
	if !m.isStarted.Load() {
		return ierrors.New("can't stop the manager: it hasn't been started yet")
	}
	m.stopOnce.Do(func() {
		m.stopFunc()
		m.stopAdvertising()
	})

	return nil
}

func (m *Manager) startAdvertisingIfNeeded() {
	m.advertiseLock.Lock()
	defer m.advertiseLock.Unlock()

	if len(m.networkManager.AutopeeringNeighbors()) >= m.maxPeers {
		return
	}

	if m.advertiseCtx == nil && m.ctx.Err() == nil {
		m.logger.LogInfof("Start advertising for namespace %s", m.namespace)
		m.advertiseCtx, m.advertiseCancel = context.WithCancel(m.ctx)
		util.Advertise(m.advertiseCtx, m.routingDiscovery, m.namespace, discovery.TTL(time.Minute))
	}
}

func (m *Manager) stopAdvertisingItNotNeeded() {
	if len(m.networkManager.AutopeeringNeighbors()) >= m.maxPeers {
		m.stopAdvertising()
	}
}

func (m *Manager) stopAdvertising() {
	m.advertiseLock.Lock()
	defer m.advertiseLock.Unlock()

	if m.advertiseCancel != nil {
		m.logger.LogInfof("Stop advertising for namespace %s", m.namespace)
		m.advertiseCancel()
		m.advertiseCtx = nil
		m.advertiseCancel = nil
	}
}

func (m *Manager) discoveryLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	m.discoverAndDialPeers()

	for {
		select {
		case <-ticker.C:
			m.discoverAndDialPeers()
		case <-m.ctx.Done():
			return
		}
	}
}

func (m *Manager) discoverAndDialPeers() {
	autopeeringNeighbors := m.networkManager.AutopeeringNeighbors()
	peersToFind := m.maxPeers - len(autopeeringNeighbors)
	if peersToFind == 0 {
		m.logger.LogDebugf("%d autopeering peers connected, not discovering new ones. (max %d)", len(autopeeringNeighbors), m.maxPeers)
		return
	}

	if peersToFind < 0 {
		m.logger.LogDebugf("Too many autopeering peers connected %d, disconnecting some", -peersToFind)
		for i := peersToFind; i < 0; i++ {
			if err := m.networkManager.DropNeighbor(autopeeringNeighbors[i].Peer().ID); err != nil {
				m.logger.LogDebugf("Failed to disconnect neighbor %s", autopeeringNeighbors[i].Peer().ID)
			}
		}

		return
	}

	findCtx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
	defer cancel()

	m.logger.LogDebugf("%d autopeering peers connected. Discovering new peers for namespace %s", len(autopeeringNeighbors), m.namespace)
	peerChan, err := m.routingDiscovery.FindPeers(findCtx, m.namespace)
	if err != nil {
		m.logger.LogWarnf("Failed to find peers: %s", err)
		return
	}

	for peerAddrInfo := range peerChan {
		if m.ctx.Err() != nil {
			m.logger.LogDebug("Context is done, stopping dialing new peers")
			return
		}

		if peersToFind <= 0 {
			m.logger.LogDebug("Enough new autopeering peers connected")
			return
		}

		// Do not self-dial.
		if peerAddrInfo.ID == m.host.ID() {
			continue
		}

		// Do not try to dial already connected peers.
		if m.networkManager.NeighborExists(peerAddrInfo.ID) {
			continue
		}

		m.logger.LogDebugf("Found peer: %s", peerAddrInfo)

		peer := network.NewPeerFromAddrInfo(&peerAddrInfo)
		if err := m.networkManager.DialPeer(m.ctx, peer); err != nil {
			m.logger.LogWarnf("Failed to dial peer %s: %s", peerAddrInfo, err)
		} else {
			peersToFind--
		}
	}
}
