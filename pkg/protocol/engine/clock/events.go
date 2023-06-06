package clock

import (
	"time"

	"github.com/iotaledger/hive.go/runtime/event"
)

// Events contains a dictionary of events that are triggered by the Clock.
type Events struct {
	// PreAcceptedTimeUpdated is triggered when the preAccepted time is updated.
	PreAcceptedTimeUpdated *event.Event1[time.Time]

	// AcceptedTimeUpdated is triggered when the accepted time is updated.
	AcceptedTimeUpdated *event.Event1[time.Time]

	// PreConfirmedTimeUpdated is triggered when the preConfirmed time is updated.
	PreConfirmedTimeUpdated *event.Event1[time.Time]

	// ConfirmedTimeUpdated is triggered when the confirmed time is updated.
	ConfirmedTimeUpdated *event.Event1[time.Time]

	// Group is trait that makes the dictionary linkable.
	event.Group[Events, *Events]
}

// NewEvents is the constructor of the Events object.
var NewEvents = event.CreateGroupConstructor(func() (newEvents *Events) {
	return &Events{
		PreAcceptedTimeUpdated:  event.New1[time.Time](),
		AcceptedTimeUpdated:     event.New1[time.Time](),
		PreConfirmedTimeUpdated: event.New1[time.Time](),
		ConfirmedTimeUpdated:    event.New1[time.Time](),
	}
})
