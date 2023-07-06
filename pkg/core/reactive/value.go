package reactive

// Value defines an interface that allows subscribers to read a value and to be notified when the value changes.
type Value[Type comparable] interface {
	// Get returns the current value.
	Get() Type

	// OnUpdate registers the given callback that is triggered when the value changes.
	OnUpdate(consumer func(oldValue, newValue Type)) (unsubscribe func())
}
