package autopeering

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/discovery/util"

	p2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/autopeering/peer"
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
	peerDB           *peer.DB
	startOnce        sync.Once
	isStarted        atomic.Bool
	stopOnce         sync.Once
	ctx              context.Context
	stopFunc         context.CancelFunc
	isStopped        bool
	routingDiscovery *routing.RoutingDiscovery
}

func NewManager(networkID string, p2pManager *p2p.Manager, host host.Host, peerDB *peer.DB, log *logger.Logger) *Manager {
	return &Manager{
		networkID:  networkID,
		p2pManager: p2pManager,
		host:       host,
		peerDB:     peerDB,
		log:        log,
	}
}

func (m *Manager) Start(ctx context.Context) {
	m.startOnce.Do(func() {
		m.ctx, m.stopFunc = context.WithCancel(ctx)
		kademliaDHT, err := dht.New(m.ctx, m.host, dht.Mode(dht.ModeServer))
		if err != nil {
			log.Fatal(err)
		}

		// Bootstrap the DHT. In the default configuration, this spawns a Background worker that will keep the
		// node connected to the bootstrap peers and will disconnect from peers that are not useful.
		if err = kademliaDHT.Bootstrap(m.ctx); err != nil {
			log.Fatal(err)
		}

		for _, seedPeer := range m.peerDB.SeedPeers() {
			addrInfo, err := p2ppeer.AddrInfoFromString(fmt.Sprintf("/ip4/%s/udp/%d/p2p/%s", seedPeer.IP(), seedPeer.Address().Port, seedPeer.Identity.PublicKey()))
			if err != nil {
				m.log.Warnln("Failed to parse bootstrap node address from PeerDB:", err)
				continue
			}
			if err := m.host.Connect(ctx, *addrInfo); err != nil {
				m.log.Infoln("Failed to connect to bootstrap node:", seedPeer, err)
				continue
			}

			if _, err := kademliaDHT.RoutingTable().TryAddPeer(addrInfo.ID, true, true); err != nil {
				m.log.Warnln("Failed to add bootstrap node to routing table:", err)
				continue
			}

			m.log.Debugln("Connected to bootstrap node:", seedPeer)
		}

		m.routingDiscovery = routing.NewRoutingDiscovery(kademliaDHT)
		util.Advertise(m.ctx, m.routingDiscovery, m.networkID, discovery.TTL(5*time.Minute))

		go m.discoveryLoop()

		m.isStarted.Store(true)
	})
}

// Stop terminates internal background workers. Calling multiple times has no effect.
func (m *Manager) Stop() (err error) {
	if !m.isStarted.Load() {
		return ierrors.New("can't stop the manager: it hasn't been started yet")
	}
	m.stopOnce.Do(func() {
		m.stopFunc()
	})

	return err
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

		peer, err := network.NewPeerFromAddrInfo(&peerAddrInfo)
		if err != nil {
			m.log.Warnf("Failed to create peer from address %s: %w", peerAddrInfo.Addrs, err)
			continue
		}

		if err := m.p2pManager.DialPeer(m.ctx, peer); err != nil {
			m.log.Warnf("Failed to dial peer %s: %w", peer, err)
		}
	}
}
