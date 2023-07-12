package reactive

// Event is a reactive component that can be triggered exactly once and that informs its subscribers about the trigger.
// It conforms to the Variable interface and exposes a boolean value that is set to true when the event was triggered.
type Event interface {
	// Variable holds the boolean value that indicates whether the event was triggered.
	Variable[bool]

	// Trigger triggers the event.
	Trigger()

	// WasTriggered returns true if the event was triggered.
	WasTriggered() bool

	// OnTrigger registers a callback that is executed when the event is triggered.
	OnTrigger(func()) (unsubscribe func())
}

// NewEvent creates a new Event instance.
func NewEvent() Event {
	return newEvent()
}
