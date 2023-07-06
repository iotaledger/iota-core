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

// event implements the Event interface.
type event struct {
	Variable[bool]
}

// newEvent creates a new event.
func newEvent() *event {
	return &event{
		Variable: NewVariable[bool](func(currentValue bool, newValue bool) bool {
			// make sure that the value will always be true once it was set to true
			return currentValue || newValue
		}),
	}
}

// Trigger triggers the event.
func (e *event) Trigger() {
	e.Set(true)
}

// WasTriggered returns true if the event was triggered.
func (e *event) WasTriggered() bool {
	return e.Get()
}

// OnTrigger registers a callback that is executed when the event is triggered.
func (e *event) OnTrigger(handler func()) (unsubscribe func()) {
	return e.OnUpdate(func(_, _ bool) {
		handler()
	})
}
