package retainer

import (
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

// Events is a collection of Tangle related Events.
type Events struct {
	// BlockRetained is triggered when a block is stored in the retainer.
	BlockRetained *event.Event1[*blocks.Block]

	event.Group[Events, *Events]
}

// NewEvents contains the constructor of the Events object (it is generated by a generic factory).
var NewEvents = event.CreateGroupConstructor(func() (newEvents *Events) {
	return &Events{
		BlockRetained: event.New1[*blocks.Block](),
	}
})