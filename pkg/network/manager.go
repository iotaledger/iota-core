package network

import (
	"context"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/iotaledger/hive.go/runtime/event"
)

// Manager is the network manager interface.
// Peer is a known node in the network.
// Neighbor is a Peer with an established connection in the gossip layer.
type Manager interface {
	Endpoint

	// DialPeer connects to a peer.
	DialPeer(ctx context.Context, peer *Peer) error
	// RemovePeer disconnects the peer with the given ID
	// and removes it from manual peering in case it was added manually.
	RemovePeer(peerID peer.ID) error
	// AddManualPeer adds a manual peer to the list of known peers.
	AddManualPeer(multiAddress multiaddr.Multiaddr) (*Peer, error)
	// ManualPeer returns the manual peer with the given ID.
	ManualPeer(peerID peer.ID) (*Peer, error)
	// ManualPeers returns all the manual peers.
	ManualPeers(onlyConnected ...bool) []*Peer

	// OnNeighborAdded registers a callback that gets triggered when a neighbor is added.
	OnNeighborAdded(handler func(Neighbor)) *event.Hook[func(Neighbor)]
	// OnNeighborRemoved registers a callback that gets triggered when a neighbor is removed.
	OnNeighborRemoved(handler func(Neighbor)) *event.Hook[func(Neighbor)]

	// Neighbor returns the neighbor with the given ID.
	Neighbor(peerID peer.ID) (Neighbor, error)
	// NeighborExists checks if a neighbor with the given ID exists.
	NeighborExists(peerID peer.ID) bool
	// DisconnectNeighbor disconnects the neighbor with the given ID.
	DisconnectNeighbor(peerID peer.ID) error

	// Neighbors returns all the neighbors that are currently connected.
	Neighbors() []Neighbor
	// AutopeeringNeighbors returns all the neighbors that are currently connected via autopeering.
	AutopeeringNeighbors() []Neighbor

	P2PHost() host.Host

	Start(ctx context.Context, networkID string) error
	Shutdown()
}
