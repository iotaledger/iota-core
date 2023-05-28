package tipmanager

import "github.com/iotaledger/hive.go/runtime/event"

type Events struct {
	BlockAdded *event.Event1[TipMetadata]

	event.Group[Events, *Events]
}

var NewEvents = event.CreateGroupConstructor(func() *Events {
	return &Events{
		BlockAdded: event.New1[TipMetadata](),
	}
})
