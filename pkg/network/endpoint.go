package network

import (
	p2ppeer "github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
)

type Endpoint interface {
	LocalPeerID() p2ppeer.ID
	Send(packet proto.Message, to ...p2ppeer.ID)
	Shutdown()
}
