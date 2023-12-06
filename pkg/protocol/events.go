package protocol

import "github.com/iotaledger/iota-core/pkg/protocol/engine"

// Events exposes the Events of the main engine of the protocol at a single endpoint.
//
// TODO: It should be replaced with reactive calls to the corresponding events and be deleted but we can do this in a
// later PR (to minimize the code changes to review).
type Events struct {
	Engine *engine.Events
}

// NewEvents creates a new Events instance.
func NewEvents() *Events {
	return &Events{
		Engine: engine.NewEvents(),
	}
}
