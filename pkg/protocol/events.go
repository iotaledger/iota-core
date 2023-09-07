package protocol

import "github.com/iotaledger/iota-core/pkg/protocol/engine"

type Events struct {
	Engine *engine.Events
}

func NewEvents() *Events {
	return &Events{
		Engine: engine.NewEvents(),
	}
}
