package network

// Neighbor is a Peer with an active connection.
type Neighbor interface {
	Peer() *Peer
	PacketsRead() uint64
	PacketsWritten() uint64
}
