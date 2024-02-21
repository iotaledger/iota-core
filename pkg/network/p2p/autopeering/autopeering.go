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
}

// NewManager creates a new autopeering manager.
func NewManager(maxPeers int, networkManager network.Manager, host host.Host, peerDB *network.DB, logger log.Logger) *Manager {
	return &Manager{
		maxPeers:       maxPeers,
		networkManager: networkManager,
		host:           host,
		peerDB:         peerDB,
		logger:         logger,
	}
}

func (m *Manager) MaxNeighbors() int {
	return m.maxPeers
}

// Start starts the autopeering manager.
func (m *Manager) Start(ctx context.Context, networkID string) (err error) {
	//nolint:contextcheck
	m.startOnce.Do(func() {
		m.namespace = fmt.Sprintf("/iota/%s/1.0.0", networkID)
		m.ctx, m.stopFunc = context.WithCancel(ctx)
		kademliaDHT, innerErr := dht.New(m.ctx, m.host, dht.Mode(dht.ModeServer))
		if innerErr != nil {
			err = innerErr
			return
		}

		// Bootstrap the DHT. In the default configuration, this spawns a Background worker that will keep the
		// node connected to the bootstrap peers and will disconnect from peers that are not useful.
		if innerErr = kademliaDHT.Bootstrap(m.ctx); innerErr != nil {
			err = innerErr
			return
		}

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
		util.Advertise(m.ctx, m.routingDiscovery, m.namespace, discovery.TTL(5*time.Minute))

		go m.discoveryLoop()

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
	})

	return nil
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
	//peersToFind := m.maxPeers - len(m.p2pManager.AutopeeredNeighbors())
	//if peersToFind <= 0 {
	//	m.logger.LogDebugf("Enough autopeering peers connected, not discovering new ones")
	//	return
	//}

	findCtx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
	defer cancel()

	m.logger.LogDebugf("Discovering new peers for namespace %s", m.namespace)
	peerChan, err := m.routingDiscovery.FindPeers(findCtx, m.namespace)
	if err != nil {
		m.logger.LogWarnf("Failed to find peers: %s", err)
		return
	}

	for peerAddrInfo := range peerChan {
		//if peersToFind <= 0 {
		//	m.logger.LogDebugf("Enough new autopeering peers connected")
		//	return
		//}

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
		}
		//peersToFind--
	}
}
