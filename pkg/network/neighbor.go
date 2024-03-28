package network

// Neighbor is a Peer with an established connection in the gossip layer.
type Neighbor interface {
	Peer() *Peer
	PacketsRead() uint64
	PacketsWritten() uint64
}
