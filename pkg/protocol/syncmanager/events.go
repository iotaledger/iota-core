package syncmanager

import (
	"github.com/iotaledger/hive.go/runtime/event"
)

type Events struct {
	UpdatedStatus *event.Event1[*SyncStatus]

	event.Group[Events, *Events]
}

var NewEvents = event.CreateGroupConstructor(func() (newEvents *Events) {
	return &Events{
		UpdatedStatus: event.New1[*SyncStatus](),
	}
})
