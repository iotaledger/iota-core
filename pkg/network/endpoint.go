package network

import (
	"google.golang.org/protobuf/proto"

	"github.com/iotaledger/hive.go/crypto/identity"
)

type PeerID = identity.ID

type Endpoint interface {
	LocalPeerID() PeerID
	Send(packet proto.Message, to ...PeerID)
	Shutdown()
}
