package network

import (
	"google.golang.org/protobuf/proto"

	"github.com/iotaledger/hive.go/crypto/identity"
)

type PeerID = identity.ID

type Endpoint interface {
	LocalPeerID() PeerID

	RegisterProtocol(protocolID string, newMessage func() proto.Message, handler func(PeerID, proto.Message) error)

	UnregisterProtocol(protocolID string)

	Send(packet proto.Message, protocolID string, to ...PeerID)
}
