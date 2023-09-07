package p2p

import (
	"github.com/iotaledger/hive.go/runtime/event"
)

// NeighborEvents is a collection of events specific for a particular neighbors group, e.g "manual" or "auto".
type NeighborEvents struct {
	// Fired when a neighbor connection has been established.
	NeighborAdded *event.Event1[*Neighbor]

	// Fired when a neighbor has been removed.
	NeighborRemoved *event.Event1[*Neighbor]
}

// NewNeighborEvents returns a new instance of NeighborGroupEvents.
func NewNeighborEvents() *NeighborEvents {
	return &NeighborEvents{
		NeighborAdded:   event.New1[*Neighbor](),
		NeighborRemoved: event.New1[*Neighbor](),
	}
}
