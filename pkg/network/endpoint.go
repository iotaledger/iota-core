package network

import (
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
)

type Endpoint interface {
	LocalPeerID() peer.ID
	RegisterProtocol(factory func() proto.Message, handler func(peer.ID, proto.Message) error)
	UnregisterProtocol()
	Send(packet proto.Message, to ...peer.ID)
	Shutdown()
}
