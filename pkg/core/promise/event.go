package promise

import (
	"sync"
)

type Event struct {
	// callbacks is nil if the event was already triggered.
	callbacks []func()

	// mutex is used to synchronize access to the callbacks slice.
	mutex sync.RWMutex
}

func NewEvent() *Event {
	return &Event{
		callbacks: make([]func(), 0),
	}
}

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

func (f *Event) queueCallback(callback func()) (callbackQueued bool) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if callbackQueued = f.callbacks != nil; callbackQueued {
		f.callbacks = append(f.callbacks, callback)
	}

	return callbackQueued
}

func (f *Event) OnTrigger(callback func()) {
	if !f.queueCallback(callback) {
		callback()
	}
}

func (f *Event) WasTriggered() bool {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	return f.callbacks == nil
}

type Event1[T any] struct {
	triggeredCallbacks []func(T)
	triggerResult      *T

	mutex sync.RWMutex
}

func NewEvent1[T any]() *Event1[T] {
	return &Event1[T]{
		triggeredCallbacks: make([]func(T), 0),
	}
}

func (f *Event1[T]) Trigger(arg T) {
	for _, callback := range func() (callbacks []func(T)) {
		f.mutex.Lock()
		defer f.mutex.Unlock()

		if callbacks = f.triggeredCallbacks; callbacks != nil {
			f.triggeredCallbacks = nil
			f.triggerResult = &arg
		}

		return callbacks
	}() {
		callback(arg)
	}
}

func (f *Event1[T]) OnTrigger(callback func(T)) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.triggeredCallbacks != nil {
		f.triggeredCallbacks = append(f.triggeredCallbacks, callback)
	} else {
		callback(*f.triggerResult)
	}
}

func (f *Event1[T]) WasTriggered() bool {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	return f.triggeredCallbacks == nil
}
