package autopeering

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/discovery/util"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/network/p2p"
)

type Manager struct {
	networkID        string
	p2pManager       *p2p.Manager
	log              *logger.Logger
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
func NewManager(networkID string, p2pManager *p2p.Manager, host host.Host, peerDB *network.DB, log *logger.Logger) *Manager {
	return &Manager{
		networkID:  networkID,
		p2pManager: p2pManager,
		host:       host,
		peerDB:     peerDB,
		log:        log,
	}
}

// Start starts the autopeering manager.
func (m *Manager) Start(ctx context.Context) (err error) {
	//nolint:contextcheck
	m.startOnce.Do(func() {
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
				m.log.Infoln("Failed to connect to bootstrap node:", seedPeer, innerErr)
				continue
			}

			if _, innerErr := kademliaDHT.RoutingTable().TryAddPeer(addrInfo.ID, true, true); innerErr != nil {
				m.log.Warnln("Failed to add bootstrap node to routing table:", innerErr)
				continue
			}

			m.log.Debugln("Connected to bootstrap node:", seedPeer)
		}

		m.routingDiscovery = routing.NewRoutingDiscovery(kademliaDHT)
		util.Advertise(m.ctx, m.routingDiscovery, m.networkID, discovery.TTL(5*time.Minute))

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
	tctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
	defer cancel()

	m.log.Debugf("Discovering peers for network ID %s", m.networkID)
	peerChan, err := m.routingDiscovery.FindPeers(tctx, m.networkID)
	if err != nil {
		m.log.Warnf("Failed to find peers: %s", err)
	}

	for peerAddrInfo := range peerChan {
		// Do not self-dial.
		if peerAddrInfo.ID == m.host.ID() {
			continue
		}

		m.log.Debugf("Found peer: %s", peerAddrInfo)

		peer := network.NewPeerFromAddrInfo(&peerAddrInfo)
		if err := m.p2pManager.DialPeer(m.ctx, peer); err != nil {
			if ierrors.Is(err, p2p.ErrDuplicateNeighbor) {
				m.log.Debugf("Already connected to peer %s", peer)
				continue
			}
			m.log.Warnf("Failed to dial peer %s: %s", peer, err)
		}
	}
}
