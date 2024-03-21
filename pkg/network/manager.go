package network

import (
	"context"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/iotaledger/hive.go/runtime/event"
)

type Manager interface {
	Endpoint

	DialPeer(ctx context.Context, peer *Peer) error

	OnNeighborAdded(handler func(Neighbor)) *event.Hook[func(Neighbor)]
	OnNeighborRemoved(handler func(Neighbor)) *event.Hook[func(Neighbor)]

	AllNeighbors() []Neighbor
	AutopeeringNeighbors() []Neighbor

	DropNeighbor(peerID peer.ID) error
	NeighborExists(peerID peer.ID) bool

	P2PHost() host.Host

	Start(ctx context.Context, networkID string) error
	Shutdown()

	AddManualPeers(multiAddresses ...multiaddr.Multiaddr) error
}
