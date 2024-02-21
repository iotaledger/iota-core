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

	DialPeer(context.Context, *Peer) error

	OnNeighborAdded(func(Neighbor)) *event.Hook[func(Neighbor)]
	OnNeighborRemoved(func(Neighbor)) *event.Hook[func(Neighbor)]

	AllNeighbors() []Neighbor
	AutopeeringNeighbors() []Neighbor

	DropNeighbor(peer.ID) error
	NeighborExists(peer.ID) bool

	P2PHost() host.Host

	Start(ctx context.Context, networkID string) error
	Shutdown()

	AddManualPeers(...multiaddr.Multiaddr) error
}
