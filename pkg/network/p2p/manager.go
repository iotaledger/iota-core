package p2p

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	p2pnetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	"google.golang.org/protobuf/proto"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/network"
)

const (
	protocolID               = "iota-core/1.0.0"
	defaultConnectionTimeout = 5 * time.Second // timeout after which the connection must be established.
	ioTimeout                = 4 * time.Second
)

var (
	// ErrTimeout is returned when an expected incoming connection was not received in time.
	ErrTimeout = ierrors.New("accept timeout")
	// ErrDuplicateAccept is returned when the server already registered an accept request for that peer ID.
	ErrDuplicateAccept = ierrors.New("accept request for that peer already exists")
	// ErrNoP2P means that the given peer does not support the p2p service.
	ErrNoP2P = ierrors.New("peer does not have a p2p service")
)

// ConnectPeerOption defines an option for the DialPeer and AcceptPeer methods.
type ConnectPeerOption func(conf *connectPeerConfig)

type connectPeerConfig struct {
	useDefaultTimeout bool
}

// ProtocolHandler holds callbacks to handle a protocol.
type ProtocolHandler struct {
	PacketFactory func() proto.Message
	PacketHandler func(peer.ID, proto.Message) error
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

	libp2pHost host.Host
	peerDB     *network.DB

	log *logger.Logger

	shutdownMutex syncutils.RWMutex
	isShutdown    bool

	neighbors      map[peer.ID]*Neighbor
	neighborsMutex syncutils.RWMutex

	protocolHandler      *ProtocolHandler
	protocolHandlerMutex syncutils.RWMutex
}

// NewManager creates a new Manager.
func NewManager(libp2pHost host.Host, peerDB *network.DB, log *logger.Logger) *Manager {
	m := &Manager{
		libp2pHost: libp2pHost,
		peerDB:     peerDB,
		log:        log,
		Events:     NewNeighborEvents(),
		neighbors:  make(map[peer.ID]*Neighbor),
	}

	return m
}

// RegisterProtocol registers the handler for the protocol within the manager.
func (m *Manager) RegisterProtocol(factory func() proto.Message, handler func(peer.ID, proto.Message) error) {
	m.protocolHandlerMutex.Lock()
	defer m.protocolHandlerMutex.Unlock()

	m.protocolHandler = &ProtocolHandler{
		PacketFactory: factory,
		PacketHandler: handler,
	}

	m.libp2pHost.SetStreamHandler(protocol.ID(protocolID), m.handleStream)
}

// UnregisterProtocol unregisters the handler for the protocol.
func (m *Manager) UnregisterProtocol() {
	m.protocolHandlerMutex.Lock()
	defer m.protocolHandlerMutex.Unlock()

	m.libp2pHost.RemoveStreamHandler(protocol.ID(protocolID))
	m.protocolHandler = nil
}

// DialPeer connects to a peer.
func (m *Manager) DialPeer(ctx context.Context, peer *network.Peer, opts ...ConnectPeerOption) error {
	m.protocolHandlerMutex.RLock()
	defer m.protocolHandlerMutex.RUnlock()

	if m.protocolHandler == nil {
		return ierrors.New("no protocol handler registered to dial peer")
	}

	if m.neighborExists(peer.ID) {
		return ierrors.Wrapf(ErrDuplicateNeighbor, "peer %s already exists", peer.ID)
	}

	conf := buildConnectPeerConfig(opts)

	// Adds the peer's multiaddresses to the peerstore, so that they can be used for dialing.
	m.libp2pHost.Peerstore().AddAddrs(peer.ID, peer.PeerAddresses, peerstore.ConnectedAddrTTL)
	cancelCtx := ctx
	if conf.useDefaultTimeout {
		var cancel context.CancelFunc
		cancelCtx, cancel = context.WithTimeout(ctx, defaultConnectionTimeout)
		defer cancel()
	}

	stream, err := m.P2PHost().NewStream(cancelCtx, peer.ID, protocolID)
	if err != nil {
		return ierrors.Wrapf(err, "dial %s / %s failed to open stream for proto %s", peer.PeerAddresses, peer.ID, protocolID)
	}

	ps := NewPacketsStream(stream, m.protocolHandler.PacketFactory)
	if err := ps.sendNegotiation(); err != nil {
		m.closeStream(stream)

		return ierrors.Wrapf(err, "dial %s / %s failed to send negotiation for proto %s", peer.PeerAddresses, peer.ID, protocolID)
	}

	m.log.Debugw("outgoing stream negotiated",
		"id", peer.ID,
		"addr", ps.Conn().RemoteMultiaddr(),
		"proto", protocolID,
	)

	if err := m.peerDB.UpdatePeer(peer); err != nil {
		m.closeStream(stream)

		return ierrors.Wrapf(err, "failed to update peer %s", peer.ID)
	}

	if err := m.addNeighbor(peer, ps); err != nil {
		m.closeStream(stream)

		return ierrors.Errorf("failed to add neighbor %s: %s", peer.ID, err)
	}

	return nil
}

