package reactive

// Value defines a common interface to read the values of the various agential components.
type Value[Type comparable] interface {
	// Get returns the current value.
	Get() Type

	// OnUpdate registers the given callback that is triggered when the value changes.
	OnUpdate(consumer func(oldValue, newValue Type)) (unsubscribe func())
}
