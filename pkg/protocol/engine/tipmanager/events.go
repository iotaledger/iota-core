package tipmanager

import "github.com/iotaledger/hive.go/runtime/event"

// Events represents events happening in the TipManager.
type Events struct {
	// BlockAdded gets triggered when a new block was added to the TipManager.
	BlockAdded *event.Event1[TipMetadata]
	// Group makes the Events linkable through the central Events dictionary.
	event.Group[Events, *Events]
}

// NewEvents creates a new Events instance.
var NewEvents = event.CreateGroupConstructor(func() *Events {
	return &Events{
		BlockAdded: event.New1[TipMetadata](),
	}
})