// Shutdown stops the manager and closes all established connections.
func (m *Manager) Shutdown() {
	m.shutdownMutex.Lock()
	defer m.shutdownMutex.Unlock()

	if m.isShutdown {
		return
	}
	m.isShutdown = true
	m.dropAllNeighbors()

	m.UnregisterProtocol()
}

// LocalPeerID returns the local peer ID.
func (m *Manager) LocalPeerID() peer.ID {
	return m.libp2pHost.ID()
}

// P2PHost returns the lib-p2p host.
func (m *Manager) P2PHost() host.Host {
	return m.libp2pHost
}

// DropNeighbor disconnects the neighbor with the given ID and the group.
func (m *Manager) DropNeighbor(id peer.ID) error {
	nbr, err := m.neighbor(id)
	if err != nil {
		return ierrors.WithStack(err)
	}
	nbr.Close()

	return nil
}

// Send sends a message with the specific protocol to a set of neighbors.
func (m *Manager) Send(packet proto.Message, to ...peer.ID) {
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
func (m *Manager) AllNeighborsIDs() (ids []peer.ID) {
	ids = make([]peer.ID, 0)
	neighbors := m.AllNeighbors()
	for _, nbr := range neighbors {
		ids = append(ids, nbr.ID)
	}

	return
}

// NeighborsByID returns all the neighbors that are currently connected corresponding to the supplied ids.
func (m *Manager) NeighborsByID(ids []peer.ID) []*Neighbor {
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

func (m *Manager) handleStream(stream p2pnetwork.Stream) {
	m.protocolHandlerMutex.RLock()
	defer m.protocolHandlerMutex.RUnlock()

	if m.protocolHandler == nil {
		m.log.Error("no protocol handler registered")
		stream.Close()

		return
	}

	ps := NewPacketsStream(stream, m.protocolHandler.PacketFactory)
	if err := ps.receiveNegotiation(); err != nil {
		m.log.Errorw("failed to receive negotiation message")
		m.closeStream(stream)

		return
	}

	peerAddrInfo := &peer.AddrInfo{
		ID:    stream.Conn().RemotePeer(),
		Addrs: []multiaddr.Multiaddr{stream.Conn().RemoteMultiaddr()},
	}
	peer := network.NewPeerFromAddrInfo(peerAddrInfo)
	if err := m.peerDB.UpdatePeer(peer); err != nil {
		m.log.Errorf("failed to update peer %s in peer database: %s", peer.ID, err)
		m.closeStream(stream)

		return
	}

	if err := m.addNeighbor(peer, ps); err != nil {
		m.log.Errorf("failed to add neighbor %s: %s", peer.ID, err)
		m.closeStream(stream)

		return
	}
}

func (m *Manager) closeStream(s p2pnetwork.Stream) {
	if err := s.Close(); err != nil {
		m.log.Warnw("close error", "err", err)
	}
}

// neighborWithGroup returns neighbor by ID and group.
func (m *Manager) neighbor(id peer.ID) (*Neighbor, error) {
	m.neighborsMutex.RLock()
	defer m.neighborsMutex.RUnlock()

	nbr, ok := m.neighbors[id]
	if !ok {
		return nil, ErrUnknownNeighbor
	}

	return nbr, nil
}

func (m *Manager) addNeighbor(peer *network.Peer, ps *PacketsStream) error {
	if peer.ID == m.libp2pHost.ID() {
		return ierrors.WithStack(ErrLoopbackNeighbor)
	}
	m.shutdownMutex.RLock()
	defer m.shutdownMutex.RUnlock()
	if m.isShutdown {
		return ErrNotRunning
	}
	if m.neighborExists(peer.ID) {
		return ierrors.WithStack(ErrDuplicateNeighbor)
	}

	// create and add the neighbor
	nbr := NewNeighbor(peer, ps, m.log, func(nbr *Neighbor, packet proto.Message) {
		m.protocolHandlerMutex.RLock()
		defer m.protocolHandlerMutex.RUnlock()

		if m.protocolHandler == nil {
			nbr.Log.Errorw("Can't handle packet as no protocol is registered")
			return
		}
		if err := m.protocolHandler.PacketHandler(nbr.ID, packet); err != nil {
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

func (m *Manager) neighborExists(id peer.ID) bool {
	m.neighborsMutex.RLock()
	defer m.neighborsMutex.RUnlock()
	_, exists := m.neighbors[id]

	return exists
}

func (m *Manager) deleteNeighbor(nbr *Neighbor) {
	m.neighborsMutex.Lock()
	defer m.neighborsMutex.Unlock()
	delete(m.neighbors, nbr.ID)
}

func (m *Manager) setNeighbor(nbr *Neighbor) error {
	m.neighborsMutex.Lock()
	defer m.neighborsMutex.Unlock()
	if _, exists := m.neighbors[nbr.ID]; exists {
		return ierrors.WithStack(ErrDuplicateNeighbor)
	}
	m.neighbors[nbr.ID] = nbr

	return nil
}

func (m *Manager) dropAllNeighbors() {
	neighborsList := m.AllNeighbors()
	for _, nbr := range neighborsList {
		nbr.Close()
	}
}
