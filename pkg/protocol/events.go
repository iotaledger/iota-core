package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
)

// Events exposes the Events of the main engine of the protocol at a single endpoint.
//
// TODO: It should be replaced with reactive calls to the corresponding events and be deleted but we can do this in a
// later PR (to minimize the code changes to review).
type Events struct {
	Engine         *engine.Events
	ProtocolFilter *event.Event1[*BlockFilteredEvent]
}

// NewEvents creates a new Events instance.
func NewEvents() *Events {
	return &Events{
		Engine:         engine.NewEvents(),
		ProtocolFilter: event.New1[*BlockFilteredEvent](),
	}
}

type BlockFilteredEvent struct {
	Block  *model.Block
	Reason error
	Source peer.ID
}
