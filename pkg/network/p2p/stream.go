package p2p

import (
	"context"
	"time"

	p2pnetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"go.uber.org/atomic"
	"google.golang.org/protobuf/proto"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/libp2putil"
	"github.com/iotaledger/iota-core/pkg/network"
	pp "github.com/iotaledger/iota-core/pkg/network/p2p/proto"
)

const (
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

func (m *Manager) DialPeer(ctx context.Context, peer *network.Peer, opts ...ConnectPeerOption) error {
	if m.neighborExists(peer.Identity.ID()) {
		return ierrors.Wrapf(ErrDuplicateNeighbor, "peer %s already exists", peer.Identity.ID())
	}

	conf := buildConnectPeerConfig(opts)
	libp2pID, err := libp2putil.ToLibp2pPeerID(peer)
	if err != nil {
		return ierrors.WithStack(err)
	}

	// Adds the peer's multiaddresses to the peerstore, so that they can be used for dialing.
	m.libp2pHost.Peerstore().AddAddrs(libp2pID, peer.PeerAddresses, peerstore.ConnectedAddrTTL)

	if conf.useDefaultTimeout {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultConnectionTimeout)
		defer cancel()
	}

	stream, err := m.P2PHost().NewStream(ctx, libp2pID, protocolID)
	if err != nil {
		return ierrors.Wrapf(err, "dial %s / %s failed to open stream for proto %s", peer.PeerAddresses, peer.Identity.ID(), protocolID)
	}

	ps := NewPacketsStream(stream, m.protocolHandler.PacketFactory)
	if err := ps.sendNegotiation(); err != nil {
		m.closeStream(stream)

		return ierrors.Wrapf(err, "dial %s / %s failed to send negotiation for proto %s", peer.PeerAddresses, peer.Identity.ID(), protocolID)
	}

	m.log.Debugw("outgoing stream negotiated",
		"id", peer.Identity.ID(),
		"addr", ps.Conn().RemoteMultiaddr(),
		"proto", protocolID,
	)

	if err := m.addNeighbor(peer, ps); err != nil {
		m.closeStream(stream)

		return ierrors.Errorf("failed to add neighbor %s: %s", peer.Identity.ID(), err)
	}

	return nil
}

func (m *Manager) handleStream(stream p2pnetwork.Stream) {
	ps := NewPacketsStream(stream, m.protocolHandler.PacketFactory)
	if err := ps.receiveNegotiation(); err != nil {
		m.log.Errorw("failed to receive negotiation message")
		m.closeStream(stream)

		return
	}

	peerMultiAddr := stream.Conn().RemoteMultiaddr()
	peer, err := network.NewPeer(peerMultiAddr)
	if err != nil {
		m.log.Errorf("failed to create peer from multiaddr %s: %s", peerMultiAddr, err)
		m.closeStream(stream)

		return
	}

	if err := m.addNeighbor(peer, ps); err != nil {
		m.log.Errorf("failed to add neighbor %s: %s", peer.Identity.ID(), err)
		m.closeStream(stream)

		return
	}
}

func (m *Manager) closeStream(s p2pnetwork.Stream) {
	if err := s.Close(); err != nil {
		m.log.Warnw("close error", "err", err)
	}
}

// PacketsStream represents a stream of packets.
type PacketsStream struct {
	p2pnetwork.Stream
	packetFactory func() proto.Message

	readerLock     syncutils.Mutex
	reader         *libp2putil.UvarintReader
	writerLock     syncutils.Mutex
	writer         *libp2putil.UvarintWriter
	packetsRead    *atomic.Uint64
	packetsWritten *atomic.Uint64
}

// NewPacketsStream creates a new PacketsStream.
func NewPacketsStream(stream p2pnetwork.Stream, packetFactory func() proto.Message) *PacketsStream {
	return &PacketsStream{
		Stream:         stream,
		packetFactory:  packetFactory,
		reader:         libp2putil.NewDelimitedReader(stream),
		writer:         libp2putil.NewDelimitedWriter(stream),
		packetsRead:    atomic.NewUint64(0),
		packetsWritten: atomic.NewUint64(0),
	}
}

// WritePacket writes a packet to the stream.
func (ps *PacketsStream) WritePacket(message proto.Message) error {
	ps.writerLock.Lock()
	defer ps.writerLock.Unlock()
	err := ps.writer.WriteBlk(message)
	if err != nil {
		return ierrors.WithStack(err)
	}
	ps.packetsWritten.Inc()

	return nil
}

// ReadPacket reads a packet from the stream.
func (ps *PacketsStream) ReadPacket(message proto.Message) error {
	ps.readerLock.Lock()
	defer ps.readerLock.Unlock()
	if err := ps.reader.ReadBlk(message); err != nil {
		return ierrors.WithStack(err)
	}
	ps.packetsRead.Inc()

	return nil
}

func (ps *PacketsStream) sendNegotiation() error {
	return ierrors.WithStack(ps.WritePacket(&pp.Negotiation{}))
}

func (ps *PacketsStream) receiveNegotiation() (err error) {
	return ierrors.WithStack(ps.ReadPacket(&pp.Negotiation{}))
}
