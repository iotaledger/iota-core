package promise

import (
	"sync"
)

// region Event

// Event is an event that can be triggered exactly once. Consumers that register themselves after the event was
// triggered, will be called immediately.
type Event struct {
	// callbacks is a slice of callbacks that will be called when the event is triggered.
	callbacks []func()

	// mutex is used to synchronize access to the callbacks.
	mutex sync.RWMutex
}

// NewEvent creates a new Event with 0 generic parameters.
func NewEvent() *Event {
	return &Event{
		callbacks: make([]func(), 0),
	}
}

// Trigger triggers the event. If the event was already triggered, this method does nothing.
func (f *Event) Trigger() (wasTriggered bool) {
	for _, callback := range func() (callbacks []func()) {
		f.mutex.Lock()
		defer f.mutex.Unlock()

		callbacks = f.callbacks
		if wasTriggered = callbacks != nil; wasTriggered {
			f.callbacks = nil
		}

		return callbacks
	}() {
		callback()
	}

	return
}

// OnTrigger registers a callback that will be called when the event is triggered. If the event was already triggered,
// the callback will be called immediately.
func (f *Event) OnTrigger(callback func()) {
	if !func() (callbackRegistered bool) {
		f.mutex.Lock()
		defer f.mutex.Unlock()

		if f.callbacks == nil {
			return false
		}

		f.callbacks = append(f.callbacks, callback)

		return true
	}() {
		callback()
	}
}

// WasTriggered returns true if the event was already triggered.
func (f *Event) WasTriggered() bool {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	return f.callbacks == nil
}

// endregion

// region Event1

// Event1 is an event with a single parameter that can be triggered exactly once. Consumers that register themselves
// after the event was triggered, will be called immediately.
type Event1[T any] struct {
	// callbacks is a slice of callbacks that will be called when the event is triggered.
	callbacks []func(T)

	// value is the value that was passed to the Trigger method.
	value *T

	// mutex is used to synchronize access to the callbacks.
	mutex sync.RWMutex
}

// NewEvent1 creates a new event with 1 generic parameter.
func NewEvent1[T any]() *Event1[T] {
	return &Event1[T]{
		callbacks: make([]func(T), 0),
	}
}

// Trigger triggers the event. If the event was already triggered, this method does nothing.
func (f *Event1[T]) Trigger(arg T) {
	for _, callback := range func() (callbacks []func(T)) {
		f.mutex.Lock()
		defer f.mutex.Unlock()

		if callbacks = f.callbacks; callbacks != nil {
			f.callbacks = nil
			f.value = &arg
		}

		return callbacks
	}() {
		callback(arg)
	}
}

// OnTrigger registers a callback that will be called when the event is triggered. If the event was already triggered,
// the callback will be called immediately.
func (f *Event1[T]) OnTrigger(callback func(T)) {
	if !func() (callbackRegistered bool) {
		f.mutex.Lock()
		defer f.mutex.Unlock()

		if f.callbacks == nil {
			return false
		}

		f.callbacks = append(f.callbacks, callback)

		return true
	}() {
		callback(*f.value)
	}
}

// WasTriggered returns true if the event was already triggered.
func (f *Event1[T]) WasTriggered() bool {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	return f.callbacks == nil
}

// endregion
