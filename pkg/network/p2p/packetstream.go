package p2p

import (
	p2pnetwork "github.com/libp2p/go-libp2p/core/network"
	"go.uber.org/atomic"
	"google.golang.org/protobuf/proto"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/libp2putil"
	pp "github.com/iotaledger/iota-core/pkg/network/p2p/proto"
)

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
	if err := ps.writer.WriteBlk(message); err != nil {
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
