package promise

import (
	"sync"
)

type Event struct {
	// triggeredCallbacks is nil if the event was already triggered.
	triggeredCallbacks []func()

	// triggeredCallbacksMutex is used to synchronize access to the triggeredCallbacks slice.
	triggeredCallbacksMutex sync.RWMutex

	// callbackOrderMutex is used to ensure that updateCallbacks queued before the event was triggered are executed first.
	callbackOrderMutex sync.RWMutex
}

func NewEvent() *Event {
	return &Event{
		triggeredCallbacks: make([]func(), 0),
	}
}

func (f *Event) Trigger() (wasTriggered bool) {
	f.callbackOrderMutex.Lock()
	defer f.callbackOrderMutex.Unlock()

	for _, callback := range func() (callbacks []func()) {
		f.triggeredCallbacksMutex.Lock()
		defer f.triggeredCallbacksMutex.Unlock()

		callbacks = f.triggeredCallbacks
		if wasTriggered = callbacks != nil; wasTriggered {
			f.triggeredCallbacks = nil
		}

		return callbacks
	}() {
		callback()
	}

	return
}

func (f *Event) queueCallback(callback func()) (callbackQueued bool) {
	f.triggeredCallbacksMutex.Lock()
	defer f.triggeredCallbacksMutex.Unlock()

	if callbackQueued = f.triggeredCallbacks != nil; callbackQueued {
		f.triggeredCallbacks = append(f.triggeredCallbacks, callback)
	}

	return callbackQueued
}

func (f *Event) OnTrigger(callback func()) {
	if !f.queueCallback(callback) {
		f.callbackOrderMutex.RLock()
		defer f.callbackOrderMutex.RUnlock()

		callback()
	}
}

func (f *Event) WasTriggered() bool {
	f.triggeredCallbacksMutex.RLock()
	defer f.triggeredCallbacksMutex.RUnlock()

	return f.triggeredCallbacks == nil
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
