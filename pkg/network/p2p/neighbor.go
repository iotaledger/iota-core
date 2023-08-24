package p2p

import (
	"context"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/protocol"
	"google.golang.org/protobuf/proto"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/iota-core/pkg/network"
)

const (
	NeighborsSendQueueSize = 20_000
)

type queuedPacket struct {
	protocolID protocol.ID
	packet     proto.Message
}

type (
	PacketReceivedFunc       func(neighbor *Neighbor, packet proto.Message)
	NeighborDisconnectedFunc func(neighbor *Neighbor)
)

// Neighbor describes the established p2p connection to another peer.
type Neighbor struct {
	*network.Peer

	packetReceivedFunc PacketReceivedFunc
	disconnectedFunc   NeighborDisconnectedFunc

	Log            *logger.Logger
	disconnectOnce sync.Once
	wg             sync.WaitGroup

	loopCtx       context.Context
	loopCtxCancel context.CancelFunc

	stream *PacketsStream

	sendQueue chan *queuedPacket
}

// NewNeighbor creates a new neighbor from the provided peer and connection.
func NewNeighbor(p *network.Peer, stream *PacketsStream, log *logger.Logger, packetReceivedCallback PacketReceivedFunc, disconnectedCallback NeighborDisconnectedFunc) *Neighbor {
	ctx, cancel := context.WithCancel(context.Background())

	neighbor := &Neighbor{
		Peer: p,

		packetReceivedFunc: packetReceivedCallback,
		disconnectedFunc:   disconnectedCallback,

		loopCtx:       ctx,
		loopCtxCancel: cancel,

		stream:    stream,
		sendQueue: make(chan *queuedPacket, NeighborsSendQueueSize),
	}

	conn := neighbor.stream.Conn()

	neighbor.Log = log.With(
		"id", p.ID,
		"localAddr", conn.LocalMultiaddr(),
		"remoteAddr", conn.RemoteMultiaddr(),
	)

	return neighbor
}

func (n *Neighbor) Enqueue(packet proto.Message, protocolID protocol.ID) {
	select {
	case n.sendQueue <- &queuedPacket{protocolID: protocolID, packet: packet}:
	default:
		n.Log.Warn("Dropped packet due to SendQueue being full")
	}
}

// PacketsRead returns number of packets this neighbor has received.
func (n *Neighbor) PacketsRead() uint64 {
	return n.stream.packetsRead.Load()
}

// PacketsWritten returns number of packets this neighbor has sent.
func (n *Neighbor) PacketsWritten() uint64 {
	return n.stream.packetsWritten.Load()
}

// ConnectionEstablished returns the connection established.
func (n *Neighbor) ConnectionEstablished() time.Time {
	return n.stream.Stat().Opened
}

func (n *Neighbor) readLoop() {
	n.wg.Add(1)
	go func(stream *PacketsStream) {
		defer n.wg.Done()
		for {
			if n.loopCtx.Err() != nil {
				n.Log.Infof("Exit %s readLoop due to canceled context")
				return
			}

			// This loop gets terminated when we encounter an error on .read() function call.
			// The error might be caused by another goroutine closing the connection by calling .disconnect() function.
			// Or by a problem with the connection itself.
			// In any case we call .disconnect() after encountering the error,
			// the disconnect call is protected with sync.Once, so in case another goroutine called it before us,
			// we won't execute it twice.
			packet := stream.packetFactory()
			err := stream.ReadPacket(packet)
			if err != nil {
				n.Log.Infow("Stream read packet error", "err", err)
				if disconnectErr := n.disconnect(); disconnectErr != nil {
					n.Log.Warnw("Failed to disconnect", "err", disconnectErr)
				}

				return
			}
			n.packetReceivedFunc(n, packet)
		}
	}(n.stream)
}

func (n *Neighbor) writeLoop() {
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		for {
			select {
			case <-n.loopCtx.Done():
				n.Log.Info("Exit writeLoop due to canceled context")
				return
			case sendPacket := <-n.sendQueue:
				if n.stream == nil {
					n.Log.Warnw("send error, no stream for protocol", "peer-id", n.ID, "protocol", sendPacket.protocolID)
					if disconnectErr := n.disconnect(); disconnectErr != nil {
						n.Log.Warnw("Failed to disconnect", "err", disconnectErr)
					}

					return
				}
				if err := n.stream.WritePacket(sendPacket.packet); err != nil {
					n.Log.Warnw("send error", "peer-id", n.ID, "err", err)
					if disconnectErr := n.disconnect(); disconnectErr != nil {
						n.Log.Warnw("Failed to disconnect", "err", disconnectErr)
					}

					return
				}
			}
		}
	}()
}

// Close closes the connection with the neighbor.
func (n *Neighbor) Close() {
	if err := n.disconnect(); err != nil {
		n.Log.Errorw("Failed to disconnect the neighbor", "err", err)
	}
	n.wg.Wait()
}

func (n *Neighbor) disconnect() (err error) {
	n.disconnectOnce.Do(func() {
		// Stop the loops
		n.loopCtxCancel()

		// Close all streams
		if streamErr := n.stream.Close(); streamErr != nil {
			err = ierrors.WithStack(streamErr)
		}
		n.Log.Infow("Stream closed", "protocol", n.stream.Protocol())
		n.Log.Info("Connection closed")
		n.disconnectedFunc(n)
	})

	return err
}
