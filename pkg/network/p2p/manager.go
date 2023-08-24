package p2p

import (
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/protocol"
	"google.golang.org/protobuf/proto"

	"github.com/iotaledger/hive.go/autopeering/peer"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/network"
)

const (
	protocolID = "iota-core/0.0.1"
)

// ConnectPeerOption defines an option for the DialPeer and AcceptPeer methods.
type ConnectPeerOption func(conf *connectPeerConfig)

type connectPeerConfig struct {
	useDefaultTimeout bool
}

// ProtocolHandler holds callbacks to handle a protocol.
type ProtocolHandler struct {
	PacketFactory func() proto.Message
	PacketHandler func(network.PeerID, proto.Message) error
}

func buildConnectPeerConfig(opts []ConnectPeerOption) *connectPeerConfig {
	conf := &connectPeerConfig{
		useDefaultTimeout: true,
	}
	for _, o := range opts {
		o(conf)
	}

	return conf
}

// WithNoDefaultTimeout returns a ConnectPeerOption that disables the default timeout for dial or accept.
func WithNoDefaultTimeout() ConnectPeerOption {
	return func(conf *connectPeerConfig) {
		conf.useDefaultTimeout = false
	}
}

// The Manager handles the connected neighbors.
type Manager struct {
	Events *NeighborEvents

	local      *peer.Local
	libp2pHost host.Host

	log *logger.Logger

	stopMutex syncutils.RWMutex
	isStopped bool

	neighbors      map[network.PeerID]*Neighbor
	neighborsMutex syncutils.RWMutex

	protocolHandler *ProtocolHandler
}

// NewManager creates a new Manager.
func NewManager(libp2pHost host.Host, local *peer.Local, log *logger.Logger) *Manager {
	m := &Manager{
		libp2pHost: libp2pHost,
		local:      local,
		log:        log,
		Events:     NewNeighborEvents(),
		neighbors:  map[network.PeerID]*Neighbor{},
	}

	m.libp2pHost.SetStreamHandler(protocol.ID(protocolID), m.handleStream)

	return m
}

// Stop stops the manager and closes all established connections.
func (m *Manager) Stop() {
	m.stopMutex.Lock()
	defer m.stopMutex.Unlock()

	if m.isStopped {
		return
	}
	m.isStopped = true
	m.dropAllNeighbors()
}

// LocalPeerID returns the local peer ID.
func (m *Manager) LocalPeerID() network.PeerID {
	return m.local.Identity.ID()
}

// Shutdown unregisters the core protocol.
func (m *Manager) Shutdown() {
	m.libp2pHost.RemoveStreamHandler(protocol.ID(protocolID))
	m.protocolHandler = nil
}

// P2PHost returns the lib-p2p host.
func (m *Manager) P2PHost() host.Host {
	return m.libp2pHost
}

// LocalPeer return the local peer.
func (m *Manager) LocalPeer() *peer.Local {
	return m.local
}

// Neighbor returns the neighbor by its id.
func (m *Manager) Neighbor(id network.PeerID) (*Neighbor, error) {
	m.neighborsMutex.RLock()
	defer m.neighborsMutex.RUnlock()
	nbr, ok := m.neighbors[id]
	if !ok {
		return nil, ErrUnknownNeighbor
	}

	return nbr, nil
}

// DropNeighbor disconnects the neighbor with the given ID and the group.
func (m *Manager) DropNeighbor(id network.PeerID) error {
	nbr, err := m.neighbor(id)
	if err != nil {
		return ierrors.WithStack(err)
	}
	nbr.Close()

	return nil
}

// Send sends a message with the specific protocol to a set of neighbors.
func (m *Manager) Send(packet proto.Message, to ...network.PeerID) {
	var neighbors []*Neighbor
	if len(to) == 0 {
		neighbors = m.AllNeighbors()
	} else {
		neighbors = m.NeighborsByID(to)
	}

	for _, nbr := range neighbors {
		nbr.Enqueue(packet, protocol.ID(protocolID))
	}
}

