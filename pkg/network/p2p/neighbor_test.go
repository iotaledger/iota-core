package p2p

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	p2pnetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/iota-core/pkg/network"
	p2pproto "github.com/iotaledger/iota-core/pkg/network/p2p/proto"
)

var (
	testPacket1 = &p2pproto.Negotiation{}
	log         = logger.NewExampleLogger("p2p_test")
)

func TestNeighborClose(t *testing.T) {
	a, _, teardown := newStreamsPipe(t)
	defer teardown()

	n := newTestNeighbor("A", a)
	n.readLoop()
	require.NoError(t, n.disconnect())
}

func TestNeighborCloseTwice(t *testing.T) {
	a, _, teardown := newStreamsPipe(t)
	defer teardown()

	n := newTestNeighbor("A", a)
	n.readLoop()
	require.NoError(t, n.disconnect())
	require.NoError(t, n.disconnect())
}

func TestNeighborWrite(t *testing.T) {
	a, b, teardown := newStreamsPipe(t)
	defer teardown()

	var countA uint32
	neighborA := newTestNeighbor("A", a, func(neighbor *Neighbor, packet proto.Message) {
		_ = packet.(*p2pproto.Negotiation)
		atomic.AddUint32(&countA, 1)
	})
	defer neighborA.disconnect()
	neighborA.readLoop()

	var countB uint32
	neighborB := newTestNeighbor("B", b, func(neighbor *Neighbor, packet proto.Message) {
		_ = packet.(*p2pproto.Negotiation)
		atomic.AddUint32(&countB, 1)
	})
	defer neighborB.disconnect()
	neighborB.readLoop()

	err := neighborA.stream.WritePacket(testPacket1)
	require.NoError(t, err)
	err = neighborB.stream.WritePacket(testPacket1)
	require.NoError(t, err)

	assert.Eventually(t, func() bool { return atomic.LoadUint32(&countA) == 1 }, time.Second, 10*time.Millisecond)
	assert.Eventually(t, func() bool { return atomic.LoadUint32(&countB) == 1 }, time.Second, 10*time.Millisecond)
}

func newTestNeighbor(name string, stream p2pnetwork.Stream, packetReceivedFunc ...PacketReceivedFunc) *Neighbor {
	var packetReceived PacketReceivedFunc
	if len(packetReceivedFunc) > 0 {
		packetReceived = packetReceivedFunc[0]
	} else {
		packetReceived = func(neighbor *Neighbor, packet proto.Message) {}
	}

	return NewNeighbor(newTestPeer(name), NewPacketsStream(stream, packetFactory), log.Named(name), packetReceived, func(neighbor *Neighbor) {})
}

func packetFactory() proto.Message {
	return &p2pproto.Negotiation{}
}

func newTestPeer(_ string) *network.Peer {
	_, pub, err := p2pcrypto.GenerateEd25519Key(nil)
	if err != nil {
		panic(err)
	}

	p2pid, err := peer.IDFromPublicKey(pub)
	if err != nil {
		panic(err)
	}

	addrInfo, _ := peer.AddrInfoFromString("/ip4/0.0.0.0/udp/14666/p2p/" + p2pid.String())
	return network.NewPeerFromAddrInfo(addrInfo)
}

// newStreamsPipe returns a pair of libp2p Stream that are talking to each other.
func newStreamsPipe(t testing.TB) (p2pnetwork.Stream, p2pnetwork.Stream, func()) {
	ctx := context.Background()
	host1, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	host2, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	acceptStremCh := make(chan p2pnetwork.Stream, 1)
	host2.Peerstore().AddAddrs(host1.ID(), host1.Addrs(), peerstore.PermanentAddrTTL)
	host2.SetStreamHandler(protocol.TestingID, func(s p2pnetwork.Stream) {
		acceptStremCh <- s
	})
	host1.Peerstore().AddAddrs(host2.ID(), host2.Addrs(), peerstore.PermanentAddrTTL)
	dialStream, err := host1.NewStream(ctx, host2.ID(), protocol.TestingID)
	require.NoError(t, err)
	_, err = dialStream.Write(nil)
	require.NoError(t, err)
	acceptStream := <-acceptStremCh
	tearDown := func() {
		err2 := dialStream.Close()
		require.NoError(t, err2)
		err2 = acceptStream.Close()
		require.NoError(t, err2)
		err2 = host1.Close()
		require.NoError(t, err2)
		err2 = host2.Close()
		require.NoError(t, err2)
	}

	return dialStream, acceptStream, tearDown
}
