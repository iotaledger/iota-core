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

	// Neighbor returns the neighbor with the given ID.
	Neighbor(peerID peer.ID) (Neighbor, error)
	// NeighborExists checks if a neighbor with the given ID exists.
	NeighborExists(peerID peer.ID) bool
	// ManualNeighborExists checks if a neighbor with the given ID exists in the manual peering layer.
	ManualNeighborExists(peerID peer.ID) bool
	// RemoveNeighbor disconnects the neighbor with the given ID
	// and removes it from manual peering in case it was added manually.
	RemoveNeighbor(peerID peer.ID) error
	// DropNeighbor disconnects the neighbor with the given ID.
	DropNeighbor(peerID peer.ID) error

	AllNeighbors() []Neighbor
	AutopeeringNeighbors() []Neighbor
	AddManualPeers(multiAddresses ...multiaddr.Multiaddr) error

	P2PHost() host.Host

	Start(ctx context.Context, networkID string) error
	Shutdown()
}
