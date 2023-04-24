package blockissuer

import (
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/model"
)

// Events represents events happening on a block factory.
type Events struct {

	// Triggered when a block is issued, i.e. sent to the protocol to be processed.
	BlockIssued *event.Event1[*model.Block]

	// Fired when an error occurred.
	Error *event.Event1[error]

	event.Group[Events, *Events]
}

// NewEvents contains the constructor of the Events object (it is generated by a generic factory).
var NewEvents = event.CreateGroupConstructor(func() (newEvents *Events) {
	return &Events{
		BlockIssued: event.New1[*model.Block](),
		Error:       event.New1[error](),
	}
})