// AllNeighbors returns all the neighbors that are currently connected.
func (m *Manager) AllNeighbors() []*Neighbor {
	m.neighborsMutex.RLock()
	defer m.neighborsMutex.RUnlock()
	result := make([]*Neighbor, 0, len(m.neighbors))
	for _, n := range m.neighbors {
		result = append(result, n)
	}

	return result
}

// AllNeighborsIDs returns all the ids of the neighbors that are currently connected.
func (m *Manager) AllNeighborsIDs() (ids []network.PeerID) {
	ids = make([]network.PeerID, 0)
	neighbors := m.AllNeighbors()
	for _, nbr := range neighbors {
		ids = append(ids, nbr.Identity.ID())
	}

	return
}

// NeighborsByID returns all the neighbors that are currently connected corresponding to the supplied ids.
func (m *Manager) NeighborsByID(ids []network.PeerID) []*Neighbor {
	result := make([]*Neighbor, 0, len(ids))
	if len(ids) == 0 {
		return result
	}

	m.neighborsMutex.RLock()
	defer m.neighborsMutex.RUnlock()
	for _, id := range ids {
		if n, ok := m.neighbors[id]; ok {
			result = append(result, n)
		}
	}

	return result
}

// neighborWithGroup returns neighbor by ID and group.
func (m *Manager) neighbor(id network.PeerID) (*Neighbor, error) {
	m.neighborsMutex.RLock()
	defer m.neighborsMutex.RUnlock()

	nbr, ok := m.neighbors[id]
	if !ok {
		return nil, ErrUnknownNeighbor
	}

	return nbr, nil
}

func (m *Manager) addNeighbor(peer *network.Peer, ps *PacketsStream) error {
	if peer.Identity.ID() == m.local.Identity.ID() {
		return ierrors.WithStack(ErrLoopbackNeighbor)
	}
	m.stopMutex.RLock()
	defer m.stopMutex.RUnlock()
	if m.isStopped {
		return ErrNotRunning
	}
	if m.neighborExists(peer.Identity.ID()) {
		return ierrors.WithStack(ErrDuplicateNeighbor)
	}

	// create and add the neighbor
	nbr := NewNeighbor(peer, ps, m.log, func(nbr *Neighbor, packet proto.Message) {
		if m.protocolHandler == nil {
			nbr.Log.Errorw("Can't handle packet as no protocol is registered")
		}
		if err := m.protocolHandler.PacketHandler(nbr.Identity.ID(), packet); err != nil {
			nbr.Log.Debugw("Can't handle packet", "err", err)
		}
	}, func(nbr *Neighbor) {
		m.deleteNeighbor(nbr)
		m.Events.NeighborRemoved.Trigger(nbr)
	})
	if err := m.setNeighbor(nbr); err != nil {
		if resetErr := ps.Close(); resetErr != nil {
			nbr.Log.Errorw("error closing stream", "err", resetErr)
		}

		return ierrors.WithStack(err)
	}
	nbr.readLoop()
	nbr.writeLoop()
	nbr.Log.Info("Connection established")
	m.Events.NeighborAdded.Trigger(nbr)

	return nil
}

func (m *Manager) neighborExists(id network.PeerID) bool {
	m.neighborsMutex.RLock()
	defer m.neighborsMutex.RUnlock()
	_, exists := m.neighbors[id]

	return exists
}

func (m *Manager) deleteNeighbor(nbr *Neighbor) {
	m.neighborsMutex.Lock()
	defer m.neighborsMutex.Unlock()
	delete(m.neighbors, nbr.Identity.ID())
}

func (m *Manager) setNeighbor(nbr *Neighbor) error {
	m.neighborsMutex.Lock()
	defer m.neighborsMutex.Unlock()
	if _, exists := m.neighbors[nbr.Identity.ID()]; exists {
		return ierrors.WithStack(ErrDuplicateNeighbor)
	}
	m.neighbors[nbr.Identity.ID()] = nbr

	return nil
}

func (m *Manager) dropAllNeighbors() {
	neighborsList := m.AllNeighbors()
	for _, nbr := range neighborsList {
		nbr.Close()
	}
}
