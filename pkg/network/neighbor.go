package network

type Neighbor interface {
	Peer() *Peer
	PacketsRead() uint64
	PacketsWritten() uint64
}
